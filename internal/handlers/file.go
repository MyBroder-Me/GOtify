package handlers

import (
	"bufio"
	"bytes"
	"net/http"
	"os"
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
	trimmed := strings.TrimPrefix(raw, "/")

	if strings.Contains(folder, "..") || strings.Contains(trimmed, "..") {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	prefix := ""
	if folder != "" {
		prefix = folder + "/"
	}

	if trimmed == "" {
		filename := "master.m3u8"
		fullPath := filepath.Join(h.root, folder, filename)
		if err := h.servePlaylist(c, fullPath, c.Request.URL.RawQuery, prefix); err != nil {
			c.AbortWithStatus(http.StatusNotFound)
		}
		return
	}

	if strings.HasSuffix(strings.ToLower(trimmed), ".m3u8") || !strings.Contains(trimmed, ".") {
		filename := resolveFilename(raw)
		fullPath := filepath.Join(h.root, folder, filename)
		if err := h.servePlaylist(c, fullPath, c.Request.URL.RawQuery, prefix); err != nil {
			c.AbortWithStatus(http.StatusNotFound)
		}
		return
	}

	fullPath := filepath.Join(h.root, folder, trimmed)
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

func (h *FileHandler) servePlaylist(c *gin.Context, path string, query string, prefix string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if query == "" {
		c.Data(http.StatusOK, "application/vnd.apple.mpegurl", data)
		return nil
	}

	rewritten := rewritePlaylist(data, query, prefix)
	c.Data(http.StatusOK, "application/vnd.apple.mpegurl", rewritten)
	return nil
}

func rewritePlaylist(data []byte, query string, prefix string) []byte {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var builder strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		hasCR := strings.HasSuffix(line, "\r")
		content := line
		if hasCR {
			content = strings.TrimSuffix(line, "\r")
		}

		trimmed := strings.TrimSpace(content)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			rewritten := trimmed

			if prefix != "" && needsRelativePrefix(trimmed, prefix) {
				if idx := strings.Index(rewritten, "?"); idx != -1 {
					rewritten = prefix + rewritten[:idx] + rewritten[idx:]
				} else {
					rewritten = prefix + rewritten
				}
			}

			if strings.Contains(rewritten, "?") {
				rewritten += "&" + query
			} else {
				rewritten += "?" + query
			}

			if trimmed != content {
				if start := strings.Index(content, trimmed); start != -1 {
					leading := content[:start]
					trailing := content[start+len(trimmed):]
					content = leading + rewritten + trailing
				} else {
					content = rewritten
				}
			} else {
				content = rewritten
			}
		}

		if hasCR {
			content += "\r"
		}

		builder.WriteString(content)
		builder.WriteByte('\n')
	}

	if err := scanner.Err(); err != nil {
		return data
	}

	return []byte(builder.String())
}

func needsRelativePrefix(value string, prefix string) bool {
	if prefix == "" {
		return false
	}

	base := value
	if idx := strings.IndexAny(base, "?#"); idx != -1 {
		base = base[:idx]
	}

	lowerBase := strings.ToLower(base)
	if strings.HasPrefix(base, "/") || strings.HasPrefix(base, prefix) {
		return false
	}
	if strings.HasPrefix(lowerBase, "http://") || strings.HasPrefix(lowerBase, "https://") || strings.HasPrefix(lowerBase, "data:") {
		return false
	}
	if strings.HasPrefix(base, "//") {
		return false
	}
	if strings.Contains(base, "/") {
		return false
	}
	return strings.HasSuffix(strings.ToLower(base), ".m3u8")
}
