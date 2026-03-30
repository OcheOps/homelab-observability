package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/oche/homelab-observability/throttle-engine/internal/config"
	"github.com/oche/homelab-observability/throttle-engine/internal/inventory"
)

// AlertmanagerPayload matches the webhook JSON that Alertmanager sends.
type AlertmanagerPayload struct {
	Version     string  `json:"version"`
	GroupKey    string  `json:"groupKey"`
	Status      string  `json:"status"`
	Receiver    string  `json:"receiver"`
	Alerts      []Alert `json:"alerts"`
	ExternalURL string  `json:"externalURL"`
}

type Alert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
}

// AlertRecord is a processed alert stored in history.
type AlertRecord struct {
	Timestamp  time.Time `json:"timestamp"`
	AlertName  string    `json:"alert_name"`
	Status     string    `json:"status"`
	Severity   string    `json:"severity"`
	Device     string    `json:"device"`
	Instance   string    `json:"instance"`
	DeviceName string    `json:"device_name"`
	Owner      string    `json:"owner"`
	Summary    string    `json:"summary"`
	Action     string    `json:"action"`
}

// WebhookHandler processes incoming alerts from Alertmanager.
type WebhookHandler struct {
	cfg       *config.Config
	inventory *inventory.Inventory
	logger    *slog.Logger

	mu       sync.RWMutex
	history  []AlertRecord
	maxHist  int
	stats    Stats
}

type Stats struct {
	TotalReceived int `json:"total_received"`
	TotalFiring   int `json:"total_firing"`
	TotalResolved int `json:"total_resolved"`
	StartedAt     time.Time `json:"started_at"`
}

func NewWebhookHandler(cfg *config.Config, inv *inventory.Inventory, logger *slog.Logger) *WebhookHandler {
	return &WebhookHandler{
		cfg:       cfg,
		inventory: inv,
		logger:    logger,
		history:   make([]AlertRecord, 0, 200),
		maxHist:   200,
		stats:     Stats{StartedAt: time.Now()},
	}
}

// GetHistory returns a copy of the alert history (most recent first).
func (h *WebhookHandler) GetHistory() []AlertRecord {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]AlertRecord, len(h.history))
	// Reverse so newest is first
	for i, rec := range h.history {
		out[len(h.history)-1-i] = rec
	}
	return out
}

// GetStats returns current stats.
func (h *WebhookHandler) GetStats() Stats {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.stats
}

func (h *WebhookHandler) addRecord(rec AlertRecord) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.history) >= h.maxHist {
		// Drop oldest 50 entries when full
		h.history = h.history[50:]
	}
	h.history = append(h.history, rec)
	h.stats.TotalReceived++
	if rec.Status == "firing" {
		h.stats.TotalFiring++
	} else {
		h.stats.TotalResolved++
	}
}

// HandleAlert is the HTTP handler for POST /api/v1/alert
func (h *WebhookHandler) HandleAlert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload AlertmanagerPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.logger.Error("failed to decode alert payload", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	h.logger.Info("received alert webhook",
		"status", payload.Status,
		"alert_count", len(payload.Alerts),
		"receiver", payload.Receiver,
	)

	for _, alert := range payload.Alerts {
		h.processAlert(alert)
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"received","processed":%d}`, len(payload.Alerts))
}

func (h *WebhookHandler) processAlert(alert Alert) {
	alertName := alert.Labels["alertname"]
	instance := alert.Labels["instance"]
	device := alert.Labels["device"]
	severity := alert.Labels["severity"]

	record := AlertRecord{
		Timestamp: time.Now(),
		AlertName: alertName,
		Status:    alert.Status,
		Severity:  severity,
		Device:    device,
		Instance:  instance,
		Summary:   alert.Annotations["summary"],
		Action:    "LOG_ONLY",
	}

	logger := h.logger.With(
		"alert", alertName,
		"status", alert.Status,
		"instance", instance,
		"device", device,
		"severity", severity,
	)

	sourceIP := extractIP(instance)
	if sourceIP != "" {
		if dev, found := h.inventory.Lookup(sourceIP); found {
			record.DeviceName = dev.Name
			record.Owner = dev.Owner
			logger = logger.With(
				"device_name", dev.Name,
				"device_owner", dev.Owner,
				"device_type", dev.Type,
			)
			logger.Warn("ALERT: known device triggering bandwidth threshold",
				"action", "LOG_ONLY",
			)
		} else {
			logger.Warn("ALERT: unknown device triggering bandwidth threshold",
				"source_ip", sourceIP,
				"action", "LOG_ONLY",
			)
		}
	}

	if alert.Status == "firing" {
		logger.Warn("BANDWIDTH ALERT FIRING",
			"summary", alert.Annotations["summary"],
			"description", alert.Annotations["description"],
		)
		logger.Info("ACTION: would throttle offender (simulated)",
			"threshold_bytes", h.cfg.Thresholds.BandwidthBytesPerSec,
			"note", "OPNsense integration not yet active",
		)
	} else if alert.Status == "resolved" {
		record.Action = "RESOLVED"
		logger.Info("ALERT RESOLVED",
			"summary", alert.Annotations["summary"],
		)
	}

	h.addRecord(record)
}

func extractIP(s string) string {
	if s == "" {
		return ""
	}
	if idx := strings.LastIndex(s, ":"); idx > 0 {
		candidate := s[:idx]
		if looksLikeIP(candidate) {
			return candidate
		}
	}
	if looksLikeIP(s) {
		return s
	}
	return ""
}

func looksLikeIP(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if len(p) == 0 || len(p) > 3 {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}
