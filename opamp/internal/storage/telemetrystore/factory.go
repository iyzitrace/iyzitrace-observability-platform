package telemetrystore

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/getlawrence/lawrence-oss/internal/config"
	"github.com/getlawrence/lawrence-oss/internal/storage/telemetrystore/types"
)

// Factory is a placeholder for telemetry storage
type Factory struct {
	logger *zap.Logger
}

func NewFactory(appConfig *config.Config) (*Factory, error) {
	return &Factory{}, nil
}

func (f *Factory) Initialize(logger *zap.Logger) error {
	f.logger = logger
	return nil
}

func (f *Factory) CreateTelemetryReader() (types.Reader, error) {
	return nil, fmt.Errorf("telemetry reader not available in proxy mode")
}

func (f *Factory) Close() error {
	return nil
}

func NewFactoryFromAppConfig(appConfig *config.Config) (*Factory, error) {
	return NewFactory(appConfig)
}

// Config dummy
type Config struct {
	Type string
	Path string
}

func ConfigFrom(appConfig *config.Config) Config {
	return Config{}
}
