// Command worker hosts the Temporal worker: it registers the ingestion workflow
// and activities, and (when configured) creates the polling Schedule that drives
// folder scans. Run it alongside the HTTP server:
//
//	go run ./cmd/worker
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/tejesh-kaliki/temporal-test/backend/internal/config"
	"github.com/tejesh-kaliki/temporal-test/backend/internal/ingest"
	"github.com/tejesh-kaliki/temporal-test/backend/internal/migrate"
)

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config/env.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()

	// Apply migrations so the ingestion ledger table exists. Idempotent — goose
	// skips already-applied versions.
	if err := migrate.Up(ctx, cfg.Database.URL, "", "sql/schema"); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	pool, err := pgxpool.New(ctx, cfg.Database.URL)
	if err != nil {
		log.Fatalf("db pool: %v", err)
	}
	defer pool.Close()

	c, err := client.Dial(client.Options{
		HostPort:  cfg.Temporal.HostPort,
		Namespace: cfg.Temporal.Namespace,
	})
	if err != nil {
		log.Fatalf("temporal dial: %v", err)
	}
	defer c.Close()

	acts := &ingest.Activities{
		Source: ingest.FolderSource{Dir: cfg.Ingest.WatchDir},
		Ollama: ingest.NewOllamaClient(cfg.Ollama.URL, cfg.Ollama.EmbeddingModel, cfg.Ollama.GenerationModel, nil),
		Qdrant: ingest.NewQdrantClient(cfg.Qdrant.URL, cfg.Qdrant.Collection, nil),
		Store:  ingest.NewStore(pool),
	}

	w := worker.New(c, cfg.Temporal.TaskQueue, worker.Options{})
	w.RegisterWorkflow(ingest.SyncCatalogWorkflow)
	w.RegisterWorkflow(ingest.IngestDocumentWorkflow)
	w.RegisterWorkflow(ingest.PurgeDocumentWorkflow)
	w.RegisterActivity(acts)

	if err := ensureSchedule(ctx, c, cfg); err != nil {
		log.Fatalf("schedule: %v", err)
	}

	if err := w.Start(); err != nil {
		log.Fatalf("worker start: %v", err)
	}
	log.Printf("worker listening on task queue %q", cfg.Temporal.TaskQueue)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down worker...")
	w.Stop()
}

// ensureSchedule creates (or leaves in place) the polling Schedule that fires
// ScanFolderWorkflow every PollInterval. Idempotent: an existing Schedule with
// the same ID is treated as success. Trigger modes other than "schedule"/"both"
// skip creation (fsnotify is a later add-on).
func ensureSchedule(ctx context.Context, c client.Client, cfg *config.Config) error {
	switch cfg.Ingest.Trigger {
	case "schedule", "both":
	default:
		log.Printf("ingest trigger=%q — no Schedule created", cfg.Ingest.Trigger)
		return nil
	}

	interval, err := time.ParseDuration(cfg.Ingest.PollInterval)
	if err != nil {
		return err
	}

	handle := c.ScheduleClient().GetHandle(ctx, ingest.ScheduleID)
	if _, err := handle.Describe(ctx); err == nil {
		log.Printf("schedule %q already exists", ingest.ScheduleID)
		return nil
	}

	_, err = c.ScheduleClient().Create(ctx, client.ScheduleOptions{
		ID:   ingest.ScheduleID,
		Spec: client.ScheduleSpec{Intervals: []client.ScheduleIntervalSpec{{Every: interval}}},
		Action: &client.ScheduleWorkflowAction{
			ID:        "scan-folder",
			Workflow:  ingest.SyncCatalogWorkflow,
			TaskQueue: cfg.Temporal.TaskQueue,
		},
	})
	if err != nil {
		return err
	}
	log.Printf("created schedule %q every %s over %s", ingest.ScheduleID, interval, cfg.Ingest.WatchDir)
	return nil
}
