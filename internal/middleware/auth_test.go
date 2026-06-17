package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── AllowLocalOnly ──────────────────────────────────────────────────────────
//
// AllowLocalOnly guards the desktop bootstrap session bridge. It must accept
// any loopback origin (IPv4 127.0.0.0/8 and IPv6 ::1, with or without a port)
// and reject everything else — and must NOT be spoofable via X-Forwarded-For.

// localRequest builds a GET request whose RemoteAddr is set to addr, simulating
// what net/http populates after accept().
func localRequest(t *testing.T, addr string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/internal/bootstrap-session", nil)
	r.RemoteAddr = addr
	return r
}

func TestAllowLocalOnly_AcceptsIPv4LoopbackWithPort(t *testing.T) {
	r := localRequest(t, "127.0.0.1:54321")
	captured, rec := applyMiddleware(t, AllowLocalOnly, r)

	require.NotNil(t, captured, "127.0.0.1 with port must reach next handler")
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAllowLocalOnly_AcceptsIPv4LoopbackWithoutPort(t *testing.T) {
	// Some test harnesses / proxies leave RemoteAddr as a bare host.
	r := localRequest(t, "127.0.0.1")
	captured, rec := applyMiddleware(t, AllowLocalOnly, r)

	require.NotNil(t, captured)
	require.Equal(t, http.StatusOK, rec.Code)
}

// TestAllowLocalOnly_AcceptsIPv6LoopbackWithPort is the regression test for the
// bug this change fixes. The previous strings.LastIndex(":") implementation
// split "[::1]:54321" into host="[::1]" (brackets kept), which never matched
// the "::1" branch and caused legitimate IPv6 loopback requests to be rejected
// with 403.
func TestAllowLocalOnly_AcceptsIPv6LoopbackWithPort(t *testing.T) {
	r := localRequest(t, "[::1]:54321")
	captured, rec := applyMiddleware(t, AllowLocalOnly, r)

	require.NotNil(t, captured, "IPv6 loopback [::1]:port must reach next handler")
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAllowLocalOnly_AcceptsIPv6LoopbackBare(t *testing.T) {
	// Bare "::1" (no brackets, no port) — the previous implementation split at
	// the last colon and produced "::", which was rejected.
	r := localRequest(t, "::1")
	captured, rec := applyMiddleware(t, AllowLocalOnly, r)

	require.NotNil(t, captured)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAllowLocalOnly_AcceptsIPv6LoopbackBracketedBare(t *testing.T) {
	r := localRequest(t, "[::1]")
	captured, rec := applyMiddleware(t, AllowLocalOnly, r)

	require.NotNil(t, captured)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAllowLocalOnly_AcceptsLocalhostHostname(t *testing.T) {
	// net/http never populates RemoteAddr with a hostname (always IP:port), but
	// the historical implementation accepted the literal "localhost" string, so
	// we keep it for test-harness compatibility.
	r := localRequest(t, "localhost:54321")
	captured, rec := applyMiddleware(t, AllowLocalOnly, r)

	require.NotNil(t, captured)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAllowLocalOnly_AcceptsNonLiteral127Loopback(t *testing.T) {
	// 127.0.0.0/8 is entirely loopback per RFC 5735. The literal-match branch
	// only covers 127.0.0.1, so 127.1.2.3 must be accepted via net.ParseIP +
	// IsLoopback to avoid falsely rejecting other loopback addresses.
	r := localRequest(t, "127.1.2.3:54321")
	captured, rec := applyMiddleware(t, AllowLocalOnly, r)

	require.NotNil(t, captured, "127.0.0.0/8 must be accepted as loopback")
	require.Equal(t, http.StatusOK, rec.Code)
}

// ─── Rejection ───────────────────────────────────────────────────────────────

func TestAllowLocalOnly_RejectsPublicIPv4(t *testing.T) {
	r := localRequest(t, "203.0.113.10:54321")
	captured, rec := applyMiddleware(t, AllowLocalOnly, r)

	require.Nil(t, captured, "public IPv4 must not reach next handler")
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestAllowLocalOnly_RejectsPublicIPv6(t *testing.T) {
	r := localRequest(t, "[2001:db8::1]:54321")
	captured, rec := applyMiddleware(t, AllowLocalOnly, r)

	require.Nil(t, captured, "public IPv6 must not reach next handler")
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestAllowLocalOnly_RejectsPrivateIPv4(t *testing.T) {
	// RFC1918 private space is NOT loopback and must be rejected — the
	// bootstrap endpoint is localhost-only, not LAN-only.
	r := localRequest(t, "192.168.1.5:54321")
	captured, rec := applyMiddleware(t, AllowLocalOnly, r)

	require.Nil(t, captured)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestAllowLocalOnly_RejectsEmptyRemoteAddr(t *testing.T) {
	// Fail closed when RemoteAddr is unset (e.g., a malformed test request or
	// an exotic transport with no peer address).
	r := localRequest(t, "")
	captured, rec := applyMiddleware(t, AllowLocalOnly, r)

	require.Nil(t, captured)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

// ─── Anti-spoofing ───────────────────────────────────────────────────────────
//
// The single most important property of AllowLocalOnly: an attacker on a public
// IP must NOT be able to bypass the check by adding X-Forwarded-For: 127.0.0.1.
// The middleware must inspect r.RemoteAddr only.

func TestAllowLocalOnly_IgnoresXForwardedFor(t *testing.T) {
	r := localRequest(t, "203.0.113.10:54321")
	r.Header.Set("X-Forwarded-For", "127.0.0.1")
	r.Header.Set("X-Real-IP", "::1")

	captured, rec := applyMiddleware(t, AllowLocalOnly, r)

	require.Nil(t, captured, "XFF must not override a non-loopback RemoteAddr")
	require.Equal(t, http.StatusForbidden, rec.Code)
}

// ─── isLoopbackRemoteAddr: pure helper unit cases ────────────────────────────
//
// Direct table-driven coverage of the parser so edge cases are pinned without
// the middleware wrapper.

func TestIsLoopbackRemoteAddr(t *testing.T) {
	cases := []struct {
		name string
		addr string
		want bool
	}{
		{"ipv4 loopback with port", "127.0.0.1:54321", true},
		{"ipv4 loopback bare", "127.0.0.1", true},
		{"ipv4 non-literal loopback", "127.255.255.254:1", true},
		{"ipv6 loopback with port", "[::1]:54321", true},
		{"ipv6 loopback bare", "::1", true},
		{"ipv6 loopback bracketed bare", "[::1]", true},
		{"localhost with port", "localhost:54321", true},
		{"localhost bare", "localhost", true},
		{"public ipv4", "203.0.113.10:54321", false},
		{"public ipv6", "[2001:db8::1]:54321", false},
		{"private ipv4", "192.168.1.5:54321", false},
		{"link-local ipv4", "169.254.1.1:54321", false},
		{"empty", "", false},
		{"garbage", "not-an-address", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isLoopbackRemoteAddr(tc.addr))
		})
	}
}
