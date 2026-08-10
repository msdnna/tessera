package handlers

import (
	"bytes"
	"mime/multipart"
	"testing"
)

// formFile builds the multipart.FileHeader an upload handler would receive,
// including the client-declared filename that sniffContentType must ignore.
func formFile(t *testing.T, filename string, body []byte) *multipart.FileHeader {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(body); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	_ = w.Close()

	form, err := multipart.NewReader(&buf, w.Boundary()).ReadForm(1 << 20)
	if err != nil {
		t.Fatalf("read form: %v", err)
	}
	return form.File["file"][0]
}

func TestSniffContentTypeGatesOnBytes(t *testing.T) {
	t.Parallel()

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	tests := []struct {
		name, filename string
		body           []byte
		wantAllowed    bool
		wantExt        string
	}{
		{name: "png", filename: "a.png", body: []byte("\x89PNG\r\n\x1a\n----"), wantAllowed: true, wantExt: ".png"},
		{name: "jpeg", filename: "a.jpg", body: []byte("\xff\xd8\xff----"), wantAllowed: true, wantExt: ".jpg"},
		{name: "gif", filename: "a.gif", body: []byte("GIF89a----"), wantAllowed: true, wantExt: ".gif"},
		{name: "bmp", filename: "a.bmp", body: []byte("BM------"), wantAllowed: true, wantExt: ".bmp"},
		{name: "webp", filename: "a.webp", body: []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), wantAllowed: true, wantExt: ".webp"},
		// The extension follows the bytes, so a real PNG named .svg is stored as .png.
		{name: "png named svg", filename: "a.svg", body: []byte("\x89PNG\r\n\x1a\n----"), wantAllowed: true, wantExt: ".png"},
		{name: "svg", filename: "a.svg", body: svg},
		{name: "svg named png", filename: "a.png", body: svg},
		{name: "html named png", filename: "a.png", body: []byte("<!DOCTYPE html><script>alert(1)</script>")},
		{name: "empty", filename: "a.png", body: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ct, err := sniffContentType(formFile(t, tc.filename, tc.body))
			if err != nil {
				t.Fatalf("sniff: %v", err)
			}
			ext, ok := mediaExts[ct]
			if ok != tc.wantAllowed {
				t.Fatalf("%q sniffed as %q: allowed=%v, want %v", tc.filename, ct, ok, tc.wantAllowed)
			}
			if ok && ext != tc.wantExt {
				t.Fatalf("%q sniffed as %q → ext %q, want %q", tc.filename, ct, ext, tc.wantExt)
			}
		})
	}
}

// Whatever uploads accept must be exactly what the public route will render
// inline — the two sets are wired together so they can't drift apart.
func TestInlineSafeExtsMatchesMediaExts(t *testing.T) {
	t.Parallel()

	if inlineSafeExts[".svg"] {
		t.Fatal(".svg is inline-safe; legacy SVGs would still render on our origin")
	}
	for _, ext := range mediaExts {
		if !inlineSafeExts[ext] {
			t.Fatalf("%s is accepted on upload but not served inline", ext)
		}
	}
	if len(inlineSafeExts) != len(mediaExts) {
		t.Fatalf("inlineSafeExts has %d entries, mediaExts %d", len(inlineSafeExts), len(mediaExts))
	}
}
