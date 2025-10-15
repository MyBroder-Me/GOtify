package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"GOtify/internal/storage"

	"github.com/gin-gonic/gin"
)

func TestFileHandlerServeDefaultMaster(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &fakeSongStore{
		songs: map[string]storage.Song{
			"song-1": {
				ID:           "song-1",
				Name:         "Song",
				BucketFolder: "my-song",
			},
		},
	}

	bucket := &fakeDownloadBucket{
		files: map[string][]byte{
			"my-song/master.m3u8": []byte("master playlist"),
		},
	}

	handler := NewFileHandler(store, bucket, "music")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "file_id", Value: "song-1"},
		{Key: "quality", Value: ""},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/stream/song-1", nil)

	handler.Serve(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if got := w.Body.String(); got != "master playlist" {
		t.Fatalf("unexpected body: %q", got)
	}
}

func TestFileHandlerServeVariant(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &fakeSongStore{
		songs: map[string]storage.Song{
			"song-1": {
				ID:           "song-1",
				Name:         "Song",
				BucketFolder: "my-song",
			},
		},
	}

	bucket := &fakeDownloadBucket{
		files: map[string][]byte{
			"my-song/variant.m3u8": []byte("variant content"),
		},
	}

	handler := NewFileHandler(store, bucket, "music")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "file_id", Value: "song-1"},
		{Key: "quality", Value: "/variant"},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/stream/song-1/variant", nil)

	handler.Serve(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if got := w.Body.String(); got != "variant content" {
		t.Fatalf("unexpected body: %q", got)
	}
}

func TestFileHandlerServeRejectTraversal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &fakeSongStore{
		songs: map[string]storage.Song{
			"song-1": {
				ID:           "song-1",
				Name:         "Song",
				BucketFolder: "my-song",
			},
		},
	}
	bucket := &fakeDownloadBucket{files: map[string][]byte{}}
	handler := NewFileHandler(store, bucket, "music")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "file_id", Value: "song-1"},
		{Key: "quality", Value: "/../../secret"},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/stream/song-1/../../secret", nil)

	handler.Serve(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", w.Code)
	}
}

func TestFileHandlerServeSegmentRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &fakeSongStore{
		songs: map[string]storage.Song{
			"song-1": {
				ID:           "song-1",
				Name:         "Song",
				BucketFolder: "my-song",
			},
		},
	}
	signed := "https://signed.example/my-song/segment_000.ts"
	bucket := &fakeDownloadBucket{
		signed: map[string]string{
			"my-song/segment_000.ts": signed,
		},
	}
	handler := NewFileHandler(store, bucket, "music")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "file_id", Value: "song-1"},
		{Key: "quality", Value: "/segment_000.ts"},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/stream/song-1/segment_000.ts", nil)

	handler.Serve(c)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != signed {
		t.Fatalf("unexpected redirect location: %s", loc)
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

	got := string(rewritePlaylist(data, query))
	want := "variant.m3u8?t=abc&e=123\n"

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

type fakeSongStore struct {
	songs map[string]storage.Song
}

func (f *fakeSongStore) GetSong(_ context.Context, id string) (storage.Song, error) {
	song, ok := f.songs[id]
	if !ok {
		return storage.Song{}, storage.ErrNotFound
	}
	return song, nil
}

type fakeDownloadBucket struct {
	files  map[string][]byte
	signed map[string]string
}

func (b *fakeDownloadBucket) DownloadFile(objectPath string) ([]byte, error) {
	data, ok := b.files[objectPath]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return data, nil
}

func (b *fakeDownloadBucket) SignedURL(objectPath string, expiresIn int) (string, error) {
	if b.signed != nil {
		if url, ok := b.signed[objectPath]; ok {
			return url, nil
		}
	}
	return fmt.Sprintf("https://signed.test/%s?ttl=%d", objectPath, expiresIn), nil
}
