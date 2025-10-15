package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	storage_go "github.com/supabase-community/storage-go"
	supabase "github.com/supabase-community/supabase-go"
)

type storageClient interface {
	UploadFile(bucketID, relativePath string, data io.Reader, fileOptions ...storage_go.FileOptions) (storage_go.FileUploadResponse, error)
	RemoveFile(bucketID string, paths []string) ([]storage_go.FileUploadResponse, error)
	DownloadFile(bucketID string, filePath string, urlOptions ...storage_go.UrlOptions) ([]byte, error)
	CreateSignedUrl(bucketId string, filePath string, expiresIn int) (storage_go.SignedUrlResponse, error)
}
type BucketClient struct {
	storage storageClient
	bucket  string
}

type UploadFile struct {
	Path        string
	Content     []byte
	ContentType string
}

func NewBucketClientFromEnv() (*BucketClient, error) {
	projectURL := strings.TrimRight(os.Getenv("SUPABASE_URL"), "/")
	serviceKey := strings.TrimSpace(os.Getenv("SUPABASE_SERVICE_KEY"))
	bucket := strings.TrimSpace(os.Getenv("SUPABASE_BUCKET"))

	if projectURL == "" || serviceKey == "" || bucket == "" {
		return nil, fmt.Errorf("supabase bucket env vars missing")
	}

	client, err := supabase.NewClient(projectURL, serviceKey, nil)
	if err != nil {
		return nil, err
	}

	return &BucketClient{
		storage: client.Storage,
		bucket:  bucket,
	}, nil
}

func (c *BucketClient) UploadBytes(_ context.Context, objectPath string, data []byte, contentType string) error {
	key := strings.TrimLeft(objectPath, "/")
	if key == "" {
		return fmt.Errorf("empty object path")
	}

	upsert := true
	opts := storage_go.FileOptions{Upsert: &upsert}
	if trimmedType := strings.TrimSpace(contentType); trimmedType != "" {
		opts.ContentType = &trimmedType
	}

	_, err := c.storage.UploadFile(c.bucket, key, bytes.NewReader(data), opts)
	return err
}

func (c *BucketClient) UploadBatch(ctx context.Context, prefix string, files []UploadFile) error {
	cleanPrefix := strings.Trim(prefix, "/")
	for _, file := range files {
		objectPath := file.Path
		if cleanPrefix != "" {
			objectPath = path.Join(cleanPrefix, objectPath)
		}
		if err := c.UploadBytes(ctx, objectPath, file.Content, file.ContentType); err != nil {
			return err
		}
	}
	return nil
}

func (c *BucketClient) DeletePrefix(_ context.Context, prefix string) error {
	clean := strings.Trim(prefix, "/")
	if clean == "" {
		return fmt.Errorf("cannot delete empty prefix")
	}
	_, err := c.storage.RemoveFile(c.bucket, []string{clean + "/"})
	return err
}

// DownloadFile recupera un objeto sin exponer la URL pública.
func (c *BucketClient) DownloadFile(objectPath string) ([]byte, error) {
	key := strings.TrimLeft(objectPath, "/")
	return c.storage.DownloadFile(c.bucket, key)
}

// SignedURL genera una URL temporal firmada para acceder al objeto directamente.
func (c *BucketClient) SignedURL(objectPath string, expiresIn int) (string, error) {
	key := strings.TrimLeft(objectPath, "/")
	if expiresIn <= 0 {
		expiresIn = 60
	}
	resp, err := c.storage.CreateSignedUrl(c.bucket, key, expiresIn)
	if err != nil {
		return "", err
	}
	return resp.SignedURL, nil
}
