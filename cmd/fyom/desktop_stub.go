//go:build !desktop || server || ios || android

package main

import (
	"context"
	"fmt"

	"github.com/fyom/fyom/internal/app"
)

func runDesktopWithRuntime(_ context.Context, _ *app.Runtime) error {
	return fmt.Errorf("desktop runtime")
}
