// Package config provides configuration management for the inventory service.
package config

import (
	"time"

	"github.com/spf13/viper"
)

// Config holds the application configuration.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	OTLP     OTLPConfig     `mapstructure:"otlp"`
	Storage  StorageConfig  `mapstructure:"storage"`
	Pipeline PipelineConfig `mapstructure:"pipeline"`
	Semconv  SemconvConfig  `mapstructure:"semconv"`
	Log      LogConfig      `mapstructure:"log"`
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// OTLPConfig holds OTLP receiver configuration.
type OTLPConfig struct {
	GRPCAddress string `mapstructure:"grpc_address"`
	HTTPAddress string `mapstructure:"http_address"`
	Enabled     bool   `mapstructure:"enabled"`
}

// StorageConfig holds storage configuration.
type StorageConfig struct {
	Type   string `mapstructure:"type"`
	DBPath string `mapstructure:"db_path"`
}

// PipelineConfig holds classification pipeline configuration.
type PipelineConfig struct {
	Enabled       bool          `mapstructure:"enabled"`
	Workers       int           `mapstructure:"workers"`
	BatchSize     int           `mapstructure:"batch_size"`
	FlushInterval time.Duration `mapstructure:"flush_interval"`
	ChannelSize   int           `mapstructure:"channel_size"`
	CacheTTL      time.Duration `mapstructure:"cache_ttl"`
}

// SemconvConfig holds semantic conventions configuration.
type SemconvConfig struct {
	Version string `mapstructure:"version"`
}

// LogConfig holds logging configuration.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		OTLP: OTLPConfig{
			GRPCAddress: "0.0.0.0:4317",
			HTTPAddress: "0.0.0.0:4318",
			Enabled:     true,
		},
		Storage: StorageConfig{
			Type:   "sqlite",
			DBPath: "/app/data/inventory.db",
		},
		Pipeline: PipelineConfig{
			Enabled:       true,
			Workers:       4,
			BatchSize:     1000,
			FlushInterval: 200 * time.Millisecond,
			ChannelSize:   10000,
			CacheTTL:      5 * time.Minute,
		},
		Semconv: SemconvConfig{
			Version: "1.26.0",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// Load loads configuration from file and environment variables.
func Load(configPath string) (*Config, error) {
	config := DefaultConfig()

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// Environment variable overrides
	v.SetEnvPrefix("INVENTORY")
	v.AutomaticEnv()

	// Bind specific environment variables
	v.BindEnv("server.port", "PORT")
	v.BindEnv("storage.db_path", "DB_PATH")
	v.BindEnv("semconv.version", "SEMCONV_VERSION")
	v.BindEnv("pipeline.enabled", "SEMREV_ENABLED")
	v.BindEnv("pipeline.workers", "WORKERS")
	v.BindEnv("pipeline.batch_size", "BATCH_SIZE")
	v.BindEnv("pipeline.flush_interval", "FLUSH_INTERVAL_MS")
	v.BindEnv("pipeline.cache_ttl", "CACHE_TTL_SEC")
	v.BindEnv("log.level", "LOG_LEVEL")
	v.BindEnv("log.format", "LOG_FORMAT")

	if err := v.ReadInConfig(); err != nil {
		// Config file is optional
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	if err := v.Unmarshal(config); err != nil {
		return nil, err
	}

	return config, nil
}
