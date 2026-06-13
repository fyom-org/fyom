// Package main is the entry point for the fyom server.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/fyom/fyom/internal/app"
)

func main() {
	logLevel := flag.String("log-level", "info", "log level: debug, info, warn, error")
	logFormat := flag.String("log-format", "text", "log format: text, json")
	dbPath := flag.String("db-path", "", "path to sqlite database file (default: <binary-dir>/fyom.db)")
	sidecar := flag.Bool("sidecar", false, "run in sidecar mode (bind to 127.0.0.1:27403)")
	flag.Parse()

	opts := app.DefaultRunOptions()
	opts.LogLevel = *logLevel
	opts.LogFormat = *logFormat
	opts.DBPath = *dbPath

	if *sidecar {
		opts = app.SidecarRunOptions(*dbPath, *logLevel, *logFormat)
	}

	if err := app.Run(opts); err != nil {
		slog.Error("application error", "error", err)
		fmt.Fprintln(os.Stderr, "server stopped with error")
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "server stopped gracefully")
}
