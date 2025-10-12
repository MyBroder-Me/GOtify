package handlers

import (
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
	folder := c.Param("folder")
	file := c.Param("file")

	// If no file is specified, default to master.m3u8, else choose quality 64k, 128k or 192k
	if file == "" || file == "/" {
		file = "master"
	} else {
		file = strings.TrimPrefix(file, "/")
	}
	file = strings.TrimSuffix(file, ".m3u8") + ".m3u8"
	fullPath := filepath.Join(h.root, folder, file)
	c.File(fullPath)
}
