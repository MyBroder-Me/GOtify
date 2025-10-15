package handlers

import (
	"GOtify/internal/security"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type tokenResponse struct {
	FileID  string `json:"file_id"`
	Expires int64  `json:"expires"`
	URL     string `json:"url"`
}

func TestTokenHandlerGenerateDefaultTTL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	secret := []byte("super-secret")
	handler := NewTokenHandler(secret)

	router := gin.New()
	router.GET("/token/:file_id", handler.Generate)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/token/track", nil)

	before := time.Now()
	router.ServeHTTP(rec, req)
	after := time.Now()

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body tokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body.FileID != "track" {
		t.Fatalf("expected file 'track', got %q", body.FileID)
	}

	assertExpiryInRange(t, body.Expires, before.Add(9*time.Minute), after.Add(10*time.Minute+time.Second))
	assertURLMatchesToken(t, body, secret)
}

func TestTokenHandlerGenerateCustomTTL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	secret := []byte("super-secret")
	handler := NewTokenHandler(secret)

	router := gin.New()
	router.GET("/token/:file_id", handler.Generate)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/token/track?ttl=2", nil)

	before := time.Now()
	router.ServeHTTP(rec, req)
	after := time.Now()

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body tokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	assertExpiryInRange(t, body.Expires, before.Add(119*time.Second), after.Add(2*time.Minute+time.Second))
	assertURLMatchesToken(t, body, secret)
}

func assertExpiryInRange(t *testing.T, exp int64, min time.Time, max time.Time) {
	t.Helper()

	expTime := time.Unix(exp, 0)
	if expTime.Before(min) || expTime.After(max) {
		t.Fatalf("expected expiration between %v and %v, got %v", min, max, expTime)
	}
}

func assertURLMatchesToken(t *testing.T, body tokenResponse, secret []byte) {
	t.Helper()

	parsed, err := url.Parse(body.URL)
	if err != nil {
		t.Fatalf("failed to parse url %q: %v", body.URL, err)
	}

	if parsed.Path != "/stream/"+body.FileID {
		t.Fatalf("expected stream path for %q, got %q", body.FileID, parsed.Path)
	}

	values := parsed.Query()
	token := values.Get("t")
	expiresStr := values.Get("e")
	if expiresStr != strconv.FormatInt(body.Expires, 10) {
		t.Fatalf("expected expires string %s, got %s", strconv.FormatInt(body.Expires, 10), expiresStr)
	}

	exp, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		t.Fatalf("failed to parse expires %q: %v", expiresStr, err)
	}

	s := &security.Signer{Secret: secret}
	if !s.Validate(body.FileID, token, exp) {
		t.Fatal("token in URL does not validate")
	}
}
