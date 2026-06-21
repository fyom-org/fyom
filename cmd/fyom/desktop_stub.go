//go:build !desktop || server || ios || android

package main

import (
	"context"
	"fmt"

	"github.com/fyom/fyom/internal/app"
)

// desktopRuntime holds the runtime reference for the Wails shutdown hook.
// It is nil in non-desktop builds. Assigned by main.go, read by desktop.go.
//
//nolint:unused
var desktopRuntime *app.DesktopRuntime

func runDesktopWithRuntime(_ context.Context, _ *app.DesktopRuntime) error {
	return fmt.Errorf("desktop runtime not available in this build")
}
