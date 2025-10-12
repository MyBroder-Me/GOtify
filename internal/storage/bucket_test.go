package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewBucketClientFromEnvMissing(t *testing.T) {
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("SUPABASE_SERVICE_KEY", "")
	t.Setenv("SUPABASE_BUCKET", "")

	if _, err := NewBucketClientFromEnv(); err == nil {
		t.Fatalf("expected error when env vars are missing")
	}
}

func TestBucketClientUploadAndDelete(t *testing.T) {
	type record struct {
		method string
		path   string
		body   []byte
		header http.Header
	}
	reqCh := make(chan record, 4)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		reqCh <- record{
			method: r.Method,
			path:   r.URL.Path,
			body:   body,
			header: r.Header.Clone(),
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("SUPABASE_URL", server.URL)
	t.Setenv("SUPABASE_SERVICE_KEY", "test-key")
	t.Setenv("SUPABASE_BUCKET", "audio")

	client, err := NewBucketClientFromEnv()
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}
	client.httpClient = server.Client()

	ctx := context.Background()

	if err := client.UploadBytes(ctx, "song/master.m3u8", []byte("#EXTM3U"), "application/vnd.apple.mpegurl"); err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	files := []UploadFile{
		{Path: "master.m3u8", Content: []byte("playlist"), ContentType: "application/vnd.apple.mpegurl"},
		{Path: "segment_000.ts", Content: []byte("segment"), ContentType: "video/mp2t"},
	}
	if err := client.UploadBatch(ctx, "song", files); err != nil {
		t.Fatalf("upload batch failed: %v", err)
	}

	if err := client.DeletePrefix(ctx, "song"); err != nil {
		t.Fatalf("delete prefix failed: %v", err)
	}

	close(reqCh)

	var uploads []record
	var deleteReq *record
	for rec := range reqCh {
		switch rec.method {
		case http.MethodPost:
			if rec.path == "/storage/v1/object/audio" {
				copy := rec
				deleteReq = &copy
				continue
			}
			uploads = append(uploads, rec)
		default:
			t.Errorf("unexpected method: %s", rec.method)
		}
	}

	if len(uploads) < 3 {
		t.Fatalf("expected multiple upload requests, got %d", len(uploads))
	}
	if deleteReq == nil {
		t.Errorf("expected delete request to be sent")
	} else {
		expected := `{"prefixes":["song/"]}`
		if !bytes.Equal(deleteReq.body, []byte(expected)) {
			t.Errorf("unexpected delete payload: %s", deleteReq.body)
		}
	}

	foundMaster := false
	for _, upload := range uploads {
		switch upload.path {
		case "/storage/v1/object/audio/song/master.m3u8":
			if !foundMaster {
				foundMaster = true
				if !bytes.Equal(upload.body, []byte("#EXTM3U")) && !bytes.Equal(upload.body, []byte("playlist")) {
					t.Errorf("unexpected master body: %s", upload.body)
				}
				if upload.header.Get("Content-Type") == "" {
					t.Errorf("content type should be set for master upload")
				}
			}
		}
	}
}
