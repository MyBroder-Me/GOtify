package handlers

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

type FileHandler struct {
	root string
}

func NewFileHandler(root string) *FileHandler {
	return &FileHandler{root: root}
}

func (h *FileHandler) Serve(c *gin.Context) {
	folder := c.Param("file")
	raw := c.Param("quality")

	filename := resolveFilename(raw)

	if strings.Contains(folder, "..") || strings.Contains(filename, "..") {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	fullPath := filepath.Join(h.root, folder, filename)
	c.File(fullPath)
}

func resolveFilename(raw string) string {
	if raw == "" || raw == "/" {
		return "master.m3u8"
	}

	name := strings.TrimPrefix(raw, "/")

	if i := strings.LastIndex(name, "."); i != -1 {
		name = name[:i]
	}

	return name + ".m3u8"
}