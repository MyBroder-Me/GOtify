package handlers

import (
	"GOtify/internal/security"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type TokenHandler struct {
	signer *security.Signer
}

func NewTokenHandler(secret []byte) *TokenHandler {
	return &TokenHandler{signer: &security.Signer{Secret: secret}}
}

func (h *TokenHandler) Generate(c *gin.Context) {
	file := c.Param("file")

	ttl := 10 * time.Minute
	if v := c.Query("ttl"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			ttl = time.Duration(n) * time.Minute
		}
	}

	token, exp := h.signer.Generate(file, ttl)
	c.JSON(http.StatusOK, gin.H{
		"file":    file,
		"expires": exp,
		"url":     "/stream/" + file + "?t=" + token + "&e=" + strconv.FormatInt(exp, 10),
	})
}
