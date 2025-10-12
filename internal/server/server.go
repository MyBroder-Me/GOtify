package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"customspotify/internal/handlers"
	"encoding/hex"
	"fmt"
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
}

func New(root string) *Server {
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

	// Routes
	hToken := handlers.NewTokenHandler(secret)
	hFile := handlers.NewFileHandler(root)
	r.GET("/token/:file", hToken.Generate)
	authorized := r.Group("/stream", AuthMiddleware(secret))
	{
		authorized.GET("/:file/*quality", hFile.Serve)
	}
	return &Server{engine: r, root: root}
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
		file := c.Param("file")

		et, _ := strconv.ParseInt(expires, 10, 64)
		if et < time.Now().Unix() {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		msg := fmt.Sprintf("%s|%s", file, expires)
		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte(msg))
		expected := hex.EncodeToString(mac.Sum(nil))

		if !hmac.Equal([]byte(token), []byte(expected)) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		c.Next()
	}
}
