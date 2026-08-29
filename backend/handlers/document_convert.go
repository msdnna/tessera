package handlers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/converter"
	"tessera/internal/db"
	"tessera/internal/office"
	"tessera/middleware"
)

// Import and export of office formats (#2733, D8 of #2718).
//
// Both directions go through the LibreOffice sidecar, and both keep the
// HTML↔blocks step on the side that owns the schema:
//
//	import  file -> [sidecar] -> HTML -> (client) TipTap -> blocks
//	export  blocks -> (server) renderDocFODT -> .fodt -> [sidecar] -> file
//
// Export went through HTML until #2849; it now goes through flat ODF, because
// HTML has one page geometry and a document with sections needs several. The
// HTML renderer stays for format=html, which needs no sidecar at all.
//
// Parsing stays in the browser because the editor's schema *is* the allow-list
// (docImport.js says so, and D9's template upload already works this way): a
// second HTML→blocks walk in Go would be a worse converter that drifts from the
// first. Rendering, by contrast, is on the server so that an export does not
// require a client with TipTap loaded — see document_render.go.

const (
	// maxDocImportBytes bounds an uploaded source document. Deliberately below
	// the 25 MiB attachment cap: a .docx expands when converted, and the result
	// has to fit the 4 MiB content ceiling with its pictures split out as assets.
	maxDocImportBytes = 20 << 20

	// Ceilings on what one import may add to a workspace's storage. A document
	// full of screenshots is normal; a hundred megabytes of them arriving under a
	// single click is not, and refusing loudly beats discovering it in the volume.
	maxImportAssets      = 200
	maxImportAssetBytes  = 8 << 20
	maxImportTotalAssets = 60 << 20
)

// docImportExts is what the import route accepts. It is not "everything
// LibreOffice opens": the list is shown to the user in the file picker, so it
// has to be something we can state rather than discover. Markdown and JSON are
// absent on purpose — those are handled in the browser without a sidecar
// (docImport.js), and routing them through LibreOffice would make an import
// that works today depend on a service that may not be deployed.
var docImportExts = map[string]bool{
	".doc":  true,
	".docx": true,
	".odt":  true,
	".rtf":  true,
	".fodt": true,
	".html": true,
	".htm":  true,
	".txt":  true,
}

// docExportFormats maps a requested format to the file extension and the MIME
// type of the response.
var docExportFormats = map[string]struct {
	ext  string
	mime string
}{
	"pdf":  {"pdf", "application/pdf"},
	"docx": {"docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	"odt":  {"odt", "application/vnd.oasis.opendocument.text"},
	"html": {"html", "text/html; charset=utf-8"},
}

// dataImageRe finds the pictures LibreOffice inlined into its HTML output. The
// sidecar folds them into data: URIs because it returns bytes, not a directory;
// this side turns them back into files so the document body stores a URL rather
// than a megabyte of base64 in jsonb.
var dataImageRe = regexp.MustCompile(`(?i)(<img\b[^>]*?\bsrc=")data:([a-z0-9.+/-]+);base64,([A-Za-z0-9+/=\s]+)(")`)

// ConverterStatus tells the client whether office import/export is available at
// all, and with which formats.
//
// This exists so the UI can be honest rather than optimistic: without it the
// import button would be offered on every install and fail only after the user
// picked a file, which is the worst moment to learn the feature is not deployed.
func (h *API) ConverterStatus(c *gin.Context) {
	// native_formats is reported in every branch: PDF import needs no sidecar,
	// so a client that hides the import button on `available:false` would hide a
	// feature that works. It is a list rather than a flag because the set of
	// formats we handle ourselves is the kind of thing that grows.
	if !h.converter.Enabled() {
		c.JSON(http.StatusOK, gin.H{
			"available":      false,
			"reason":         "конвертация документов не настроена (CONVERTER_URL)",
			"native_formats": docNativeImportExts,
		})
		return
	}
	info, err := h.converter.Health(c)
	if err != nil {
		// The reason in the body is what the user reads, so it cannot carry the
		// transport error — and without this line nothing else does either: the
		// access log shows a 200, the sidecar's log is empty because the request
		// never arrived, and an operator who set CONVERTER_URL correctly is left
		// with no thread to pull (#2733).
		log.Printf("converter: health check failed: %v", err)
		c.JSON(http.StatusOK, gin.H{
			"available":      false,
			"reason":         "сервис конвертации недоступен",
			"native_formats": docNativeImportExts,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"available":      true,
		"import_formats": sortedKeys(docImportExts),
		"native_formats": docNativeImportExts,
		"export_formats": exportFormatsFor(info),
	})
}

// ImportDocument creates a document from an uploaded office file.
//
// The order — convert, then create — is the point: a conversion that fails
// leaves no half-made document behind for the user to find and delete.
func (h *API) ImportDocument(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "файл не передан"})
		return
	}
	if fileHeader.Size > maxDocImportBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "файл больше 20 МБ"})
		return
	}
	name := filepath.Base(fileHeader.Filename)
	ext := strings.ToLower(filepath.Ext(name))

	// PDF branches off before the converter gate on purpose: it is stored, not
	// converted, so it is the one import that keeps working on an install with
	// no sidecar deployed. Gating it behind CONVERTER_URL would make the
	// cheapest path depend on the heaviest dependency.
	if ext == ".pdf" {
		h.importPdfDocument(c, wsID, fileHeader, name)
		return
	}
	if !h.converter.Enabled() {
		converterUnavailable(c, converter.ErrDisabled)
		return
	}
	if !docImportExts[ext] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "поддерживаются " + strings.Join(append(sortedKeys(docImportExts), ".pdf"), ", "),
		})
		return
	}
	src, err := readUpload(fileHeader, maxDocImportBytes)
	if err != nil {
		fail(c, err)
		return
	}

	html, err := h.converter.Convert(c, src, strings.TrimPrefix(ext, "."), "html")
	if err != nil {
		converterUnavailable(c, err)
		return
	}

	doc, ok := h.createImportedDocument(c, wsID, name)
	if !ok {
		return
	}

	body, images := h.storeImportedImages(doc, string(html))
	images.log(doc, name)
	h.broadcast(wsID, "document.created", documentMeta(doc))
	// The body comes back as HTML rather than as blocks: the client parses it
	// with the editor's schema and saves it through the ordinary content
	// endpoint, so an import is validated by exactly the same code as typing.
	//
	// The page geometry is read from the source bytes rather than from the HTML,
	// because it is not in the HTML to begin with — see importedPageSetup.
	body, page, differ := importedPageSetup(body, ext, src, doc, name)
	c.JSON(http.StatusCreated, gin.H{
		"document":               viewDocument(doc),
		"html":                   body,
		"images_dropped":         images.dropped,
		"images_dropped_reason":  images.summary(),
		"images_dropped_reasons": images.counts(),
		"source_file_name":       name,
		"page":                   page,
		"sections_differ":        differ,
	})
}

// importedPageSetup gives the imported body its page geometry: a section break
// per boundary the source file had, and the first section's geometry for the
// document node.
//
// Read from the original bytes, not from the converted HTML, because the
// conversion is where it is lost: LibreOffice emits an @page rule for the first
// section only, so a file whose landscape section holds the wide table arrives
// as portrait HTML with nothing but a page break where the boundary was.
// LayoutSections puts the geometry back at that break (#2848).
//
// When the breaks cannot be matched to the sections it falls back to what
// #2821 did — one sheet, the widest section — and returns `differ` so the client
// can say the sheet is a reduction of the file rather than a copy of it. A laid
// out document is not a reduction, so it never carries that warning.
//
// A file we cannot read geometry from is not an error: .doc, .rtf, .txt and
// .html have none to give, and the client falls back to the editor's default.
// Returns nil in that case, so the response carries `page: null` and the client
// has one thing to test rather than a zero geometry to second-guess.
func importedPageSetup(body, ext string, src []byte, doc db.Document, name string) (string, *office.PageSetup, bool) {
	setups, err := office.PageSetups(ext, src)
	if err != nil {
		if !errors.Is(err, office.ErrNoPageSetup) {
			// Worth a line but not a failed import: the document's text converted
			// fine, and refusing it over its margins would trade a real import for a
			// cosmetic one.
			log.Printf("document import %s (%q): page setup unreadable: %v", doc.ID, name, err)
		}
		return body, nil, false
	}
	if office.SectionsInDocumentOrder(ext) {
		if out, ok := office.LayoutSections(body, setups); ok {
			first := setups[0]
			return out, &first, false
		}
	}
	widest, ok := office.Widest(setups)
	if !ok {
		return body, nil, false
	}
	return body, &widest, office.Differ(setups)
}

// createImportedDocument makes the empty document an import will fill in, doing
// the parts of it the server owns: placement (parent, project, position), the
// unique slug and the title derived from the file name.
//
// Shared by the two import paths — the converted one above and the PDF one in
// document_pdf.go — because "where does an imported document land" is a rule
// about the section, not about the format that happened to arrive.
//
// Reports false having already answered the request.
func (h *API) createImportedDocument(c *gin.Context, wsID uuid.UUID, fileName string) (db.Document, bool) {
	var parentID *uuid.UUID
	if raw := c.PostForm("parent_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "malformed parent_id"})
			return db.Document{}, false
		}
		if !h.parentInWorkspace(c, id, wsID) {
			return db.Document{}, false
		}
		parentID = &id
	}
	var projectID *uuid.UUID
	if raw := c.PostForm("project_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "malformed project_id"})
			return db.Document{}, false
		}
		projectID = &id
	}

	title := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	if strings.TrimSpace(title) == "" {
		title = "Импортированный документ"
	}
	uid := middleware.CurrentUser(c)
	doc, err := h.q.CreateDocument(c, db.CreateDocumentParams{
		WorkspaceID: wsID,
		ProjectID:   projectID,
		ParentID:    parentID,
		AuthorID:    &uid,
		Title:       title,
		Slug:        h.uniqueDocumentSlug(c, wsID, title),
		Icon:        "",
		Position:    h.nextDocumentPosition(c, wsID, parentID),
		Content:     []byte(`{"type":"doc","content":[]}`),
		Preview:     "",
	})
	if err != nil {
		fail(c, err)
		return db.Document{}, false
	}
	return doc, true
}

// ExportDocument renders a document and hands back a file.
//
// A plain GET, so the browser, Android and a script with a token all export the
// same way. format=html needs no sidecar at all and therefore keeps working on
// an install without one — the cheapest useful export should not be the one
// that requires the heaviest dependency.
func (h *API) ExportDocument(c *gin.Context) {
	doc, ok := h.loadDocument(c)
	if !ok {
		return
	}
	format := strings.ToLower(strings.TrimSpace(c.Query("format")))
	if format == "" {
		format = "pdf"
	}
	spec, ok := docExportFormats[format]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "поддерживаются " + strings.Join(sortedKeys(docExportFormats), ", ")})
		return
	}

	var root docNode
	if len(doc.Content) > 0 {
		if err := json.Unmarshal(doc.Content, &root); err != nil {
			fail(c, fmt.Errorf("document %s holds unreadable content: %w", doc.ID, err))
			return
		}
	}
	// format=html is the one export rendered here and handed straight back, so it
	// keeps the HTML renderer. Everything else goes to the sidecar as .fodt: HTML
	// carries a single @page rule and would flatten a document's sections onto one
	// sheet (#2849), which is the whole of what #2827 set out to fix.
	if format == "html" {
		out := []byte(renderDocHTML(doc.Title, root, h.inlineDocAsset(doc.WorkspaceID)))
		c.Header("Content-Disposition", contentDisposition(doc.Title+"."+spec.ext))
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(http.StatusOK, spec.mime, out)
		return
	}

	if !h.converter.Enabled() {
		converterUnavailable(c, converter.ErrDisabled)
		return
	}
	src := []byte(renderDocFODT(doc.Title, root, h.inlineDocAsset(doc.WorkspaceID)))
	out, err := h.converter.Convert(c, src, "fodt", format)
	if err != nil {
		converterUnavailable(c, err)
		return
	}

	c.Header("Content-Disposition", contentDisposition(doc.Title+"."+spec.ext))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, spec.mime, out)
}

// dropReason names why one inlined picture did not become a document asset.
//
// The reasons are separate values rather than one "dropped" counter because
// they call for different actions: a format we do not store is the user's to
// fix (re-insert the picture as PNG), a ceiling is the operator's, and a write
// failure is ours. Reporting only the count made an import that lost fifteen
// figures indistinguishable from all three (#2755) — the diagnosis cost an hour
// of forensics that the response could have answered.
type dropReason string

const (
	dropUnreadable  dropReason = "unreadable"
	dropCountLimit  dropReason = "count_limit"
	dropTotalLimit  dropReason = "total_limit"
	dropBase64      dropReason = "base64"
	dropEmpty       dropReason = "empty"
	dropTooLarge    dropReason = "too_large"
	dropUnsupported dropReason = "unsupported_type"
	dropMkdir       dropReason = "mkdir_failed"
	dropWrite       dropReason = "write_failed"
)

// dropReasonText is what the user reads in the import warning, so it says what
// happened to their document rather than which branch was taken.
var dropReasonText = map[dropReason]string{
	dropUnreadable:  "не удалось разобрать разметку",
	dropCountLimit:  "превышен лимит на число картинок в одном импорте",
	dropTotalLimit:  "превышен лимит на общий объём картинок",
	dropBase64:      "повреждённые данные картинки",
	dropEmpty:       "пустая картинка",
	dropTooLarge:    "картинка больше 8 МБ",
	dropUnsupported: "формат картинки не поддерживается",
	dropMkdir:       "не удалось создать каталог для файлов",
	dropWrite:       "не удалось записать файл",
}

// importImageStats is the outcome of moving one import's pictures onto disk.
type importImageStats struct {
	saved   int
	dropped int
	reasons map[dropReason]int
	// details keeps a bounded sample for the log — enough to identify the
	// offending picture (its declared type and size) without turning one bad
	// document into thousands of log lines.
	details []string
}

// maxDropLogLines bounds the per-picture log sample. Fifteen dropped figures
// are worth naming one by one; a thousand are not.
const maxDropLogLines = 10

func (s *importImageStats) drop(reason dropReason, mime string, size int) {
	s.dropped++
	if s.reasons == nil {
		s.reasons = map[dropReason]int{}
	}
	s.reasons[reason]++
	if len(s.details) < maxDropLogLines {
		s.details = append(s.details, fmt.Sprintf("%s (declared %q, %d B)", reason, mime, size))
	}
}

// counts is the reason breakdown for the API response, keyed by the stable
// reason name so a client can branch on it.
func (s importImageStats) counts() map[string]int {
	if s.dropped == 0 {
		return nil
	}
	out := make(map[string]int, len(s.reasons))
	for reason, n := range s.reasons {
		out[string(reason)] = n
	}
	return out
}

// summary is the human-readable half of the same thing: one Russian phrase the
// client can append to its warning without knowing the reason vocabulary.
func (s importImageStats) summary() string {
	if s.dropped == 0 {
		return ""
	}
	names := make([]string, 0, len(s.reasons))
	for reason := range s.reasons {
		names = append(names, string(reason))
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		text, ok := dropReasonText[dropReason(name)]
		if !ok {
			text = name
		}
		parts = append(parts, fmt.Sprintf("%s — %d", text, s.reasons[dropReason(name)]))
	}
	return strings.Join(parts, "; ")
}

// log records what an import did with its pictures. It runs on every import,
// not only on failure: "0 dropped" in the log is what tells an operator the
// import happened at all on this build, which is exactly the question #2755
// could not answer from the logs afterwards.
func (s importImageStats) log(doc db.Document, fileName string) {
	if s.dropped == 0 {
		log.Printf("document import %s (%q): %d image(s) stored", doc.ID, fileName, s.saved)
		return
	}
	log.Printf("document import %s (%q): %d image(s) stored, %d dropped [%s]%s",
		doc.ID, fileName, s.saved, s.dropped, s.summary(), logSuffix(s))
}

func logSuffix(s importImageStats) string {
	if len(s.details) == 0 {
		return ""
	}
	suffix := "; first: " + strings.Join(s.details, ", ")
	if s.dropped > len(s.details) {
		suffix += ", …"
	}
	return suffix
}

// storeImportedImages moves the pictures LibreOffice inlined into the HTML onto
// disk as document assets and rewrites the markup to point at their signed URLs.
//
// Returns the rewritten HTML and what happened to each picture. Dropping is not
// silent at the API level — the count and the reasons are reported to the
// client and to the log — because a document that quietly lost half its figures
// looks like a successful import until somebody scrolls.
func (h *API) storeImportedImages(doc db.Document, page string) (string, importImageStats) {
	dir := filepath.Join(h.uploadDir, "documents", doc.ID.String())
	stats := importImageStats{}
	total := 0
	made := false

	out := dataImageRe.ReplaceAllStringFunc(page, func(match string) string {
		groups := dataImageRe.FindStringSubmatch(match)
		if len(groups) != 5 {
			stats.drop(dropUnreadable, "", len(match))
			return ""
		}
		mime := groups[2]
		if stats.saved >= maxImportAssets {
			stats.drop(dropCountLimit, mime, 0)
			return ""
		}
		if total >= maxImportTotalAssets {
			stats.drop(dropTotalLimit, mime, 0)
			return ""
		}
		// The base64 payload carries the newlines LibreOffice wrapped it with.
		payload := strings.NewReplacer("\n", "", "\r", "", " ", "", "\t", "").Replace(groups[3])
		raw, err := base64.StdEncoding.DecodeString(payload)
		switch {
		case err != nil:
			stats.drop(dropBase64, mime, len(payload))
			return ""
		case len(raw) == 0:
			stats.drop(dropEmpty, mime, 0)
			return ""
		case len(raw) > maxImportAssetBytes:
			stats.drop(dropTooLarge, mime, len(raw))
			return ""
		}
		// The declared MIME type is ignored in favour of sniffing the bytes: the
		// same rule UploadDocumentAsset follows, and for the same reason — the
		// declaration comes from outside and would otherwise let anything through
		// as an image.
		ext, ok := mediaExts[sniffBytes(raw)]
		if !ok {
			// The sniffed type, not the declared one, is what explains this drop:
			// a Word document full of EMF vector drawings arrives declaring
			// image/png and is refused on its bytes.
			stats.drop(dropUnsupported, sniffBytes(raw), len(raw))
			return ""
		}
		if !made {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				stats.drop(dropMkdir, mime, len(raw))
				return ""
			}
			made = true
		}
		fname := uuid.NewString() + ext
		if err := os.WriteFile(filepath.Join(dir, fname), raw, 0o644); err != nil {
			stats.drop(dropWrite, mime, len(raw))
			return ""
		}
		stats.saved++
		total += len(raw)
		return groups[1] + escapeAttr(h.docAssetURL(doc.WorkspaceID, doc.ID, fname)) + groups[4]
	})
	return out, stats
}

// converterUnavailable answers a conversion that could not happen.
//
// A missing or unreachable sidecar is 503 rather than 500: it is a deployment
// state the operator chose, not a fault in the request or a bug to be reported,
// and the client shows it as "conversion is unavailable" instead of "something
// went wrong". A refusal from LibreOffice itself (a file it cannot parse) keeps
// its own status, because that one *is* about the file the user picked.
func converterUnavailable(c *gin.Context, err error) {
	var convErr *converter.Error
	if errors.As(err, &convErr) && convErr.Status >= 400 && convErr.Status < 500 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "не удалось преобразовать документ: " + convErr.Message,
		})
		return
	}
	// Same reasoning as in ConverterStatus: a refusal the sidecar produced is
	// already in the response above, but a failure to reach it at all leaves the
	// user with a message that names no cause. Log it once, here.
	log.Printf("converter: request failed: %v", err)
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"error": "сервис конвертации документов недоступен",
	})
}

// exportFormatsFor intersects what we know how to ask for with what the sidecar
// says it can produce, so a client is never offered a format that would fail.
// html is always there: it is rendered here and needs no sidecar.
func exportFormatsFor(info converter.Info) []string {
	have := map[string]bool{"html": true}
	for _, t := range info.Targets {
		have[strings.ToLower(t)] = true
	}
	out := make([]string, 0, len(docExportFormats))
	for f := range docExportFormats {
		if have[f] {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

// contentDisposition builds an attachment header that survives a Cyrillic
// title. The ASCII filename is a fallback for clients that do not read RFC 5987;
// without the starred form a document called "Протокол" downloads as a row of
// underscores.
func contentDisposition(name string) string {
	ascii := make([]rune, 0, len(name))
	for _, r := range name {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			ascii = append(ascii, '_')
			continue
		}
		ascii = append(ascii, r)
	}
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
		string(ascii), url.PathEscape(name))
}

// readUpload reads a multipart file into memory under a ceiling.
func readUpload(fh *multipart.FileHeader, limit int64) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, limit))
}

// sniffBytes is sniffContentType for bytes already in memory. The imported
// pictures arrive inside the HTML, not as multipart files, so there is no
// FileHeader to open.
func sniffBytes(raw []byte) string {
	if len(raw) > 512 {
		raw = raw[:512]
	}
	return http.DetectContentType(raw)
}

func escapeAttr(s string) string {
	return strings.NewReplacer(`"`, "&quot;", "&", "&amp;").Replace(s)
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
