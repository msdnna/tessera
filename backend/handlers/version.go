package handlers

import "github.com/gin-gonic/gin"

// VersionHandler serves the API version string.
type VersionHandler struct {
	version string
}

// NewVersionHandler returns a VersionHandler reporting the given version.
func NewVersionHandler(version string) *VersionHandler {
	return &VersionHandler{version: version}
}

// Get responds with the API version as JSON.
func (h *VersionHandler) Get(c *gin.Context) {
	c.JSON(200, gin.H{"api": h.version})
}
