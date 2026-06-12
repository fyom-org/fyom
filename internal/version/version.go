// Package version provides build-time version information.
// These variables are overridden at build time via -ldflags:
//
//	go build -ldflags "-X github.com/fyom/fyom/internal/version.Version=v0.9.0
//	                    -X github.com/fyom/fyom/internal/version.Commit=$(git rev-parse --short HEAD)
//	                    -X github.com/fyom/fyom/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
package version

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)
