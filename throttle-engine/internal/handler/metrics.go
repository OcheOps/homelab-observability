package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// MetricsHandler proxies Prometheus queries for the dashboard charts.
type MetricsHandler struct {
	prometheusURL string
	client        *http.Client
}

func NewMetricsHandler(promURL string) *MetricsHandler {
	return &MetricsHandler{
		prometheusURL: promURL,
		client:        &http.Client{Timeout: 10 * time.Second},
	}
}

// HandleBandwidth returns bandwidth data for all physical interfaces over the last hour.
func (m *MetricsHandler) HandleBandwidth(w http.ResponseWriter, r *http.Request) {
	duration := r.URL.Query().Get("duration")
	if duration == "" {
		duration = "1h"
	}
	step := r.URL.Query().Get("step")
	if step == "" {
		step = "30s"
	}

	queries := map[string]string{
		"rx": `rate(node_network_receive_bytes_total{device!~"lo|docker.*|br-.*|veth.*"}[2m])`,
		"tx": `rate(node_network_transmit_bytes_total{device!~"lo|docker.*|br-.*|veth.*"}[2m])`,
	}

	result := make(map[string]interface{})
	for key, query := range queries {
		data, err := m.queryRange(query, duration, step)
		if err != nil {
			result[key] = map[string]string{"error": err.Error()}
			continue
		}
		result[key] = data
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleSystemStats returns current CPU, memory, disk usage.
func (m *MetricsHandler) HandleSystemStats(w http.ResponseWriter, r *http.Request) {
	queries := map[string]string{
		"cpu":        `100 - (avg(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)`,
		"memory":     `(1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100`,
		"disk":       `(node_filesystem_avail_bytes{fstype!~"tmpfs|overlay",mountpoint="/"} / node_filesystem_size_bytes) * 100`,
		"connections": `node_netstat_Tcp_CurrEstab`,
	}

	result := make(map[string]interface{})
	for key, query := range queries {
		data, err := m.queryInstant(query)
		if err != nil {
			result[key] = map[string]string{"error": err.Error()}
			continue
		}
		result[key] = data
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (m *MetricsHandler) queryRange(query, duration, step string) (interface{}, error) {
	end := time.Now()
	start := end.Add(-parseDuration(duration))

	params := url.Values{}
	params.Set("query", query)
	params.Set("start", fmt.Sprintf("%d", start.Unix()))
	params.Set("end", fmt.Sprintf("%d", end.Unix()))
	params.Set("step", step)

	resp, err := m.client.Get(fmt.Sprintf("%s/api/v1/query_range?%s", m.prometheusURL, params.Encode()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result interface{}
	json.Unmarshal(body, &result)
	return result, nil
}

func (m *MetricsHandler) queryInstant(query string) (interface{}, error) {
	params := url.Values{}
	params.Set("query", query)

	resp, err := m.client.Get(fmt.Sprintf("%s/api/v1/query?%s", m.prometheusURL, params.Encode()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result interface{}
	json.Unmarshal(body, &result)
	return result, nil
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Hour
	}
	return d
}
