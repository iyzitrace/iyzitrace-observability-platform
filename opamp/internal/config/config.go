package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Server           ServerConfig           `yaml:"server"`
	OTLP             OTLPConfig             `yaml:"otlp"`
	AppStorage       AppStorageConfig       `yaml:"app_storage"`
	ExternalPlatform ExternalPlatformConfig `yaml:"external_platform"`
	Logging          LoggingConfig          `yaml:"logging"`
	Worker           WorkerConfig           `yaml:"worker"`
}

// ServerConfig contains server configuration
type ServerConfig struct {
	HTTPPort  int `yaml:"http_port"`
	OpAMPPort int `yaml:"opamp_port"`
}

// OTLPConfig contains OTLP receiver configuration
type OTLPConfig struct {
	GRPCEndpoint      string `yaml:"grpc_endpoint"`
	HTTPEndpoint      string `yaml:"http_endpoint"`
	AgentGRPCEndpoint string `yaml:"agent_grpc_endpoint"` // Endpoint to offer to agents
	AgentHTTPEndpoint string `yaml:"agent_http_endpoint"` // Endpoint to offer to agents
}

// AppStorageConfig contains app storage configuration
type AppStorageConfig struct {
	Type string `yaml:"type"`
	Path string `yaml:"path"`
}

// ExternalPlatformConfig contains configuration for the external observability platform
type ExternalPlatformConfig struct {
	PrometheusURL   string `yaml:"prometheus_url"`
	LokiURL         string `yaml:"loki_url"`
	TempoURL        string `yaml:"tempo_url"`
	OTLPExporterURL string `yaml:"otlp_exporter_url"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// WorkerConfig contains worker pool configuration
type WorkerConfig struct {
	QueueSize int    `yaml:"queue_size"`
	Workers   int    `yaml:"workers"`
	Timeout   string `yaml:"timeout"` // Duration string like "5s", "1m"
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Parse YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			HTTPPort:  8080,
			OpAMPPort: 4320,
		},
		OTLP: OTLPConfig{
			GRPCEndpoint: "0.0.0.0:4317",
			HTTPEndpoint: "0.0.0.0:4318",
		},
		AppStorage: AppStorageConfig{
			Type: "sqlite",
			Path: "./data/app.db",
		},
		ExternalPlatform: ExternalPlatformConfig{
			PrometheusURL:   "http://localhost:9090",
			LokiURL:         "http://localhost:3100",
			OTLPExporterURL: "http://localhost:4317",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Worker: WorkerConfig{
			QueueSize: 10000,
			Workers:   3,
			Timeout:   "5s",
		},
	}
}

// ParseDuration parses a duration string like "24h", "7d", "30d"
func ParseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid duration format: %s", s)
	}

	unit := s[len(s)-1:]
	value := s[:len(s)-1]

	var duration time.Duration
	switch unit {
	case "h":
		d, err := time.ParseDuration(value + "h")
		if err != nil {
			return 0, err
		}
		duration = d
	case "d":
		// Parse days as integer
		var days int
		if _, err := fmt.Sscanf(value, "%d", &days); err != nil {
			return 0, fmt.Errorf("invalid day value: %s", value)
		}
		duration = time.Duration(days*24) * time.Hour
	default:
		return time.ParseDuration(s)
	}

	return duration, nil
}
