package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultRunOptions(t *testing.T) {
	opts := DefaultRunOptions()
	assert.Equal(t, RunModeServer, opts.Mode)
	assert.Equal(t, "0.0.0.0", opts.Host)
	assert.Equal(t, 8080, opts.Port)
	assert.Equal(t, ".", opts.DataDir)
	assert.Equal(t, "info", opts.LogLevel)
	assert.Equal(t, "text", opts.LogFormat)
}

func TestSidecarRunOptions(t *testing.T) {
	opts := SidecarRunOptions("/tmp/fyom-data", "debug", "json")
	assert.Equal(t, RunModeSidecar, opts.Mode)
	assert.Equal(t, "127.0.0.1", opts.Host)
	assert.Equal(t, 27403, opts.Port)
	assert.Equal(t, "/tmp/fyom-data", opts.DataDir)
	assert.Equal(t, "debug", opts.LogLevel)
	assert.Equal(t, "json", opts.LogFormat)
}

func TestRunModeConstants(t *testing.T) {
	assert.Equal(t, RunMode("server"), RunModeServer)
	assert.Equal(t, RunMode("sidecar"), RunModeSidecar)
}
