package handlers

import "github.com/gin-gonic/gin"

// VersionHandler serves the API build version and, when the binary was built
// with ldflags, its commit and build timestamp.
type VersionHandler struct {
	version string
	commit  string
	builtAt string
}

// NewVersionHandler returns a VersionHandler reporting the given version and
// optional build metadata (commit/builtAt may be empty in a dev build).
func NewVersionHandler(version, commit, builtAt string) *VersionHandler {
	return &VersionHandler{version: version, commit: commit, builtAt: builtAt}
}

// Get responds with the API version as JSON, including commit and build time
// when they were injected at build.
func (h *VersionHandler) Get(c *gin.Context) {
	body := gin.H{"api": h.version}
	if h.commit != "" {
		body["commit"] = h.commit
	}
	if h.builtAt != "" {
		body["built_at"] = h.builtAt
	}
	c.JSON(200, body)
}
