package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultRunOptions(t *testing.T) {
	opts := DefaultRunOptions()
	assert.Equal(t, RunModeServer, opts.Mode)
	assert.Equal(t, "0.0.0.0", opts.Host)
	assert.Equal(t, 8080, opts.Port)
	assert.Equal(t, "", opts.DBPath)
	assert.Equal(t, "info", opts.LogLevel)
	assert.Equal(t, "text", opts.LogFormat)
}

func TestSidecarRunOptions(t *testing.T) {
	opts := SidecarRunOptions("/tmp/fyom.db", "debug", "json")
	assert.Equal(t, RunModeSidecar, opts.Mode)
	assert.Equal(t, "127.0.0.1", opts.Host)
	assert.Equal(t, 27403, opts.Port)
	assert.Equal(t, "/tmp/fyom.db", opts.DBPath)
	assert.Equal(t, "debug", opts.LogLevel)
	assert.Equal(t, "json", opts.LogFormat)
}

func TestRunModeConstants(t *testing.T) {
	assert.Equal(t, RunMode("server"), RunModeServer)
	assert.Equal(t, RunMode("sidecar"), RunModeSidecar)
}

func TestResolveDBPath_Flag(t *testing.T) {
	path, source, err := ResolveDBPath("/tmp/test.db")
	assert.NoError(t, err)
	assert.Equal(t, "flag", source)
	assert.Equal(t, filepath.Join("/tmp", "test.db"), path)
}

func TestResolveDBPath_Env(t *testing.T) {
	os.Setenv("FYOM_DB_PATH", "/env/path.db")
	defer os.Unsetenv("FYOM_DB_PATH")
	path, source, err := ResolveDBPath("")
	assert.NoError(t, err)
	assert.Equal(t, "env", source)
	assert.Equal(t, filepath.Join("/env", "path.db"), path)
}

func TestResolveDBPath_Default(t *testing.T) {
	os.Unsetenv("FYOM_DB_PATH")
	path, source, err := ResolveDBPath("")
	assert.NoError(t, err)
	assert.Equal(t, "default-binary-dir", source)
	assert.Contains(t, path, "fyom.db")
}

func TestResolveDBPath_RelativeFlag(t *testing.T) {
	path, source, err := ResolveDBPath("relative.db")
	assert.NoError(t, err)
	assert.Equal(t, "flag", source)
	abs, _ := filepath.Abs("relative.db")
	assert.Equal(t, abs, path)
}
