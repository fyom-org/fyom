package app

// RunMode represents the application run mode.
type RunMode string

const (
	// RunModeServer is the primary server mode that listens for HTTP requests.
	RunModeServer RunMode = "server"
	// RunModeSidecar is the lightweight companion mode for desktop integration.
	RunModeSidecar RunMode = "sidecar"
)

// RunOptions holds the configuration for running the application.
type RunOptions struct {
	Mode      RunMode
	Host      string
	Port      int
	DBPath    string // raw --db-path flag value; empty means "use koanf/env/default"
	LogLevel  string // raw --log-level flag value; empty means "use koanf/env/default"
	LogFormat string // raw --log-format flag value; empty means "use koanf/env/default"
}

// DefaultRunOptions returns the default run options for server mode.
func DefaultRunOptions() RunOptions {
	return RunOptions{
		Mode:      RunModeServer,
		Host:      "0.0.0.0",
		Port:      8080,
		DBPath:    "",
		LogLevel:  "",
		LogFormat: "",
	}
}

// SidecarRunOptions returns the run options for sidecar mode.
func SidecarRunOptions(dbPath, logLevel, logFormat string) RunOptions {
	return RunOptions{
		Mode:      RunModeSidecar,
		Host:      "127.0.0.1",
		Port:      27403,
		DBPath:    dbPath,
		LogLevel:  logLevel,
		LogFormat: logFormat,
	}
}
