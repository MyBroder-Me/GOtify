package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRequireSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		configSecret string
		headerSecret string
		query        string
		wantStatus   int
	}{
		{
			name:         "rejects when secret not configured",
			configSecret: "",
			wantStatus:   http.StatusInternalServerError,
		},
		{
			name:         "rejects when secret missing",
			configSecret: "super-secret",
			wantStatus:   http.StatusUnauthorized,
		},
		{
			name:         "rejects when secret incorrect",
			configSecret: "super-secret",
			headerSecret: "wrong",
			wantStatus:   http.StatusUnauthorized,
		},
		{
			name:         "accepts valid header secret",
			configSecret: "super-secret",
			headerSecret: "super-secret",
			wantStatus:   http.StatusOK,
		},
		{
			name:         "trims header value",
			configSecret: "super-secret",
			headerSecret: "  super-secret  ",
			wantStatus:   http.StatusOK,
		},
		{
			name:         "rejects when secret only in query",
			configSecret: "super-secret",
			query:        "secret=super-secret",
			wantStatus:   http.StatusUnauthorized,
		},
		{
			name:         "rejects when secret only in query even with spaces",
			configSecret: "super-secret",
			query:        "secret=%20super-secret%20",
			wantStatus:   http.StatusUnauthorized,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			middleware := RequireSecret(tc.configSecret)

			resp := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(resp)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			if tc.query != "" {
				ctx.Request.URL.RawQuery = tc.query
			}
			if tc.headerSecret != "" {
				ctx.Request.Header.Set("X-API-Key", tc.headerSecret)
			}

			middleware(ctx)

			if resp.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d", tc.wantStatus, resp.Code)
			}
		})
	}
}

func TestAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	secret := []byte("super-secret")
	router := gin.New()
	router.GET("/stream/:file/*quality", AuthMiddleware(secret), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	file := "song"
	basePath := "/stream/" + file + "/master"

	futureExp := time.Now().Add(2 * time.Minute).Unix()
	futureExpStr := strconv.FormatInt(futureExp, 10)
	validToken := buildToken(secret, file, futureExpStr)

	pastExp := time.Now().Add(-time.Minute).Unix()
	pastExpStr := strconv.FormatInt(pastExp, 10)
	expiredToken := buildToken(secret, file, pastExpStr)

	tests := []struct {
		name       string
		queryParts []string
		wantStatus int
	}{
		{
			name:       "rejects when token missing",
			queryParts: []string{"e=" + futureExpStr},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "rejects when expiry missing",
			queryParts: []string{"t=" + validToken},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "rejects when token invalid",
			queryParts: []string{"t=invalid", "e=" + futureExpStr},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "rejects when token expired",
			queryParts: []string{"t=" + expiredToken, "e=" + pastExpStr},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "accepts when token and expiry valid",
			queryParts: []string{"t=" + validToken, "e=" + futureExpStr},
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			target := basePath
			if len(tc.queryParts) > 0 {
				target = target + "?" + strings.Join(tc.queryParts, "&")
			}

			req := httptest.NewRequest(http.MethodGet, target, nil)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d", tc.wantStatus, resp.Code)
			}
		})
	}
}

func buildToken(secret []byte, file string, expires string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(fmt.Sprintf("%s|%s", file, expires)))
	return hex.EncodeToString(mac.Sum(nil))
}
