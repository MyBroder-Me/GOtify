package server

import (
	"GOtify/internal/handlers"
	"GOtify/internal/storage"
	"GOtify/internal/transcode"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/didip/tollbooth/v7"
	"github.com/didip/tollbooth_gin"
	"github.com/gin-gonic/gin"
)

type Server struct {
	engine *gin.Engine
	root   string
	store  *storage.Store
	bucket *storage.BucketClient
}

func New(root string) *Server {
	ctx := context.Background()
	store, err := storage.NewStore(ctx)
	if err != nil {
		panic(err)
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	secretValue := strings.TrimSpace(os.Getenv("SECRET"))
	secret := []byte(secretValue)

	// Headers
	r.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Cache-Control", "private, max-age=600")
		c.Next()
	})

	// API key enforcement
	r.Use(RequireSecret(secretValue))

	// Rate limiting
	limiter := tollbooth.NewLimiter(10, nil) // 10 req/s
	r.Use(tollbooth_gin.LimitHandler(limiter))

	// Handlers
	hToken := handlers.NewTokenHandler(secret)
	bucketClient, err := storage.NewBucketClientFromEnv()
	if err != nil {
		panic(err)
	}
	bucketName := strings.TrimSpace(os.Getenv("SUPABASE_BUCKET"))
	hFile := handlers.NewFileHandler(store, bucketClient, bucketName)
	segmentSeconds := parseSegmentSeconds(os.Getenv("HLS_SEGMENT_SECONDS"))
	variantCfg := parseVariantConfig(os.Getenv("HLS_AUDIO_VARIANTS"))
	handlerCfg := handlers.SongHandlerConfig{
		BucketBaseURL:  strings.TrimSpace(os.Getenv("SUPABASE_BUCKET_PUBLIC_URL")),
		FFmpegBin:      strings.TrimSpace(os.Getenv("FFMPEG_BIN")),
		FFProbeBin:     strings.TrimSpace(os.Getenv("FFPROBE_BIN")),
		SegmentSeconds: segmentSeconds,
		Variants:       variantCfg,
	}
	hSong, err := handlers.NewSongHandler(store, bucketClient, handlerCfg)
	if err != nil {
		panic(err)
	}

	// Routes
	r.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.GET("/token/:file_id", hToken.Generate)
	authorized := r.Group("/stream", AuthMiddleware(secret))
	{
		authorized.GET("/:file_id/*quality", hFile.Serve)
	}
	r.POST("/songs", hSong.Create)
	r.GET("/songs", hSong.List)
	r.GET("/songs/:id", hSong.Get)
	r.PUT("/songs/:id", hSong.Update)
	r.DELETE("/songs/:id", hSong.Delete)
	return &Server{engine: r, root: root, store: store, bucket: bucketClient}
}

func (s *Server) Run(addr string) {
	if err := s.engine.Run(addr); err != nil {
		panic(err)
	}
}

func RequireSecret(secret string) gin.HandlerFunc {
	secretBytes := []byte(secret)
	return func(c *gin.Context) {
		if len(secretBytes) == 0 {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		input := strings.TrimSpace(c.GetHeader("X-API-Key"))

		if input == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if subtle.ConstantTimeCompare([]byte(input), secretBytes) != 1 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}

func AuthMiddleware(secret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Query("t")
		expires := c.Query("e")
		file := c.Param("file_id")
		if file == "" {
			file = c.Param("file")
		}

		et, _ := strconv.ParseInt(expires, 10, 64)
		if et < time.Now().Unix() {
			log.Println("Token expired:", et, "<", time.Now().Unix())
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		msg := fmt.Sprintf("%s|%s", file, expires)
		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte(msg))
		expected := hex.EncodeToString(mac.Sum(nil))

		if !hmac.Equal([]byte(token), []byte(expected)) {
			log.Println("Invalid token:", token, "!=", expected, "for msg:", msg)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		c.Next()
	}
}

func parseSegmentSeconds(value string) int {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	seconds, err := strconv.Atoi(trimmed)
	if err != nil || seconds <= 0 {
		return 0
	}
	return seconds
}

func parseVariantConfig(value string) []transcode.Variant {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, ",")
	var variants []transcode.Variant
	seen := map[int]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		num, err := strconv.Atoi(part)
		if err != nil || num <= 0 {
			continue
		}
		if seen[num] {
			continue
		}
		seen[num] = true
		variants = append(variants, transcode.Variant{
			Name:        fmt.Sprintf("%dk", num),
			BitrateKbps: num,
		})
	}
	return variants
}
