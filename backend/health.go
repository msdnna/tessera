package main

import "github.com/gin-gonic/gin"

// healthHandler is a public probe used by clients (and later Android discovery)
// to confirm the server is "ours" via the app=tessera field.
func healthHandler(c *gin.Context) {
	c.JSON(200, gin.H{"ok": true, "app": "tessera"})
}
