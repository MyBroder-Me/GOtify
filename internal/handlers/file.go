package handlers

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path"
	"strings"

	"GOtify/internal/storage"

	"github.com/gin-gonic/gin"
)

type songLoader interface {
	GetSong(ctx context.Context, id string) (storage.Song, error)
}

type bucketDownloader interface {
	DownloadFile(objectPath string) ([]byte, error)
	SignedURL(objectPath string, expiresIn int) (string, error)
}

type FileHandler struct {
	store      songLoader
	bucket     bucketDownloader
	bucketName string
}

func NewFileHandler(store songLoader, bucket bucketDownloader, bucketName string) *FileHandler {
	return &FileHandler{
		store:      store,
		bucket:     bucket,
		bucketName: strings.TrimSpace(bucketName),
	}
}

func (h *FileHandler) Serve(c *gin.Context) {
	songID := c.Param("file_id")
	log.Println("Requested song ID:", songID)
	if songID == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	rawQuality := c.Param("quality")
	trimmedQuality := strings.TrimPrefix(rawQuality, "/")
	if strings.Contains(songID, "..") || strings.Contains(trimmedQuality, "..") {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	song, err := h.store.GetSong(c.Request.Context(), songID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, storage.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.AbortWithStatus(status)
		return
	}

	masterKey, err := h.masterObjectKey(song)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	objectKey, err := resolveObjectKey(masterKey, rawQuality)
	if err != nil {
		if errors.Is(err, errInvalidObjectKey) {
			c.AbortWithStatus(http.StatusForbidden)
		} else {
			c.AbortWithStatus(http.StatusInternalServerError)
		}
		return
	}

	if strings.HasSuffix(strings.ToLower(objectKey), ".m3u8") {
		data, err := h.bucket.DownloadFile(objectKey)
		if err != nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		h.servePlaylist(c, data, c.Request.URL.RawQuery)
		return
	}

	const signedTTLSeconds = 60
	signedURL, err := h.bucket.SignedURL(objectKey, signedTTLSeconds)
	log.Println("Signed url:", signedURL)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, signedURL)
}

func (h *FileHandler) masterObjectKey(song storage.Song) (string, error) {
	key := strings.TrimSpace(song.BucketFolder)
	if key == "" {
		return "", fmt.Errorf("bucket path missing")
	}

	if idx := strings.Index(key, "?"); idx != -1 {
		key = key[:idx]
	}
	key = strings.ReplaceAll(key, "\\", "/")

	lower := strings.ToLower(key)
	if strings.Contains(lower, "/storage/v1/object/") {
		parts := strings.SplitN(key, "/storage/v1/object/", 2)
		if len(parts) == 2 {
			key = parts[1]
		}
	}

	key = strings.TrimPrefix(key, "/")
	key = strings.TrimPrefix(key, "public/")

	if h.bucketName != "" {
		prefix := strings.Trim(h.bucketName, "/")
		if prefix != "" && strings.HasPrefix(key, prefix+"/") {
			key = key[len(prefix)+1:]
		}
	}

	if key == "" || strings.Contains(key, "..") {
		return "", fmt.Errorf("invalid bucket path")
	}

	if !strings.HasSuffix(strings.ToLower(key), ".m3u8") {
		return path.Join(key, "master.m3u8"), nil
	}
	return key, nil
}

var errInvalidObjectKey = errors.New("invalid object key")

func resolveObjectKey(masterKey string, rawQuality string) (string, error) {
	baseDir := path.Dir(masterKey)

	raw := strings.TrimPrefix(rawQuality, "/")
	if raw == "" || raw == "/" {
		return masterKey, nil
	}

	if strings.Contains(raw, "..") {
		return "", errInvalidObjectKey
	}

	lower := strings.ToLower(raw)
	var target string
	if strings.HasSuffix(lower, ".m3u8") || !strings.Contains(raw, ".") {
		target = resolveFilename(rawQuality)
	} else {
		target = raw
	}

	clean := path.Clean(path.Join(baseDir, target))
	if strings.HasPrefix(clean, "../") || clean == ".." {
		return "", errInvalidObjectKey
	}
	return clean, nil
}

func (h *FileHandler) servePlaylist(c *gin.Context, data []byte, query string) {
	if query == "" {
		c.Data(http.StatusOK, "application/vnd.apple.mpegurl", data)
		return
	}
	rewritten := rewritePlaylist(data, query)
	c.Data(http.StatusOK, "application/vnd.apple.mpegurl", rewritten)
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

func rewritePlaylist(data []byte, query string) []byte {
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
