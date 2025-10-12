package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

func TestSignerGenerateAndValidate(t *testing.T) {
	s := &Signer{Secret: []byte("super-secret")}
	token, exp := s.Generate("song", time.Minute)

	if exp <= time.Now().Unix() {
		t.Fatalf("expected expiration in the future, got %d", exp)
	}

	if !s.Validate("song", token, exp) {
		t.Fatal("expected token to validate")
	}
}

func TestSignerValidateExpired(t *testing.T) {
	s := &Signer{Secret: []byte("super-secret")}
	exp := time.Now().Add(-time.Second).Unix()

	token := manualToken(s.Secret, "song", exp)
	if s.Validate("song", token, exp) {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestSignerValidateMismatchedToken(t *testing.T) {
	s := &Signer{Secret: []byte("super-secret")}
	token, exp := s.Generate("song", time.Minute)

	if s.Validate("other-song", token, exp) {
		t.Fatal("expected token for another file to be rejected")
	}
}

func manualToken(secret []byte, file string, exp int64) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(fmt.Sprintf("%s|%d", file, exp)))
	return hex.EncodeToString(mac.Sum(nil))
}
