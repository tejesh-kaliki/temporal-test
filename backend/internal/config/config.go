// Package config loads runtime configuration from a YAML file with env overrides.
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Database      DatabaseConfig      `yaml:"database"`
	Observability ObservabilityConfig `yaml:"observability"`
	Token         TokenConfig         `yaml:"token"`
}

// TokenConfig holds JWT signing settings (used by the auth module).
type TokenConfig struct {
	Secret string `yaml:"secret"`
	// ExpiryHours is the access-token TTL. Keep it short; clients refresh.
	ExpiryHours int `yaml:"expiry_hours"`
	// RefreshExpiryHours is the refresh-token TTL (defaults to 720h / 30 days).
	RefreshExpiryHours int `yaml:"refresh_expiry_hours"`
}

type ServerConfig struct {
	Port string `yaml:"port"`
}

type DatabaseConfig struct {
	URL string `yaml:"url"`
}

// ObservabilityConfig is wired in by default. Leaving Endpoint empty selects a
// no-op tracer, so the app runs with zero external dependencies until you point
// it at a real OTel collector.
type ObservabilityConfig struct {
	ServiceName string `yaml:"service_name"`
	Endpoint    string `yaml:"endpoint"`
}

// envOverride copies the named env var into dst when set (a no-op otherwise).
// Centralizing the "if set, override" check keeps Load a flat list of fields
// instead of a branch per field, which is what actually drives its cyclomatic
// complexity.
func envOverride(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

// Load reads the YAML at path, then applies env overrides for anything that is
// commonly injected by the deployment environment.
func Load(path string) (*Config, error) {
	cfg := &Config{
		Server:        ServerConfig{Port: "8080"},
		Observability: ObservabilityConfig{ServiceName: "backend"},
		Token:         TokenConfig{ExpiryHours: 1, RefreshExpiryHours: 720},
	}

	if b, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(b, cfg); err != nil {
			return nil, err
		}
	}

	envOverride(&cfg.Server.Port, "SERVER_PORT")
	envOverride(&cfg.Database.URL, "DATABASE_URL")
	envOverride(&cfg.Observability.Endpoint, "OTEL_EXPORTER_OTLP_ENDPOINT")
	envOverride(&cfg.Token.Secret, "JWT_SECRET")

	return cfg, nil
}
