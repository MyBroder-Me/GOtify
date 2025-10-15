package storage

import (
	"context"
	"io"
	"strings"
	"testing"

	storage_go "github.com/supabase-community/storage-go"
)

func TestNewBucketClientFromEnvMissing(t *testing.T) {
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("SUPABASE_SERVICE_KEY", "")
	t.Setenv("SUPABASE_BUCKET", "")

	if _, err := NewBucketClientFromEnv(); err == nil {
		t.Fatalf("expected error when env vars are missing")
	}
}

type fakeStorage struct {
	uploads []struct {
		bucket string
		path   string
		body   []byte
		opts   []storage_go.FileOptions
	}
	deletions [][]string
}

func (f *fakeStorage) UploadFile(bucketID, relativePath string, data io.Reader, fileOptions ...storage_go.FileOptions) (storage_go.FileUploadResponse, error) {
	body, err := io.ReadAll(data)
	if err != nil {
		return storage_go.FileUploadResponse{}, err
	}
	f.uploads = append(f.uploads, struct {
		bucket string
		path   string
		body   []byte
		opts   []storage_go.FileOptions
	}{
		bucket: bucketID,
		path:   relativePath,
		body:   body,
		opts:   append([]storage_go.FileOptions{}, fileOptions...),
	})
	return storage_go.FileUploadResponse{}, nil
}

func (f *fakeStorage) RemoveFile(bucketID string, paths []string) ([]storage_go.FileUploadResponse, error) {
	f.deletions = append(f.deletions, append([]string{}, paths...))
	return nil, nil
}

func TestBucketClientUploadAndDelete(t *testing.T) {
	fake := &fakeStorage{}
	client := &BucketClient{
		storage: fake,
		bucket:  "audio",
	}

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

	if len(fake.uploads) != 3 {
		t.Fatalf("expected 3 uploads, got %d", len(fake.uploads))
	}

	for _, upload := range fake.uploads {
		if upload.bucket != "audio" {
			t.Errorf("unexpected bucket %s", upload.bucket)
		}
		if !strings.HasPrefix(upload.path, "song/") {
			t.Errorf("expected upload path under prefix, got %s", upload.path)
		}
		var foundUpsert bool
		for _, opt := range upload.opts {
			if opt.Upsert != nil && *opt.Upsert {
				foundUpsert = true
			}
			if opt.ContentType != nil && len(*opt.ContentType) == 0 {
				t.Errorf("content type option should not be empty")
			}
		}
		if !foundUpsert {
			t.Errorf("upload should set upsert")
		}
	}

	if len(fake.deletions) != 1 {
		t.Fatalf("expected one deletion call, got %d", len(fake.deletions))
	}
	expectedDelete := []string{"song/"}
	if !equalStringSlices(fake.deletions[0], expectedDelete) {
		t.Errorf("unexpected delete payload: %v", fake.deletions[0])
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
