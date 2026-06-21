// Package main is the entry point for the fyom server and desktop application.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/fyom/fyom/internal/app"
)

// desktopRuntime is set by runDesktop and read by the Wails shutdown hook
// in desktop.go. It is nil in serve mode.
var desktopRuntime *app.DesktopRuntime

func main() {
	logLevel := flag.String("log-level", "", "log level: debug, info, warn, error")
	logFormat := flag.String("log-format", "", "log format: text, json")
	dbPath := flag.String("db-path", "", "path to sqlite database file")
	mode := flag.String("mode", "serve", "run mode: serve or desktop")
	flag.Parse()

	switch *mode {
	case "serve":
		if err := runServe(*logLevel, *logFormat, *dbPath); err != nil {
			slog.Error("server error", "error", err)
			fmt.Fprintln(os.Stderr, "server stopped with error")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "server stopped gracefully")

	case "desktop":
		if err := runDesktop(*dbPath); err != nil {
			slog.Error("desktop error", "error", err)
			fmt.Fprintln(os.Stderr, "desktop stopped with error")
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q; expected serve or desktop\n", *mode)
		os.Exit(1)
	}
}

func runServe(logLevel, logFormat, dbPath string) error {
	opts := app.DefaultRunOptions()

	if logLevel != "" {
		opts.LogLevel = logLevel
	}
	if logFormat != "" {
		opts.LogFormat = logFormat
	}
	opts.DBPath = dbPath

	return app.Run(opts)
}

func runDesktop(dbPath string) error {
	ctx := context.Background()

	rt, err := app.NewDesktopRuntime(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("new desktop runtime: %w", err)
	}

	if err := rt.Startup(ctx); err != nil {
		return fmt.Errorf("desktop startup: %w", err)
	}

	// Store reference for shutdown hook in desktop.go.
	desktopRuntime = rt

	return runDesktopWithRuntime(ctx, rt)
}
