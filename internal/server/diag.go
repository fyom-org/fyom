package server

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
)

// FrontendAssetHash computes a stable hash over the embedded frontend dist
// by hashing index.html's content. This is a cheap fingerprint sufficient to
// detect whether the frontend bundle changed between two builds of the same binary.
func FrontendAssetHash(distFS fs.FS) string {
	data, err := fs.ReadFile(distFS, "dist/index.html")
	if err != nil {
		return "unavailable"
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}
