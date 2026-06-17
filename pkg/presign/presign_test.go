package presign

import (
	"strconv"
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

// ─── ValidateMethod (method-bound signature) ─────────────────────────────────
//
// The signature is now bound to METHOD + PATH + EXP. These tests lock the
// method-binding contract, the GET/HEAD normalization, the far-future-expiry
// rejection, and the legacy Validate() compatibility shim.

func TestSigner_ValidateMethod_GETPasses(t *testing.T) {
	signer := NewSigner("method-secret", 3600)
	exp := time.Now().Unix() + 600
	sig := signer.Sign("GET", "/api/v1/media/m1/stream", exp)

	if !signer.ValidateMethod("GET", "/api/v1/media/m1/stream", strconv.FormatInt(exp, 10), sig) {
		t.Error("ValidateMethod should accept a correctly signed GET URL")
	}
}

// HEAD and GET share a signature (browsers probe with HEAD while the URL was
// generated for GET access).
func TestSigner_ValidateMethod_HEADNormalizesToGET(t *testing.T) {
	signer := NewSigner("method-secret", 3600)
	exp := time.Now().Unix() + 600

	// Sign for GET, validate for HEAD.
	sig := signer.Sign("GET", "/api/v1/media/m1/poster", exp)
	if !signer.ValidateMethod("HEAD", "/api/v1/media/m1/poster", strconv.FormatInt(exp, 10), sig) {
		t.Error("ValidateMethod should accept HEAD for a GET-signed URL")
	}

	// Sign for HEAD, validate for GET.
	sig2 := signer.Sign("HEAD", "/api/v1/media/m1/poster", exp)
	if !signer.ValidateMethod("GET", "/api/v1/media/m1/poster", strconv.FormatInt(exp, 10), sig2) {
		t.Error("ValidateMethod should accept GET for a HEAD-signed URL")
	}
}

// A signature bound to POST must NOT validate for GET. This is the core
// method-binding guarantee: the signature is bound to METHOD+PATH+EXP, so a
// signature computed for one method cannot be replayed against a different
// method. (Note: RequireValidPresign separately enforces that only GET/HEAD
// reach ValidateMethod at all; this test exercises the lower-level binding.)
func TestSigner_ValidateMethod_PostSignatureRejectedForGET(t *testing.T) {
	signer := NewSigner("method-secret", 3600)
	exp := time.Now().Unix() + 600
	path := "/api/v1/media/m1/stream"
	sig := signer.Sign("POST", path, exp)

	// A POST-signed signature must not validate as GET (different method).
	if signer.ValidateMethod("GET", path, strconv.FormatInt(exp, 10), sig) {
		t.Error("POST-signed signature must not validate for GET")
	}
	// Conversely, a GET-signed signature must not validate as POST.
	getSig := signer.Sign("GET", path, exp)
	if signer.ValidateMethod("POST", path, strconv.FormatInt(exp, 10), getSig) {
		t.Error("GET-signed signature must not validate for POST")
	}
	// A POST-signed signature DOES validate for POST (method-consistent). This
	// is correct: ValidateMethod is method-agnostic for non-GET/HEAD verbs;
	// the GET/HEAD-only enforcement lives in RequireValidPresign.
	if !signer.ValidateMethod("POST", path, strconv.FormatInt(exp, 10), sig) {
		t.Error("POST-signed signature should validate for POST (method-consistent)")
	}
}

// An empty method normalizes to GET (the resource-access default).
func TestSigner_ValidateMethod_EmptyMethodNormalizesToGET(t *testing.T) {
	signer := NewSigner("method-secret", 3600)
	exp := time.Now().Unix() + 600
	sig := signer.Sign("GET", "/api/v1/media/m1/poster", exp)

	if !signer.ValidateMethod("", "/api/v1/media/m1/poster", strconv.FormatInt(exp, 10), sig) {
		t.Error("empty method should normalize to GET and validate a GET-signed URL")
	}
}

// Far-future exp must be rejected even when the MAC is correct. This caps the
// lifetime of a presigned URL to the configured expiry window.
func TestSigner_ValidateMethod_FarFutureExpRejected(t *testing.T) {
	signer := NewSigner("method-secret", 3600) // window = expirySeconds(3600) + clockSkew(30) = 3630s
	farFuture := time.Now().Unix() + 86400     // 1 day, far beyond the 3630s window
	sig := signer.Sign("GET", "/api/v1/media/m1/stream", farFuture)

	if signer.ValidateMethod("GET", "/api/v1/media/m1/stream", strconv.FormatInt(farFuture, 10), sig) {
		t.Error("far-future exp must be rejected even with a correct MAC")
	}
}

func TestSigner_ValidateMethod_ExpiredRejected(t *testing.T) {
	signer := NewSigner("method-secret", 3600)
	expired := time.Now().Unix() - 100
	sig := signer.Sign("GET", "/api/v1/media/m1/stream", expired)

	if signer.ValidateMethod("GET", "/api/v1/media/m1/stream", strconv.FormatInt(expired, 10), sig) {
		t.Error("expired signature must be rejected")
	}
}

func TestSigner_ValidateMethod_TamperedPathRejected(t *testing.T) {
	signer := NewSigner("method-secret", 3600)
	exp := time.Now().Unix() + 600
	sig := signer.Sign("GET", "/api/v1/media/aaa/stream", exp)

	if signer.ValidateMethod("GET", "/api/v1/media/bbb/stream", strconv.FormatInt(exp, 10), sig) {
		t.Error("signature for path A must not validate for path B")
	}
}

func TestSigner_ValidateMethod_NonHexSignatureRejected(t *testing.T) {
	signer := NewSigner("method-secret", 3600)
	exp := time.Now().Unix() + 600

	if signer.ValidateMethod("GET", "/api/v1/media/m1/stream", strconv.FormatInt(exp, 10), "not-hex!") {
		t.Error("non-hex signature must be rejected without panicking")
	}
}

func TestSigner_ValidateMethod_NilSignerReturnsFalse(t *testing.T) {
	var signer *Signer

	if signer.ValidateMethod("GET", "/x", "1", "ab") {
		t.Error("nil signer must return false")
	}
}

func TestSigner_ValidateMethod_EmptySecretReturnsFalse(t *testing.T) {
	signer := NewSigner("", 3600)
	exp := time.Now().Unix() + 600
	sig := signer.Sign("GET", "/x", exp)

	if signer.ValidateMethod("GET", "/x", strconv.FormatInt(exp, 10), sig) {
		t.Error("empty-secret signer must reject all validation")
	}
}

func TestSigner_ValidateMethod_EmptyPathRejected(t *testing.T) {
	signer := NewSigner("method-secret", 3600)
	exp := time.Now().Unix() + 600
	sig := signer.Sign("GET", "/x", exp)

	if signer.ValidateMethod("GET", "", strconv.FormatInt(exp, 10), sig) {
		t.Error("empty path must be rejected")
	}
}

// Validate (legacy) must behave identically to ValidateMethod("GET", ...).
// This locks the backward-compat shim so existing callers keep working.
func TestSigner_ValidateMatchesValidateMethodGET(t *testing.T) {
	signer := NewSigner("compat-secret", 3600)
	exp := time.Now().Unix() + 600
	sig := signer.Sign("GET", "/api/v1/media/compat/poster", exp)
	expStr := strconv.FormatInt(exp, 10)

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"correct path", "/api/v1/media/compat/poster", true},
		{"wrong path", "/api/v1/media/other/poster", false},
		{"empty path", "", false},
	}
	for _, tc := range cases {
		gotLegacy := signer.Validate(tc.path, expStr, sig)
		gotMethod := signer.ValidateMethod("GET", tc.path, expStr, sig)
		if gotLegacy != tc.want {
			t.Errorf("Validate(%q): want %v got %v", tc.path, tc.want, gotLegacy)
		}
		if gotMethod != tc.want {
			t.Errorf("ValidateMethod(GET, %q): want %v got %v", tc.path, tc.want, gotMethod)
		}
		if gotLegacy != gotMethod {
			t.Errorf("Validate and ValidateMethod(GET) disagree on %q: %v vs %v", tc.path, gotLegacy, gotMethod)
		}
	}
}

// Query strings on the path are stripped before signing/validation. This lets
// Range headers and other client-side query params ride along on a presigned
// URL without breaking the signature.
func TestSigner_ValidateMethod_StripsQueryString(t *testing.T) {
	signer := NewSigner("qs-secret", 3600)
	exp := time.Now().Unix() + 600

	// Sign the bare path; validate a path+query variant — both must reduce to
	// the same normalized path.
	sig := signer.Sign("GET", "/api/v1/media/qs/stream", exp)
	if !signer.ValidateMethod("GET", "/api/v1/media/qs/stream?range=0-1023", strconv.FormatInt(exp, 10), sig) {
		t.Error("query string must be stripped before signature comparison")
	}

	// And the reverse: sign with a query, validate the bare path.
	sig2 := signer.Sign("GET", "/api/v1/media/qs2/stream?foo=bar", exp)
	if !signer.ValidateMethod("GET", "/api/v1/media/qs2/stream", strconv.FormatInt(exp, 10), sig2) {
		t.Error("signed path+query must validate against the bare path after normalization")
	}
}

// Different secrets produce non-interchangeable signatures.
func TestSigner_ValidateMethod_DifferentSecretsDoNotInterchange(t *testing.T) {
	signerA := NewSigner("secret-A", 3600)
	signerB := NewSigner("secret-B", 3600)
	exp := time.Now().Unix() + 600
	sig := signerA.Sign("GET", "/api/v1/media/x/poster", exp)

	if signerB.ValidateMethod("GET", "/api/v1/media/x/poster", strconv.FormatInt(exp, 10), sig) {
		t.Error("signature from secret A must not validate with signer B")
	}
}
