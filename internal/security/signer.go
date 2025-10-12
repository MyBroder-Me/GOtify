package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

type Signer struct {
	Secret []byte
}

func (s *Signer) Generate(file string, ttl time.Duration) (token string, exp int64) {
	exp = time.Now().Add(ttl).Unix()
	msg := fmt.Sprintf("%s|%d", file, exp)
	mac := hmac.New(sha256.New, s.Secret)
	mac.Write([]byte(msg))
	token = hex.EncodeToString(mac.Sum(nil))
	return
}

func (s *Signer) Validate(file string, token string, exp int64) bool {
	if exp < time.Now().Unix() {
		return false
	}
	msg := fmt.Sprintf("%s|%d", file, exp)
	mac := hmac.New(sha256.New, s.Secret)
	mac.Write([]byte(msg))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(token), []byte(expected))
}
