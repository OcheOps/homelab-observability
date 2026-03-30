package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Logging    LoggingConfig    `yaml:"logging"`
	Thresholds ThresholdConfig  `yaml:"thresholds"`
	Inventory  InventoryConfig  `yaml:"inventory"`
}

type ServerConfig struct {
	ListenAddr   string        `yaml:"listen_addr"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type ThresholdConfig struct {
	BandwidthBytesPerSec   int64 `yaml:"bandwidth_bytes_per_sec"`
	AlertCountBeforeAction int   `yaml:"alert_count_before_action"`
}

type InventoryConfig struct {
	Path string `yaml:"path"`
}

// Load reads config from a YAML file and applies environment variable overrides.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Environment variable overrides take precedence over file values
	if v := os.Getenv("THROTTLE_LISTEN_ADDR"); v != "" {
		cfg.Server.ListenAddr = v
	}
	if v := os.Getenv("THROTTLE_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("THROTTLE_BW_THRESHOLD"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Thresholds.BandwidthBytesPerSec = n
		}
	}

	return cfg, nil
}
