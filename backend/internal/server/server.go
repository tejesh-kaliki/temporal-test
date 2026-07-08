// Package server wires config, observability, the database pool and the HTTP
// router together. New domains register their routes in Run().
package server

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/tejesh-kaliki/temporal-test/backend/internal/config"
	"github.com/tejesh-kaliki/temporal-test/backend/internal/observability"
)

func Run() {
	ctx := context.Background()

	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config/env.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	obs := observability.Init(cfg.Observability.ServiceName, cfg.Observability.Endpoint)
	defer obs.Shutdown()

	r := gin.Default()
	r.Use(corsMiddleware())
	r.Use(otelgin.Middleware(cfg.Observability.ServiceName))
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Server.Port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Run until SIGINT/SIGTERM, then drain in-flight requests before exiting.
	shutdownCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-shutdownCtx.Done()
	log.Println("shutting down...")
	drainCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(drainCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}

// corsMiddleware enables cross-origin browser requests (e.g. the Flutter web
// app calling the API from a different origin). In development — APP_ENV unset
// or anything other than "production" — it reflects any Origin so the frontend
// works with zero configuration. In production it only allows origins listed in
// CORS_ALLOWED_ORIGINS (comma-separated); anything else gets no CORS headers.
func corsMiddleware() gin.HandlerFunc {
	dev := os.Getenv("APP_ENV") != "production"
	allowed := map[string]bool{}
	for _, o := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = true
		}
	}
	return func(c *gin.Context) {
		if origin := c.GetHeader("Origin"); origin != "" && (dev || allowed[origin]) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
