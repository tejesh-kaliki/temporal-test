package config

import "testing"

func TestLoad_WorkerConcurrencyDefault(t *testing.T) {
	cfg, err := Load("testdata/does-not-exist.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Temporal.WorkerConcurrency != 8 {
		t.Fatalf("expected default worker concurrency 8, got %d", cfg.Temporal.WorkerConcurrency)
	}
}

func TestLoad_WorkerConcurrencyEnvOverride(t *testing.T) {
	t.Setenv("TEMPORAL_WORKER_CONCURRENCY", "3")

	cfg, err := Load("testdata/does-not-exist.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Temporal.WorkerConcurrency != 3 {
		t.Fatalf("expected env override to set worker concurrency to 3, got %d", cfg.Temporal.WorkerConcurrency)
	}
}

func TestLoad_WorkerConcurrencyInvalidEnv(t *testing.T) {
	t.Setenv("TEMPORAL_WORKER_CONCURRENCY", "not-a-number")

	if _, err := Load("testdata/does-not-exist.yaml"); err == nil {
		t.Fatal("expected an error for a non-numeric TEMPORAL_WORKER_CONCURRENCY, got nil")
	}
}
