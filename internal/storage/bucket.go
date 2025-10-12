package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

// BucketClient encapsula el acceso al bucket de Supabase Storage.
type BucketClient struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	bucket     string
}

// UploadFile describe un archivo listo para subir a Supabase Storage.
type UploadFile struct {
	Path        string
	Content     []byte
	ContentType string
}

type bucketConfig struct {
	BaseURL string
	APIKey  string
	Bucket  string
}

// NewBucketClientFromEnv inicializa el cliente usando variables de entorno SUPABASE_URL, SUPABASE_SERVICE_KEY y SUPABASE_BUCKET.
func NewBucketClientFromEnv() (*BucketClient, error) {
	cfg := bucketConfig{
		BaseURL: strings.TrimRight(os.Getenv("SUPABASE_URL"), "/"),
		APIKey:  strings.TrimSpace(os.Getenv("SUPABASE_SERVICE_KEY")),
		Bucket:  strings.TrimSpace(os.Getenv("SUPABASE_BUCKET")),
	}

	if cfg.BaseURL == "" || cfg.APIKey == "" || cfg.Bucket == "" {
		return nil, fmt.Errorf("supabase bucket env vars missing")
	}

	return &BucketClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    cfg.BaseURL,
		apiKey:     cfg.APIKey,
		bucket:     cfg.Bucket,
	}, nil
}

func (c *BucketClient) objectURL(objectPath string) (string, error) {
	trimmed := strings.TrimLeft(objectPath, "/")
	if trimmed == "" {
		return "", fmt.Errorf("empty object path")
	}
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	u.Path = path.Join(u.Path, "storage", "v1", "object", c.bucket, trimmed)
	return u.String(), nil
}

// UploadBytes sube un archivo binario al bucket con opción de upsert.
func (c *BucketClient) UploadBytes(ctx context.Context, objectPath string, data []byte, contentType string) error {
	url, err := c.objectURL(objectPath)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Apikey", c.apiKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "true")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return fmt.Errorf("supabase upload failed: %s %s", res.Status, string(body))
	}
	return nil
}

// UploadBatch sube múltiples archivos bajo un prefijo dado.
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

// DeletePrefix elimina recursivamente los objetos bajo un prefijo.
func (c *BucketClient) DeletePrefix(ctx context.Context, prefix string) error {
	clean := strings.Trim(prefix, "/")
	if clean == "" {
		return fmt.Errorf("cannot delete empty prefix")
	}

	u, err := url.Parse(c.baseURL)
	if err != nil {
		return err
	}
	u.Path = path.Join(u.Path, "storage", "v1", "object", c.bucket)

	payload := struct {
		Prefixes []string `json:"prefixes"`
	}{
		Prefixes: []string{clean + "/"},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Apikey", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		resBody, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return fmt.Errorf("supabase delete failed: %s %s", res.Status, string(resBody))
	}
	return nil
}
