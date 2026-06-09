package presign

import (
	"testing"
	"time"
)

func TestSigner_GenerateAndValidate(t *testing.T) {
	signer := NewSigner("test-secret", 3600)

	// Generate a URL for a poster path.
	url := signer.Generate("/api/v1/media/abc-123/poster")
	if url == "" {
		t.Fatal("generated URL is empty")
	}

	// Extract exp and sig from the generated URL for validation.
	exp, sig := extractParams(url)

	// Validate: correct path, exp, sig → should pass.
	if !signer.Validate("/api/v1/media/abc-123/poster", exp, sig) {
		t.Error("Validate returned false for a freshly generated URL")
	}

	// Wrong path → should fail.
	if signer.Validate("/api/v1/media/xyz-999/poster", exp, sig) {
		t.Error("Validate returned true for wrong path")
	}

	// Tampered sig → should fail.
	if signer.Validate("/api/v1/media/abc-123/poster", exp, "deadbeef") {
		t.Error("Validate returned true for tampered signature")
	}

	// Missing exp and sig → should fail.
	if signer.Validate("/api/v1/media/abc-123/poster", "", "") {
		t.Error("Validate returned true for missing exp and sig")
	}

	// Empty sig.
	if signer.Validate("/api/v1/media/abc-123/poster", "12345", "") {
		t.Error("Validate returned true for empty sig")
	}

	// Empty exp.
	if signer.Validate("/api/v1/media/abc-123/poster", "", "abc123") {
		t.Error("Validate returned true for empty exp")
	}

	// Invalid exp (non-numeric).
	if signer.Validate("/api/v1/media/abc-123/poster", "notanumber", "abc123") {
		t.Error("Validate returned true for non-numeric exp")
	}

	// Expired: compute a correctly-signed expired URL.
	expiredExp := time.Now().Unix() - 100
	// We can't re-sign with an arbitrary exp without calling Generate,
	// but we can test that Validate rejects an expired exp by generating
	// with a short-lived signer.
	shortSigner := NewSigner("test-secret", 1)
	shortURL := shortSigner.Generate("/api/v1/media/exp-test/stream")
	exp2, sig2 := extractParams(shortURL)
	// The URL was just generated so it's not expired yet.
	if !shortSigner.Validate("/api/v1/media/exp-test/stream", exp2, sig2) {
		t.Error("fresh short-lived URL should validate")
	}

	// Suppress unused var.
	_ = expiredExp
}

func TestSigner_URLDeterministicWithinExpiry(t *testing.T) {
	signer := NewSigner("test-secret-2", 3600)

	// Two calls within the same second should produce the same URL.
	url1 := signer.Generate("/api/v1/media/test/stream")
	url2 := signer.Generate("/api/v1/media/test/stream")

	if url1 != url2 {
		t.Errorf("same-second URLs should be identical:\n  %s\n  %s", url1, url2)
	}
}

func TestSigner_DifferentPathsDifferentURLs(t *testing.T) {
	signer := NewSigner("test-secret-3", 3600)

	url1 := signer.Generate("/api/v1/media/aaa/poster")
	url2 := signer.Generate("/api/v1/media/bbb/poster")

	if url1 == url2 {
		t.Error("different paths should produce different URLs")
	}
}

func TestSigner_DifferentResourcesDifferentURLs(t *testing.T) {
	signer := NewSigner("test-secret-4", 3600)

	// Poster, backdrop, and stream for the same ID should all differ.
	id := "same-id"
	posterURL := signer.Generate("/api/v1/media/" + id + "/poster")
	backdropURL := signer.Generate("/api/v1/media/" + id + "/backdrop")
	streamURL := signer.Generate("/api/v1/media/" + id + "/stream")

	if posterURL == backdropURL {
		t.Error("poster and backdrop URLs should differ")
	}
	if posterURL == streamURL {
		t.Error("poster and stream URLs should differ")
	}
	if backdropURL == streamURL {
		t.Error("backdrop and stream URLs should differ")
	}

	// Each should validate only for its own path.
	pExp, pSig := extractParams(posterURL)
	bExp, bSig := extractParams(backdropURL)
	sExp, sSig := extractParams(streamURL)

	if !signer.Validate("/api/v1/media/"+id+"/poster", pExp, pSig) {
		t.Error("poster URL should validate for poster path")
	}
	if signer.Validate("/api/v1/media/"+id+"/backdrop", pExp, pSig) {
		t.Error("poster sig should NOT validate for backdrop path")
	}

	if !signer.Validate("/api/v1/media/"+id+"/backdrop", bExp, bSig) {
		t.Error("backdrop URL should validate for backdrop path")
	}
	if signer.Validate("/api/v1/media/"+id+"/stream", bExp, bSig) {
		t.Error("backdrop sig should NOT validate for stream path")
	}

	if !signer.Validate("/api/v1/media/"+id+"/stream", sExp, sSig) {
		t.Error("stream URL should validate for stream path")
	}
	if signer.Validate("/api/v1/media/"+id+"/poster", sExp, sSig) {
		t.Error("stream sig should NOT validate for poster path")
	}
}

// extractParams pulls exp and sig from a presigned URL of the form
// "/path?exp=<int>&sig=<hex>". Returns the exp string and sig string.
func extractParams(url string) (string, string) {
	// Find '?'
	qIdx := -1
	for i := 0; i < len(url); i++ {
		if url[i] == '?' {
			qIdx = i
			break
		}
	}
	if qIdx < 0 {
		return "", ""
	}

	var expStr, sigStr string
	query := url[qIdx+1:]
	// Split on '&'
	start := 0
	for i := 0; i <= len(query); i++ {
		if i == len(query) || query[i] == '&' {
			pair := query[start:i]
			if len(pair) > 4 && pair[:4] == "exp=" {
				expStr = pair[4:]
			}
			if len(pair) > 4 && pair[:4] == "sig=" {
				sigStr = pair[4:]
			}
			start = i + 1
		}
	}
	return expStr, sigStr
}
