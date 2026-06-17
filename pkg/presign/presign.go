// Package presign provides S3-style presigned URL generation and validation
// for media resource access without requiring Authorization headers.
package presign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultExpirySeconds    int64 = 3600
	defaultClockSkewSeconds int64 = 30
)

// Signer generates and validates presigned URLs using HMAC-SHA256.
//
// The signature is bound to:
//   - HTTP method, normalized to GET for GET/HEAD resource access
//   - request path
//   - expiry timestamp
//
// Query parameters other than exp and sig are not part of the signature.
// This matches fyom's resource-dispatcher model where signed URLs authorize
// direct GET/HEAD access to a specific resource path for a limited time.
type Signer struct {
	secret           []byte
	expirySeconds    int64
	clockSkewSeconds int64
}

// NewSigner creates a new Signer with the given secret and expiry window.
func NewSigner(secret string, expirySeconds int64) *Signer {
	if expirySeconds <= 0 {
		expirySeconds = defaultExpirySeconds
	}

	return &Signer{
		secret:           []byte(secret),
		expirySeconds:    expirySeconds,
		clockSkewSeconds: defaultClockSkewSeconds,
	}
}

// Generate creates a presigned URL for the given path.
//
// The returned URL includes:
//   - exp: unix timestamp
//   - sig: HMAC-SHA256 signature
func (s *Signer) Generate(basePath string) string {
	exp := time.Now().Unix() + s.expirySeconds
	sig := s.Sign("GET", basePath, exp)

	separator := "?"
	if strings.Contains(basePath, "?") {
		separator = "&"
	}

	return fmt.Sprintf("%s%sexp=%d&sig=%s", basePath, separator, exp, url.QueryEscape(sig))
}

// Sign returns the hex-encoded HMAC-SHA256 signature for method/path/exp.
func (s *Signer) Sign(method string, path string, exp int64) string {
	return hex.EncodeToString(s.signBytes(method, path, exp))
}

// Validate checks whether exp and sig are valid for a GET request path.
//
// This method is kept for backward compatibility with older callers.
// Prefer ValidateMethod when the HTTP method is available.
func (s *Signer) Validate(path string, expStr string, sig string) bool {
	return s.ValidateMethod("GET", path, expStr, sig)
}

// ValidateMethod checks whether exp and sig are valid for the given method/path.
//
// GET and HEAD intentionally share the same signature because browsers and
// intermediaries may use HEAD for resource probing while the URL was generated
// for GET access.
func (s *Signer) ValidateMethod(method string, path string, expStr string, sig string) bool {
	if s == nil || len(s.secret) == 0 {
		return false
	}

	path = normalizePath(path)
	if path == "" {
		return false
	}

	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return false
	}

	now := time.Now().Unix()

	if now > exp {
		return false
	}

	// Enforce the configured expiry window. This prevents accepting signatures
	// with arbitrarily far-future expirations.
	if exp > now+s.expirySeconds+s.clockSkewSeconds {
		return false
	}

	providedMAC, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}

	expectedMAC := s.signBytes(method, path, exp)

	return hmac.Equal(providedMAC, expectedMAC)
}

func (s *Signer) signBytes(method string, path string, exp int64) []byte {
	normalizedMethod := normalizeMethod(method)
	normalizedPath := normalizePath(path)

	stringToSign := fmt.Sprintf("%s\n%s\n%d", normalizedMethod, normalizedPath, exp)

	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(stringToSign))

	return mac.Sum(nil)
}

func normalizeMethod(method string) string {
	normalized := strings.ToUpper(strings.TrimSpace(method))

	switch normalized {
	case "", "GET", "HEAD":
		return "GET"
	default:
		return normalized
	}
}

func normalizePath(path string) string {
	if path == "" {
		return ""
	}

	// Sign only the path portion. If a caller accidentally passes a full URL or
	// a path with query parameters, strip the query before signing.
	if parsed, err := url.Parse(path); err == nil {
		if parsed.Path != "" {
			return parsed.Path
		}
	}

	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}

	return path
}
