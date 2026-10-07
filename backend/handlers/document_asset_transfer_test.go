package handlers

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// writeDocAsset puts one image on disk for a document and answers its bytes.
func writeDocAsset(t *testing.T, h *API, docID uuid.UUID, name string) []byte {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(h.uploadDir, "documents", docID.String()), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte{0x89, 'P', 'N', 'G', 4, 5, 6}
	if err := os.WriteFile(h.docAssetPath(docID, name), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	return payload
}

// A project moved to another workspace takes its documents with it
// (TransferProject re-stamps documents.workspace_id), but the image URLs inside
// their bodies still name the workspace they were minted in — nothing rewrites
// stored content. The export must keep inlining them: the pictures did not move
// anywhere, only the row above them did.
func TestInlineDocAssetSurvivesWorkspaceTransfer(t *testing.T) {
	dir := t.TempDir()
	h := &API{uploadDir: dir, assetKey: []byte("test-key")}
	oldWS, docID := uuid.New(), uuid.New()
	name := uuid.NewString() + ".png"
	payload := writeDocAsset(t, h, docID, name)

	// The document has since moved to another workspace; its stored src still
	// names the one the URL was minted in, which is the whole of the case.
	src := h.docAssetURL(oldWS, docID, name)
	got := h.inlineDocAsset()(src)
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(payload)
	if got != want {
		t.Fatalf("asset of a transferred document was not inlined:\n got %q\nwant %q", got, want)
	}
}

// The signature stays the only thing that matters. A URL Tessera never minted is
// refused whichever workspace it names, so widening the check to the workspace in
// the URL does not turn the exporter into a file reader.
func TestInlineDocAssetStillRefusesUnsignedURLs(t *testing.T) {
	dir := t.TempDir()
	h := &API{uploadDir: dir, assetKey: []byte("test-key")}
	wsID, docID := uuid.New(), uuid.New()
	name := uuid.NewString() + ".png"
	writeDocAsset(t, h, docID, name)

	base := "/api/documents/asset?doc=" + docID.String() + "&n=" + name
	cases := map[string]string{
		"forged signature": base + "&ws=" + wsID.String() + "&sig=deadbeef",
		"no signature":     base + "&ws=" + wsID.String(),
		"no workspace":     base + "&sig=" + h.signAsset(wsID, docAssetRel(docID, name)),
		"other workspace":  base + "&ws=" + uuid.New().String() + "&sig=" + h.signAsset(wsID, docAssetRel(docID, name)),
		"signature for another file": "/api/documents/asset?doc=" + docID.String() + "&ws=" + wsID.String() +
			"&n=" + name + "&sig=" + h.signAsset(wsID, docAssetRel(uuid.New(), name)),
	}
	resolve := h.inlineDocAsset()
	for label, src := range cases {
		if got := resolve(src); got != src {
			if strings.HasPrefix(got, "data:") {
				got = got[:strings.Index(got, ",")+1] + "…"
			}
			t.Errorf("%s: the file was inlined anyway: %q", label, got)
		}
	}
}
