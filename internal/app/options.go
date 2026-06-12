package app

// RunMode represents the application run mode.
type RunMode string

const (
	RunModeServer  RunMode = "server"
	RunModeSidecar RunMode = "sidecar"
)

// RunOptions holds the configuration for running the application.
type RunOptions struct {
	Mode       RunMode
	Host       string
	Port       int
	DataDir    string
	LogLevel   string
	LogFormat  string
}

// DefaultRunOptions returns the default run options for server mode.
func DefaultRunOptions() RunOptions {
	return RunOptions{
		Mode:      RunModeServer,
		Host:      "0.0.0.0",
		Port:      8080,
		DataDir:   ".",
		LogLevel:  "info",
		LogFormat: "text",
	}
}

// SidecarRunOptions returns the run options for sidecar mode.
func SidecarRunOptions(dataDir, logLevel, logFormat string) RunOptions {
	return RunOptions{
		Mode:       RunModeSidecar,
		Host:       "127.0.0.1",
		Port:       27403,
		DataDir:    dataDir,
		LogLevel:   logLevel,
		LogFormat:  logFormat,
	}
}
