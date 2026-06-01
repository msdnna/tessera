package handlers

import "github.com/gin-gonic/gin"

type VersionHandler struct {
	version string
}

func NewVersionHandler(version string) *VersionHandler {
	return &VersionHandler{version: version}
}

func (h *VersionHandler) Get(c *gin.Context) {
	c.JSON(200, gin.H{"api": h.version})
}
