//go:build desktop && !server && !ios && !android

// Package main contains the fyom desktop entry point.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/fyom/fyom/internal/app"
	web "github.com/fyom/fyom/frontend"
	"github.com/fyom/fyom/internal/desktop"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func runDesktopWithRuntime(_ context.Context, rt *app.DesktopRuntime) error {
	if rt == nil {
		return fmt.Errorf("desktop runtime is nil")
	}

	handler, err := desktop.NewHandler(desktop.HandlerOptions{
		APIPrefix: "/api/v1/",
		API:       rt.Router(),
		Assets:    web.Dist,
	})
	if err != nil {
		return fmt.Errorf("create desktop handler: %w", err)
	}

	wailsApp := application.New(application.Options{
		Name:        "fyom",
		Description: "fyom desktop application",
		Assets: application.AssetOptions{
			Handler: handler,
		},
	})

	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:   "main",
		Title:  "fyom",
		Width:  1280,
		Height: 800,
		URL:    "/",
	})

	wailsApp.OnShutdown(func() {
		log.Print("fyom desktop shutting down")

		if desktopRuntime != nil {
			if err := desktopRuntime.Shutdown(context.Background()); err != nil {
				log.Printf("shutdown desktop runtime: %v", err)
			}
		}
	})

	log.Print("fyom desktop started; serving API in-process via Wails")

	if err := wailsApp.Run(); err != nil {
		return fmt.Errorf("run wails desktop app: %w", err)
	}

	return nil
}
