// Package config loads runtime configuration from a YAML file with env overrides.
package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Database      DatabaseConfig      `yaml:"database"`
	Observability ObservabilityConfig `yaml:"observability"`
	Token         TokenConfig         `yaml:"token"`
	Temporal      TemporalConfig      `yaml:"temporal"`
	Qdrant        QdrantConfig        `yaml:"qdrant"`
	Ollama        OllamaConfig        `yaml:"ollama"`
	Ingest        IngestConfig        `yaml:"ingest"`
}

// TemporalConfig points the client and worker at the Temporal frontend.
type TemporalConfig struct {
	// HostPort is the Temporal frontend gRPC address (e.g. "localhost:7233").
	HostPort string `yaml:"hostport"`
	// Namespace defaults to "default" on the dev server.
	Namespace string `yaml:"namespace"`
	// TaskQueue that the worker listens on and clients target.
	TaskQueue string `yaml:"task_queue"`
	// WorkerConcurrency caps how many activities the worker executes at once
	// (worker.Options.MaxConcurrentActivityExecutionSize). Every EmbedAndUpsert
	// activity hits the same local Ollama instance; without a cap, a bulk drop
	// of many documents starts them all in parallel and the resulting
	// contention makes individual embed calls slow enough to blow past their
	// activity timeout instead of completing steadily.
	WorkerConcurrency int `yaml:"worker_concurrency"`
}

// QdrantConfig addresses the Qdrant vector DB over its REST API.
type QdrantConfig struct {
	URL string `yaml:"url"`
	// Collection holds the ingested chunk vectors.
	Collection string `yaml:"collection"`
}

// OllamaConfig addresses a running Ollama instance and names the models used
// for embedding (ingestion + query) and generation (the ask endpoint).
type OllamaConfig struct {
	URL             string `yaml:"url"`
	EmbeddingModel  string `yaml:"embedding_model"`
	GenerationModel string `yaml:"generation_model"`
}

// IngestConfig controls the folder-watching trigger.
type IngestConfig struct {
	// WatchDir is the folder scanned/watched for files to ingest.
	WatchDir string `yaml:"watch_dir"`
	// Trigger selects the mechanism: "schedule" | "fsnotify" | "both" | "off".
	Trigger string `yaml:"trigger"`
	// PollInterval is the Schedule tick (Go duration string, e.g. "30s").
	PollInterval string `yaml:"poll_interval"`
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

// envOverrideInt is envOverride's int counterpart. Unlike a string, not every
// value is valid, so a malformed override returns an error instead of being
// silently ignored — an env var is a boundary this project's conventions say
// should be validated, and failing fast beats surprising you with the default.
func envOverrideInt(dst *int, key string) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	*dst = n
	return nil
}

// Load reads the YAML at path, then applies env overrides for anything that is
// commonly injected by the deployment environment.
func Load(path string) (*Config, error) {
	cfg := &Config{
		Server:        ServerConfig{Port: "8080"},
		Observability: ObservabilityConfig{ServiceName: "backend"},
		Token:         TokenConfig{ExpiryHours: 1, RefreshExpiryHours: 720},
		Temporal: TemporalConfig{
			HostPort:          "localhost:7233",
			Namespace:         "default",
			TaskQueue:         "ingest",
			WorkerConcurrency: 8,
		},
		Qdrant: QdrantConfig{
			URL:        "http://localhost:6333",
			Collection: "documents",
		},
		Ollama: OllamaConfig{
			URL:             "http://localhost:11434",
			EmbeddingModel:  "nomic-embed-text",
			GenerationModel: "llama3.2",
		},
		Ingest: IngestConfig{
			WatchDir:     "./data/inbox",
			Trigger:      "schedule",
			PollInterval: "30s",
		},
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

	envOverride(&cfg.Temporal.HostPort, "TEMPORAL_HOSTPORT")
	envOverride(&cfg.Temporal.Namespace, "TEMPORAL_NAMESPACE")
	envOverride(&cfg.Temporal.TaskQueue, "TEMPORAL_TASK_QUEUE")
	if err := envOverrideInt(&cfg.Temporal.WorkerConcurrency, "TEMPORAL_WORKER_CONCURRENCY"); err != nil {
		return nil, err
	}
	envOverride(&cfg.Qdrant.URL, "QDRANT_URL")
	envOverride(&cfg.Qdrant.Collection, "QDRANT_COLLECTION")
	envOverride(&cfg.Ollama.URL, "OLLAMA_URL")
	envOverride(&cfg.Ollama.EmbeddingModel, "OLLAMA_EMBEDDING_MODEL")
	envOverride(&cfg.Ollama.GenerationModel, "OLLAMA_GENERATION_MODEL")
	envOverride(&cfg.Ingest.WatchDir, "INGEST_WATCH_DIR")
	envOverride(&cfg.Ingest.Trigger, "INGEST_TRIGGER")
	envOverride(&cfg.Ingest.PollInterval, "INGEST_POLL_INTERVAL")

	return cfg, nil
}
