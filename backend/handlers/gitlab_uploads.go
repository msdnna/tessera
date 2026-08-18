package handlers

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/gitlab"
)

// Outbound asset mirroring (task #2713).
//
// A task's description and comments reference assets by Tessera-relative URL
// ("/api/uploads/<name>" for inline media, "/api/attachments/<id>/download" for a
// task file). Pushed to GitLab verbatim, those resolve against GitLab's own origin
// and render as dead links. So before a body leaves for GitLab we upload the bytes
// into the project's upload store and swap in the "/uploads/<secret>/<file>" URL
// GitLab hands back.
//
// Every mirrored asset is recorded in gitlab_uploads, and that map is read in both
// directions — see rewriteAssets and migration 0054. Without the inbound half, the
// description Tessera stores and the one GitLab returns would never compare equal
// and the title_desc conflict detector would re-upload every image on every edit.

// assetURLPat matches the two Tessera-hosted asset URL shapes that can appear in a
// stored body: an inline media upload, and a signed proxy link to an asset that
// actually lives in GitLab (written by the inbound rewrite).
const assetURLPat = `/api/uploads/[A-Za-z0-9_-]+\.[A-Za-z0-9]+|/api/gitlab/asset\?[^\s)"'<>]+`

// assetLinkRe matches a markdown image or link whose target is such an asset. It is
// applied before the bare-URL pass so a failed upload can degrade the whole
// construct to text instead of leaving a broken image behind.
var assetLinkRe = regexp.MustCompile(`(!?)\[([^\]\n]*)\]\((` + assetURLPat + `)\)`)

// assetURLRe matches a bare occurrence of an asset URL (HTML <img src>, an
// autolink, a reference definition).
var assetURLRe = regexp.MustCompile(assetURLPat)

// assetStats counts what a push did with a body's assets, for the API response and
// the sync journal.
type assetStats struct {
	Uploaded int `json:"uploaded"`
	Skipped  int `json:"skipped"`
}

// outboundRefs lists the distinct Tessera-hosted asset URLs a body references,
// ignoring anything inside a code fence or an inline code span (rewriting a link
// inside a snippet would silently corrupt the text). Exported behaviour is pure —
// no I/O — so it is unit-testable without a GitLab.
func outboundRefs(body string) []string {
	if !hasAssetRef(body) {
		return nil
	}
	code := codeRanges(body)
	seen := map[string]bool{}
	var out []string
	for _, m := range assetURLRe.FindAllStringIndex(body, -1) {
		if inRanges(code, m[0]) {
			continue
		}
		u := body[m[0]:m[1]]
		if seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// hasAssetRef is the cheap pre-check that keeps the regexes off the vast majority
// of bodies, which reference nothing.
func hasAssetRef(body string) bool {
	return strings.Contains(body, "/api/uploads/") || strings.Contains(body, "/api/gitlab/asset")
}

// codeRanges returns the byte ranges of body that markdown renders as code —
// fenced blocks and inline spans. Indented code blocks are deliberately not
// detected: telling one from a wrapped list item needs a real parser, and treating
// an indented line as code would skip a legitimate link and leave it dead.
func codeRanges(body string) [][2]int {
	var out [][2]int
	fenceOpen := ""  // the exact fence marker that opened the current block
	fenceStart := -1 // byte offset of that block's start
	pos := 0
	for pos <= len(body) {
		end := strings.IndexByte(body[pos:], '\n')
		lineEnd := len(body)
		if end >= 0 {
			lineEnd = pos + end
		}
		line := body[pos:lineEnd]
		trimmed := strings.TrimLeft(line, " \t")

		switch {
		case fenceOpen != "":
			// Inside a fence: only a closing marker of at least the opening length ends it.
			if closesFence(trimmed, fenceOpen) {
				out = append(out, [2]int{fenceStart, lineEnd})
				fenceOpen, fenceStart = "", -1
			}
		case strings.HasPrefix(trimmed, "```"), strings.HasPrefix(trimmed, "~~~"):
			fenceOpen = fenceMarker(trimmed)
			fenceStart = pos
		default:
			out = append(out, inlineCodeRanges(body, pos, lineEnd)...)
		}

		if end < 0 {
			break
		}
		pos = lineEnd + 1
	}
	// An unterminated fence still swallows the rest of the body, exactly as marked renders it.
	if fenceStart >= 0 {
		out = append(out, [2]int{fenceStart, len(body)})
	}
	return out
}

// fenceMarker returns the run of fence characters that opens a block ("```" or longer).
func fenceMarker(trimmed string) string {
	ch := trimmed[0]
	n := 0
	for n < len(trimmed) && trimmed[n] == ch {
		n++
	}
	return trimmed[:n]
}

// closesFence reports whether a line closes the fence opened by `open`: the same
// character, at least as long, and nothing but whitespace after it.
func closesFence(trimmed, open string) bool {
	if !strings.HasPrefix(trimmed, open) {
		return false
	}
	return strings.TrimSpace(strings.TrimLeft(trimmed, string(open[0]))) == ""
}

// inlineCodeRanges finds `code` spans within one line. A run of N backticks opens a
// span that the next run of exactly N backticks closes; an unmatched run is literal.
func inlineCodeRanges(body string, start, end int) [][2]int {
	var out [][2]int
	i := start
	for i < end {
		if body[i] != '`' {
			i++
			continue
		}
		openStart := i
		for i < end && body[i] == '`' {
			i++
		}
		n := i - openStart
		// Look for a closing run of exactly n backticks on the same line.
		j := i
		for j < end {
			if body[j] != '`' {
				j++
				continue
			}
			runStart := j
			for j < end && body[j] == '`' {
				j++
			}
			if j-runStart == n {
				out = append(out, [2]int{openStart, j})
				break
			}
		}
		if j >= end {
			return out // unmatched opener — the rest of the line is literal text
		}
		i = j
	}
	return out
}

// inRanges reports whether an offset falls inside any of the ranges.
func inRanges(ranges [][2]int, off int) bool {
	for _, r := range ranges {
		if off >= r[0] && off < r[1] {
			return true
		}
	}
	return false
}

// pushAssets rewrites a body's Tessera-hosted asset links so they resolve for GitLab
// readers, uploading the bytes into the project's store on first use. Assets already
// mirrored are reused from gitlab_uploads (so re-pushing an unchanged description
// uploads nothing). A single asset that can't be mirrored — too large for the
// instance, insufficient token scope, missing on disk — degrades to a text note and
// is counted in skipped, rather than failing the whole push.
func (h *API) pushAssets(ctx context.Context, client *gitlab.Client, integ db.GitlabIntegration, body string) (string, assetStats) {
	var st assetStats
	if !hasAssetRef(body) {
		return body, st
	}
	code := codeRanges(body)

	// Resolve every distinct reference once, so a body embedding the same image
	// twice costs one upload and one map row.
	resolved := map[string]string{} // asset URL → GitLab URL ("" = could not mirror)
	for _, ref := range outboundRefs(body) {
		glURL, err := h.mirrorAsset(ctx, client, integ, ref)
		if err != nil {
			log.Printf("gitlab push assets: integration %s could not mirror %s: %v", integ.ID, ref, err)
			resolved[ref] = ""
			st.Skipped++
			continue
		}
		resolved[ref] = glURL
		st.Uploaded++
	}

	return rewriteOutbound(body, code, resolved), st
}

// rewriteOutbound is the pure half of pushAssets: given the resolved asset map
// (URL → GitLab URL, "" for one that couldn't be mirrored) it produces the body to
// send. Kept free of I/O so the round-trip invariant with rewriteAssets — the thing
// that keeps title_desc conflict detection honest — is directly testable.
func rewriteOutbound(body string, code [][2]int, resolved map[string]string) string {
	// Construct pass first: a link/image we could not mirror becomes plain text, so
	// GitLab shows an explanation instead of a broken image icon.
	out := replaceOutsideCode(body, code, assetLinkRe, func(m []string) string {
		if glURL := resolved[m[3]]; glURL != "" {
			return m[1] + "[" + m[2] + "](" + glURL + ")"
		}
		label := strings.TrimSpace(m[2])
		if label == "" {
			label = "вложение"
		}
		return "_(" + label + " — файл доступен в Tessera)_"
	})
	// Bare occurrences (HTML <img src>, autolinks). A failure here has no construct
	// to degrade, so the URL is left as-is — it was already dead, and mangling raw
	// HTML would be worse.
	return replaceOutsideCode(out, codeRanges(out), assetURLRe, func(m []string) string {
		if glURL := resolved[m[0]]; glURL != "" {
			return glURL
		}
		return m[0]
	})
}

// replaceOutsideCode applies re to body, skipping matches that start inside a code
// range, and hands the submatch slice to fn for the replacement.
func replaceOutsideCode(body string, code [][2]int, re *regexp.Regexp, fn func([]string) string) string {
	idx := re.FindAllStringSubmatchIndex(body, -1)
	if len(idx) == 0 {
		return body
	}
	var b strings.Builder
	prev := 0
	for _, loc := range idx {
		if inRanges(code, loc[0]) {
			continue
		}
		groups := make([]string, len(loc)/2)
		for g := range groups {
			if loc[2*g] >= 0 {
				groups[g] = body[loc[2*g]:loc[2*g+1]]
			}
		}
		b.WriteString(body[prev:loc[0]])
		b.WriteString(fn(groups))
		prev = loc[1]
	}
	b.WriteString(body[prev:])
	return b.String()
}

// mirrorAsset resolves one Tessera asset URL to the URL GitLab serves it by,
// uploading it on first use and memoising the pair in gitlab_uploads.
func (h *API) mirrorAsset(ctx context.Context, client *gitlab.Client, integ db.GitlabIntegration, ref string) (string, error) {
	// A signed proxy link points at an asset that already lives in GitLab — it got
	// this shape on the way IN (see rewriteAssets). Unwrapping it back to the
	// project-relative path costs nothing and, unlike a re-upload, round-trips: the
	// next pull rewrites it to exactly the proxy URL the task already stores.
	if strings.HasPrefix(ref, "/api/gitlab/asset?") {
		if rel, ok := h.unwrapAssetProxy(ref); ok {
			return rel, nil
		}
		return "", fmt.Errorf("proxy link failed signature check")
	}

	if row, err := h.q.GetGitlabUpload(ctx, db.GetGitlabUploadParams{
		IntegrationID: integ.ID, SourceKey: ref,
	}); err == nil && row.GlUrl != "" {
		return row.GlUrl, nil
	}

	name := strings.TrimPrefix(ref, "/api/uploads/")
	path := filepath.Join(h.uploadDir, "media", filepath.Base(name))
	f, err := os.Open(path) //nolint:gosec // name is regex-constrained and Base'd
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	up, err := client.UploadFile(ctx, integ.ProjectPath, name, f)
	if err != nil {
		return "", err
	}
	if _, err := h.q.UpsertGitlabUpload(ctx, db.UpsertGitlabUploadParams{
		IntegrationID: integ.ID, SourceKey: ref, GlUrl: up.URL, GlMarkdown: up.Markdown,
	}); err != nil {
		// The bytes are in GitLab; losing the map row only costs a duplicate upload
		// next time, so don't fail the push over it.
		log.Printf("gitlab push assets: record upload %s: %v", ref, err)
	}
	return up.URL, nil
}

// unwrapAssetProxy turns a signed /api/gitlab/asset link back into the
// project-relative "/uploads/…" path it wraps, verifying the signature so a
// hand-written link can't smuggle an arbitrary path into a pushed body.
func (h *API) unwrapAssetProxy(ref string) (string, bool) {
	q, err := url.Parse(ref)
	if err != nil {
		return "", false
	}
	v := q.Query()
	wsID, err := uuid.Parse(v.Get("ws"))
	if err != nil {
		return "", false
	}
	pb, err := base64.RawURLEncoding.DecodeString(v.Get("p"))
	if err != nil {
		return "", false
	}
	rel := string(pb)
	if !strings.HasPrefix(rel, "/uploads/") {
		return "", false
	}
	if !hmac.Equal([]byte(v.Get("sig")), []byte(h.signAsset(wsID, rel))) {
		return "", false
	}
	return rel, true
}

// pushTaskAttachmentsNote mirrors the task's Attachments tab into the new issue as a
// single note. Deliberately a note and not part of the description: the description
// is compared field-by-field against `tasks.description` on every write-back, so any
// block we inject there would read as a permanent divergence and park the sync in a
// conflict. The note carries the Tessera marker so the next pull recognises it as
// ours and doesn't import it back as a user comment.
//
// Best-effort by design — the issue already exists and is not rolled back if this
// fails.
func (h *API) pushTaskAttachmentsNote(ctx context.Context, client *gitlab.Client, integ db.GitlabIntegration, taskID uuid.UUID, iid int64) assetStats {
	var st assetStats
	atts, err := h.q.ListTaskAttachments(ctx, taskID)
	if err != nil || len(atts) == 0 {
		return st
	}
	var lines []string
	for _, att := range atts {
		key := "att:" + att.ID.String()
		glURL, merr := h.mirrorAttachment(ctx, client, integ, key, att)
		if merr != nil {
			log.Printf("gitlab push attachments: task %s file %s: %v", taskID, att.Filename, merr)
			lines = append(lines, "- "+att.Filename+" — файл доступен в Tessera")
			st.Skipped++
			continue
		}
		lines = append(lines, fmt.Sprintf("- [%s](%s) — %s", att.Filename, glURL, humanBytes(att.Size)))
		st.Uploaded++
	}
	if len(lines) == 0 {
		return st
	}
	body := "**Вложения задачи Tessera**\n\n" + strings.Join(lines, "\n") + tesseraCommentMarker
	if _, cerr := client.CreateIssueNote(ctx, integ.ProjectPath, iid, body); cerr != nil {
		log.Printf("gitlab push attachments: post note for task %s: %v", taskID, cerr)
	}
	return st
}

// mirrorAttachment uploads one task attachment, reusing an earlier mirror of the
// same file.
func (h *API) mirrorAttachment(ctx context.Context, client *gitlab.Client, integ db.GitlabIntegration, key string, att db.ListTaskAttachmentsRow) (string, error) {
	if row, err := h.q.GetGitlabUpload(ctx, db.GetGitlabUploadParams{
		IntegrationID: integ.ID, SourceKey: key,
	}); err == nil && row.GlUrl != "" {
		return row.GlUrl, nil
	}
	f, err := os.Open(att.StoragePath) //nolint:gosec // path comes from our own attachments row
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	up, err := client.UploadFile(ctx, integ.ProjectPath, att.Filename, f)
	if err != nil {
		return "", err
	}
	if _, err := h.q.UpsertGitlabUpload(ctx, db.UpsertGitlabUploadParams{
		IntegrationID: integ.ID, SourceKey: key, GlUrl: up.URL, GlMarkdown: up.Markdown,
	}); err != nil {
		log.Printf("gitlab push attachments: record upload %s: %v", key, err)
	}
	return up.URL, nil
}

// humanBytes renders a file size the way the attachments tab does.
func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f МБ", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f КБ", float64(n)/float64(1<<10))
	default:
		return fmt.Sprintf("%d Б", n)
	}
}
