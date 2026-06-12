package server

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
)

func TestFrontendAssetHash(t *testing.T) {
	fsys := fstest.MapFS{
		"dist/index.html": &fstest.MapFile{Data: []byte("<html>test</html>")},
	}
	hash := FrontendAssetHash(fsys)
	assert.Len(t, hash, 12, "hash should be 12 hex chars")
	assert.NotEmpty(t, hash)
}

func TestFrontendAssetHash_MissingIndex(t *testing.T) {
	fsys := fstest.MapFS{}
	hash := FrontendAssetHash(fsys)
	assert.Equal(t, "unavailable", hash)
}
