// Package config loads and validates fyom application configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Server holds HTTP server configuration.
type Server struct {
	Host string `koanf:"host"`
	Port int    `koanf:"port"`
	Mode string `koanf:"mode"` // debug, release, test
}

// Database holds database configuration.
type Database struct {
	DBPath          string `koanf:"db_path"`
	MaxOpenConns    int    `koanf:"max_open_conns"`
	MaxIdleConns    int    `koanf:"max_idle_conns"`
	ConnMaxLifetime int    `koanf:"conn_max_lifetime_seconds"`
}

// Auth holds authentication configuration.
type Auth struct {
	JWTSecret     string `koanf:"jwt_secret"`
	TokenExpiry   int    `koanf:"token_expiry_hours"`
	RefreshExpiry int    `koanf:"refresh_expiry_hours"`
}

// Log holds logging configuration.
type Log struct {
	Level  string `koanf:"level"`  // debug, info, warn, error
	Format string `koanf:"format"` // json, text
}

// Config is the root configuration struct.
type Config struct {
	Server   Server   `koanf:"server"`
	Database Database `koanf:"database"`
	Auth     Auth     `koanf:"auth"`
	Log      Log      `koanf:"log"`
}

// Address returns the full server address.
func (s Server) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// Load reads configuration from file, env, and defaults using Koanf.
func Load(cfgPath string) (*Config, error) {
	k := koanf.New(".")

	// Defaults via confmap provider
	defaults := map[string]interface{}{
		"server.host":                        "0.0.0.0",
		"server.port":                        8080,
		"server.mode":                        "release",
		"database.db_path":                   "",
		"database.max_open_conns":            25,
		"database.max_idle_conns":            5,
		"database.conn_max_lifetime_seconds": 300,
		"auth.jwt_secret":                    "change-me-in-production",
		"auth.token_expiry_hours":            24,
		"auth.refresh_expiry_hours":          168,
		"log.level":                          "info",
		"log.format":                         "json",
	}
	if err := k.Load(confmap.Provider(defaults, "."), nil); err != nil {
		return nil, fmt.Errorf("load defaults: %w", err)
	}

	// Config file (YAML)
	if cfgPath != "" {
		f := file.Provider(cfgPath)
		if err := k.Load(f, yaml.Parser()); err != nil {
			var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
				return nil, fmt.Errorf("read config file: %w", err)
			}
			// Config file not found is OK — use defaults + env
		}
	} else {
		// Try default locations (ignore errors — files may not exist)
		for _, path := range []string{"configs/fyom.yaml", "fyom.yaml"} {
			f := file.Provider(path)
			_ = k.Load(f, yaml.Parser())
		}
	}

	// Environment variables with FYOM_ prefix
	_ = k.Load(env.Provider("FYOM_", ".", func(s string) string {
		s = strings.TrimPrefix(s, "FYOM_")
		return strings.ReplaceAll(strings.ToLower(s), "_", ".")
	}), nil)

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Explicit env override for DB path (koanf env conversion turns
	// FYOM_DB_PATH into db.path which doesn't match the db_path struct tag).
	if envDBPath := os.Getenv("FYOM_DB_PATH"); envDBPath != "" {
		cfg.Database.DBPath = envDBPath
	}

	return &cfg, nil
}
