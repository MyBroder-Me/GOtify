package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestFileHandlerServeDefaultMaster(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := t.TempDir()
	folder := "album"
	fullDir := filepath.Join(root, folder)
	if err := os.MkdirAll(fullDir, 0o755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	expected := "master playlist"
	if err := os.WriteFile(filepath.Join(fullDir, "master.m3u8"), []byte(expected), 0o644); err != nil {
		t.Fatalf("failed to write master file: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "file", Value: folder},
		{Key: "quality", Value: ""},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/stream/"+folder, nil)

	NewFileHandler(root).Serve(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if got := w.Body.String(); got != expected {
		t.Fatalf("expected body %q, got %q", expected, got)
	}
}

func TestFileHandlerServeVariant(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := t.TempDir()
	folder := "album"
	fullDir := filepath.Join(root, folder)
	if err := os.MkdirAll(fullDir, 0o755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	expected := "variant content"
	if err := os.WriteFile(filepath.Join(fullDir, "variant.m3u8"), []byte(expected), 0o644); err != nil {
		t.Fatalf("failed to write variant file: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "file", Value: folder},
		{Key: "quality", Value: "/variant"},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/stream/"+folder+"/variant", nil)

	NewFileHandler(root).Serve(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if got := w.Body.String(); got != expected {
		t.Fatalf("expected body %q, got %q", expected, got)
	}
}

func TestFileHandlerServeRejectTraversal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := t.TempDir()
	handler := NewFileHandler(root)

	cases := []struct {
		name    string
		file    string
		quality string
	}{
		{name: "folder traversal", file: "..", quality: ""},
		{name: "quality traversal", file: "album", quality: "/../secret"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{
				{Key: "file", Value: tc.file},
				{Key: "quality", Value: tc.quality},
			}
			c.Request = httptest.NewRequest(http.MethodGet, "/stream/"+tc.file, nil)

			handler.Serve(c)

			if w.Code != http.StatusForbidden {
				t.Fatalf("expected status 403, got %d", w.Code)
			}
		})
	}
}

func TestResolveFilename(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected string
	}{
		{name: "empty", raw: "", expected: "master.m3u8"},
		{name: "root slash", raw: "/", expected: "master.m3u8"},
		{name: "with extension", raw: "/variant.m3u8", expected: "variant.m3u8"},
		{name: "without extension", raw: "/variant", expected: "variant.m3u8"},
		{name: "other extension", raw: "/variant.ts", expected: "variant.m3u8"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveFilename(tc.raw); got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestRewritePlaylistAddsPrefixToVariantEntries(t *testing.T) {
	data := []byte("variant.m3u8\n")
	query := "t=abc&e=123"
	prefix := "album/"

	got := string(rewritePlaylist(data, query, prefix))
	want := "album/variant.m3u8?t=abc&e=123\n"

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestRewritePlaylistLeavesSegmentsWithoutPrefix(t *testing.T) {
	data := []byte("segment.ts\n")
	query := "t=abc&e=123"
	prefix := "album/"

	got := string(rewritePlaylist(data, query, prefix))
	want := "segment.ts?t=abc&e=123\n"

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
