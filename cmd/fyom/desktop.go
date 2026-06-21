// Package main — desktop entry point for Wails integration.
package main

import (
	"context"
	"fmt"

	web "github.com/fyom/fyom/frontend"
	"github.com/fyom/fyom/internal/app"
	"github.com/fyom/fyom/internal/desktop"
	"github.com/wailsapp/wails/v2/pkg/application"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func runDesktopWithRuntime(ctx context.Context, rt *app.Runtime) error {
	handler, err := desktop.NewHandler(desktop.HandlerOptions{
		APIPrefix: "/api/v1/",
		API:       rt.Router,
		Assets:    web.Dist,
	})
	if err != nil {
		return fmt.Errorf("create desktop handler: %w", err)
	}

	wailsApp := application.NewWithOptions(&options.App{
		Title:  "fyom",
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets:  web.Dist,
			Handler: handler,
		},
		OnStartup: func(ctx context.Context) {
			runtime.LogInfo(ctx, "fyom desktop started; serving API in-process via Wails")
		},
		OnShutdown: func(ctx context.Context) {
			runtime.LogInfo(ctx, "fyom desktop shutting down")
			if rt != nil && rt.Shutdown != nil {
				_ = rt.Shutdown(ctx)
			}
		},
		Bind: []interface{}{},
	})

	return wailsApp.Run()
}
