// Package worker provides background workers for the inventory service.
package worker

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/iyzitrace/inventory-service/internal/storage"
)

// StalenessConfig configures the staleness worker.
type StalenessConfig struct {
	// CheckInterval is how often to check for stale entities
	CheckInterval time.Duration `mapstructure:"check_interval"`
	// StaleThreshold is how long since last_seen before marking as stale
	StaleThreshold time.Duration `mapstructure:"stale_threshold"`
	// Enabled controls whether the worker runs
	Enabled bool `mapstructure:"enabled"`
}

// DefaultStalenessConfig returns sensible defaults.
func DefaultStalenessConfig() StalenessConfig {
	return StalenessConfig{
		CheckInterval:  1 * time.Minute,
		StaleThreshold: 5 * time.Minute,
		Enabled:        true,
	}
}

// StalenessWorker periodically checks and marks stale entities.
type StalenessWorker struct {
	config StalenessConfig
	store  storage.Store
	logger *zap.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewStalenessWorker creates a new staleness worker.
func NewStalenessWorker(config StalenessConfig, store storage.Store, logger *zap.Logger) *StalenessWorker {
	ctx, cancel := context.WithCancel(context.Background())
	return &StalenessWorker{
		config: config,
		store:  store,
		logger: logger.Named("staleness-worker"),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins the staleness check loop.
func (w *StalenessWorker) Start() {
	if !w.config.Enabled {
		w.logger.Info("Staleness worker disabled")
		return
	}

	w.logger.Info("Starting staleness worker",
		zap.Duration("check_interval", w.config.CheckInterval),
		zap.Duration("stale_threshold", w.config.StaleThreshold),
	)

	w.wg.Add(1)
	go w.run()
}

// Stop stops the staleness worker.
func (w *StalenessWorker) Stop() {
	w.logger.Info("Stopping staleness worker")
	w.cancel()
	w.wg.Wait()
	w.logger.Info("Staleness worker stopped")
}

func (w *StalenessWorker) run() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.config.CheckInterval)
	defer ticker.Stop()

	// Run immediately on start
	w.checkStaleness()

	for {
		select {
		case <-ticker.C:
			w.checkStaleness()
		case <-w.ctx.Done():
			return
		}
	}
}

func (w *StalenessWorker) checkStaleness() {
	threshold := time.Now().Add(-w.config.StaleThreshold)

	// Mark stale entities
	entityCount, err := w.store.MarkStaleEntities(w.ctx, threshold)
	if err != nil {
		w.logger.Error("Failed to mark stale entities", zap.Error(err))
	} else if entityCount > 0 {
		w.logger.Info("Marked entities as stale", zap.Int64("count", entityCount))
	}

	// Mark stale instances within entities
	instanceCount, err := w.store.MarkInstancesStale(w.ctx, threshold)
	if err != nil {
		w.logger.Error("Failed to mark stale instances", zap.Error(err))
	} else if instanceCount > 0 {
		w.logger.Info("Updated entities with stale instances", zap.Int64("count", instanceCount))
	}
}
