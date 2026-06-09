// Package presign provides S3-style presigned URL generation and validation
// for media resource access without requiring Authorization headers.
package presign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Signer generates and validates presigned URLs using HMAC-SHA256.
type Signer struct {
	secret        []byte
	expirySeconds int64
}

// NewSigner creates a new Signer with the given secret and expiry window.
func NewSigner(secret string, expirySeconds int64) *Signer {
	return &Signer{secret: []byte(secret), expirySeconds: expirySeconds}
}

// Generate creates a presigned URL for the given path.
// The URL includes exp (expiry timestamp) and sig (HMAC-SHA256 signature).
func (s *Signer) Generate(basePath string) string {
	exp := time.Now().Unix() + s.expirySeconds
	stringToSign := fmt.Sprintf("GET\n%s\n%d", basePath, exp)
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(stringToSign))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s?exp=%d&sig=%s", basePath, exp, sig)
}

// Validate checks whether exp and sig are valid for the given request path.
// Returns false if the signature is missing, malformed, expired, or incorrect.
func (s *Signer) Validate(path string, expStr string, sig string) bool {
	if expStr == "" || sig == "" {
		return false
	}

	var exp int64
	if _, err := fmt.Sscanf(expStr, "%d", &exp); err != nil {
		return false
	}

	if time.Now().Unix() > exp {
		return false
	}

	stringToSign := fmt.Sprintf("GET\n%s\n%d", path, exp)
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(stringToSign))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(sig), []byte(expectedSig))
}
