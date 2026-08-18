package main

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var rawVersion string

var appVersion = strings.TrimSpace(rawVersion)

// Build metadata, injected at image-build time via
//   -ldflags "-X main.commit=<sha> -X main.builtAt=<rfc3339>"
// (see backend/Dockerfile). Both are empty in a plain `go run` dev build, in
// which case /api/version omits them.
var (
	commit  string
	builtAt string
)
