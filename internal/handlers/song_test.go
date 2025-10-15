package handlers

import (
	"GOtify/internal/storage"
	"GOtify/internal/testutil/ffmpegstub"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSongHandlerCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := newFakeStore()
	bucket := &fakeBucket{}

	paths := ffmpegstub.Build(t)

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "audio.wav")
	if err := writeFile(sourcePath, []byte("audio")); err != nil {
		t.Fatalf("create source file: %v", err)
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read source file: %v", err)
	}

	cfg := SongHandlerConfig{
		BucketBaseURL:  "https://example.com/storage",
		FFmpegBin:      paths.FFmpeg,
		FFProbeBin:     paths.FFProbe,
		SegmentSeconds: 4,
	}

	handler, err := NewSongHandler(store, bucket, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fields := map[string]string{
		"name": "My Song",
	}

	code, resp := performMultipartRequest(t, handler.Create, http.MethodPost, "/songs", "/songs", fields, "file", "audio.wav", data)

	if code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d body=%s", code, resp)
	}

	var created songResponse
	if err := json.Unmarshal([]byte(resp), &created); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected generated id")
	}
	if created.Duration != 120 {
		t.Fatalf("expected duration 120, got %d", created.Duration)
	}

	if len(bucket.uploads) == 0 {
		t.Fatalf("expected upload call")
	}
	if bucket.uploads[0].prefix != "my-song" {
		t.Errorf("unexpected prefix: %s", bucket.uploads[0].prefix)
	}

	song, ok := store.songs[created.ID]
	if !ok {
		t.Fatalf("song not persisted")
	}
	if song.BucketFolder == "" {
		t.Errorf("bucket folder not set")
	}
	if song.DurationSeconds == 0 {
		t.Errorf("duration not populated")
	}
}

func TestSongHandlerUpdateRegeneratesAssets(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := newFakeStore()
	store.songs["song-1"] = storage.Song{
		ID:              "song-1",
		Name:            "Old Song",
		DurationSeconds: 200,
		BucketFolder:    "https://example.com/storage/old-song/master.m3u8",
	}

	bucket := &fakeBucket{}
	paths := ffmpegstub.Build(t)

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "audio.wav")
	if err := writeFile(sourcePath, []byte("audio")); err != nil {
		t.Fatalf("create source file: %v", err)
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read source file: %v", err)
	}

	handler, err := NewSongHandler(store, bucket, SongHandlerConfig{
		BucketBaseURL:  "https://example.com/storage",
		FFmpegBin:      paths.FFmpeg,
		FFProbeBin:     paths.FFProbe,
		SegmentSeconds: 4,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	existingBefore := store.songs["song-1"]

	fields := map[string]string{
		"name": "New Song",
	}

	code, _ := performMultipartRequest(t, handler.Update, http.MethodPut, "/songs/:id", "/songs/song-1", fields, "file", "audio.wav", data)

	if code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", code)
	}

	if len(bucket.deletes) != 1 || bucket.deletes[0] != "old-song" {
		t.Fatalf("expected delete of old folder, got %#v", bucket.deletes)
	}
	if len(bucket.uploads) == 0 {
		t.Fatalf("expected uploads for regenerated assets")
	}

	updated := store.songs["song-1"]
	if updated.Name != "New Song" {
		t.Errorf("unexpected name: %s", updated.Name)
	}
	if !strings.Contains(updated.BucketFolder, "new-song") {
		t.Errorf("bucket folder not updated: %s", updated.BucketFolder)
	}
	if updated.DurationSeconds == existingBefore.DurationSeconds {
		t.Errorf("duration not recalculated")
	}
}

func TestSongHandlerDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := newFakeStore()
	store.songs["song-1"] = storage.Song{
		ID:           "song-1",
		Name:         "Song",
		BucketFolder: "https://example.com/storage/song/master.m3u8",
	}

	bucket := &fakeBucket{}

	handler, err := NewSongHandler(store, bucket, SongHandlerConfig{
		BucketBaseURL: "https://example.com/storage",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	code, resp := performRequest(handler.Delete, http.MethodDelete, "/songs/:id", "/songs/song-1", nil)
	if len(bucket.deletes) == 0 {
		t.Fatalf("delete prefix not called")
	}
	if code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d body=%s", code, resp)
	}

	if len(bucket.deletes) != 1 || bucket.deletes[0] != "song" {
		t.Fatalf("expected delete call, got %#v", bucket.deletes)
	}
	if _, exists := store.songs["song-1"]; exists {
		t.Fatalf("song should be removed from store")
	}
}

// Helpers

type fakeStore struct {
	songs   map[string]storage.Song
	upserts []storage.Song
	lists   int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		songs: make(map[string]storage.Song),
	}
}

func (f *fakeStore) UpsertSong(_ context.Context, song storage.Song) error {
	f.songs[song.ID] = song
	f.upserts = append(f.upserts, song)
	return nil
}

func (f *fakeStore) GetSong(_ context.Context, id string) (storage.Song, error) {
	song, ok := f.songs[id]
	if !ok {
		return storage.Song{}, storage.ErrNotFound
	}
	return song, nil
}

func (f *fakeStore) ListSongs(_ context.Context) ([]storage.Song, error) {
	f.lists++
	result := make([]storage.Song, 0, len(f.songs))
	for _, song := range f.songs {
		result = append(result, song)
	}
	return result, nil
}

func (f *fakeStore) DeleteSong(_ context.Context, id string) error {
	if _, ok := f.songs[id]; !ok {
		return storage.ErrNotFound
	}
	delete(f.songs, id)
	return nil
}

type fakeBucket struct {
	uploads []struct {
		prefix string
		files  []storage.UploadFile
	}
	deletes []string
}

func (b *fakeBucket) UploadBatch(_ context.Context, prefix string, files []storage.UploadFile) error {
	b.uploads = append(b.uploads, struct {
		prefix string
		files  []storage.UploadFile
	}{prefix: prefix, files: files})
	return nil
}

func (b *fakeBucket) DeletePrefix(_ context.Context, prefix string) error {
	b.deletes = append(b.deletes, prefix)
	return nil
}

func performMultipartRequest(t *testing.T, handler gin.HandlerFunc, method, route, path string, fields map[string]string, fileField, fileName string, fileData []byte) (int, string) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write form field %s: %v", key, err)
		}
	}

	if fileField != "" && len(fileData) > 0 {
		part, err := writer.CreateFormFile(fileField, fileName)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write(fileData); err != nil {
			t.Fatalf("write file data: %v", err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(method, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	w := httptest.NewRecorder()
	router := gin.New()
	router.Handle(method, route, handler)
	router.ServeHTTP(w, req)

	result := w.Result()
	defer result.Body.Close()

	payload, _ := io.ReadAll(result.Body)
	return result.StatusCode, string(payload)
}

func performRequest(handler gin.HandlerFunc, method, route, path string, body any) (int, string) {
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router := gin.New()
	router.Handle(method, route, handler)
	router.ServeHTTP(w, req)
	result := w.Result()
	defer result.Body.Close()
	bodyBytes, _ := io.ReadAll(result.Body)
	return result.StatusCode, string(bodyBytes)
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
