package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/oche/homelab-observability/throttle-engine/internal/config"
	"github.com/oche/homelab-observability/throttle-engine/internal/handler"
	"github.com/oche/homelab-observability/throttle-engine/internal/inventory"
)

func main() {
	cfg, err := config.Load("/app/config.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	var logLevel slog.Level
	switch cfg.Logging.Level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	logger.Info("starting throttle engine",
		"listen_addr", cfg.Server.ListenAddr,
		"bw_threshold_bytes", cfg.Thresholds.BandwidthBytesPerSec,
	)

	inv, err := inventory.Load(cfg.Inventory.Path)
	if err != nil {
		logger.Warn("could not load device inventory, continuing without it", "error", err)
		inv = &inventory.Inventory{}
	} else {
		logger.Info("device inventory loaded", "device_count", inv.Count())
	}

	webhookHandler := handler.NewWebhookHandler(cfg, inv, logger)
	uiHandler := handler.NewUIHandler(webhookHandler, inv)
	mux := http.NewServeMux()

	// Dashboard UI
	mux.HandleFunc("/", uiHandler.HandleDashboard)

	// API endpoints
	mux.HandleFunc("/api/v1/alert", webhookHandler.HandleAlert)
	mux.HandleFunc("/api/v1/alerts", uiHandler.HandleAPIAlerts)
	mux.HandleFunc("/api/v1/stats", uiHandler.HandleAPIStats)
	mux.HandleFunc("/api/v1/devices", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(inv.AllDevices())
	})
	mux.HandleFunc("/api/v1/devices/update", uiHandler.HandleUpdateDevice)
	mux.HandleFunc("/api/v1/scan", uiHandler.HandleNetworkScan)

	// Health check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"service": "throttle-engine",
		})
	})

	server := &http.Server{
		Addr:         cfg.Server.ListenAddr,
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  60 * time.Second,
	}

	logger.Info("throttle engine ready", "addr", cfg.Server.ListenAddr)
	if err := server.ListenAndServe(); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
