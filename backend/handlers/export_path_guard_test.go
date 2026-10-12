package handlers

import (
	"context"
	"strings"
	"testing"

	"tessera/internal/db"
)

// The export file name takes its extension from exportFormat, never from the
// request: only the two known formats map, and they map onto constants.
func TestExportFormatOnlyKnownExtensions(t *testing.T) {
	for in, want := range map[string]string{"csv": "csv", "json": "json"} {
		if got, ok := exportFormat(in); !ok || got != want {
			t.Errorf("exportFormat(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "CSV", "xlsx", "csv/../../x", "../json", "json\x00"} {
		if got, ok := exportFormat(in); ok {
			t.Errorf("exportFormat(%q) = %q, true; want refusal", in, got)
		}
	}
}

// mirrorAsset opens a file under the media dir only for a plain media name; a
// ref that would climb out of it is refused before any lookup or open (h.q is
// nil here — reaching the database would panic).
func TestMirrorAssetRefusesNonMediaName(t *testing.T) {
	h := &API{uploadDir: t.TempDir()}
	for _, ref := range []string{
		"/api/uploads/../../etc/passwd",
		"/api/uploads/..",
		"/api/uploads/a/b.png",
		"/api/uploads/",
	} {
		_, err := h.mirrorAsset(context.Background(), nil, db.GitlabIntegration{}, ref)
		if err == nil || !strings.Contains(err.Error(), "not a media upload name") {
			t.Errorf("mirrorAsset(%q) err = %v; want the media-name refusal", ref, err)
		}
	}
}
