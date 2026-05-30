// Package pipeline implements the classification processing pipeline.
package pipeline

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/iyzitrace/inventory-service/internal/semrev/adapter"
	"github.com/iyzitrace/inventory-service/internal/semrev/detectors"
	"github.com/iyzitrace/inventory-service/internal/semrev/model"
	"github.com/iyzitrace/inventory-service/internal/storage"
)

// Config holds pipeline configuration.
type Config struct {
	Workers       int           `mapstructure:"workers"`
	BatchSize     int           `mapstructure:"batch_size"`
	FlushInterval time.Duration `mapstructure:"flush_interval"`
	ChannelSize   int           `mapstructure:"channel_size"`
}

// DefaultConfig returns the default pipeline configuration.
func DefaultConfig() Config {
	return Config{
		Workers:       4,
		BatchSize:     1000,
		FlushInterval: 200 * time.Millisecond,
		ChannelSize:   10000,
	}
}

// Pipeline processes telemetry items and classifies entities/relations.
type Pipeline struct {
	config   Config
	registry *detectors.DetectorRegistry
	store    storage.Store
	logger   *zap.Logger

	itemCh   chan *model.TelemetryItem
	resultCh chan *model.ClassificationResult

	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// NewPipeline creates a new processing pipeline.
func NewPipeline(cfg Config, store storage.Store, semconvAdapter adapter.SemconvAdapter, logger *zap.Logger) *Pipeline {
	ctx, cancel := context.WithCancel(context.Background())

	return &Pipeline{
		config:   cfg,
		registry: detectors.NewDetectorRegistry(semconvAdapter),
		store:    store,
		logger:   logger.Named("pipeline"),
		itemCh:   make(chan *model.TelemetryItem, cfg.ChannelSize),
		resultCh: make(chan *model.ClassificationResult, cfg.ChannelSize),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start starts the pipeline workers.
func (p *Pipeline) Start() {
	p.logger.Info("Starting pipeline",
		zap.Int("workers", p.config.Workers),
		zap.Int("batch_size", p.config.BatchSize),
		zap.Duration("flush_interval", p.config.FlushInterval),
	)

	// Start classification workers
	for i := 0; i < p.config.Workers; i++ {
		p.wg.Add(1)
		go p.classifyWorker(i)
	}

	// Start writer worker
	p.wg.Add(1)
	go p.writerWorker()
}

// Stop stops the pipeline and waits for all workers to finish.
func (p *Pipeline) Stop() {
	p.logger.Info("Stopping pipeline")
	p.cancel()
	close(p.itemCh)
	p.wg.Wait()
	close(p.resultCh)
	p.logger.Info("Pipeline stopped")
}

// Submit submits a telemetry item for processing.
// Returns false if the channel is full (backpressure).
func (p *Pipeline) Submit(item *model.TelemetryItem) bool {
	select {
	case p.itemCh <- item:
		return true
	default:
		return false
	}
}

// SubmitBlocking submits a telemetry item, blocking if the channel is full.
func (p *Pipeline) SubmitBlocking(ctx context.Context, item *model.TelemetryItem) error {
	select {
	case p.itemCh <- item:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-p.ctx.Done():
		return p.ctx.Err()
	}
}

// classifyWorker runs classification on items from the channel.
func (p *Pipeline) classifyWorker(id int) {
	defer p.wg.Done()

	logger := p.logger.With(zap.Int("worker_id", id))
	logger.Debug("Classification worker started")

	for item := range p.itemCh {
		result := p.registry.Classify(item)
		if len(result.Entities) > 0 || len(result.Relations) > 0 {
			select {
			case p.resultCh <- result:
			case <-p.ctx.Done():
				return
			}
		}
	}

	logger.Debug("Classification worker stopped")
}

// writerWorker batches and writes classification results to storage.
func (p *Pipeline) writerWorker() {
	defer p.wg.Done()

	logger := p.logger.Named("writer")
	logger.Debug("Writer worker started")

	entityBatch := make([]*model.Entity, 0, p.config.BatchSize)
	relationBatch := make([]*model.Relation, 0, p.config.BatchSize)
	ticker := time.NewTicker(p.config.FlushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(entityBatch) > 0 {
			if err := p.store.UpsertEntities(p.ctx, entityBatch); err != nil {
				logger.Error("Failed to upsert entities", zap.Error(err))
			} else {
				logger.Debug("Flushed entities", zap.Int("count", len(entityBatch)))
			}
			entityBatch = entityBatch[:0]
		}

		if len(relationBatch) > 0 {
			if err := p.store.UpsertRelations(p.ctx, relationBatch); err != nil {
				logger.Error("Failed to upsert relations", zap.Error(err))
			} else {
				logger.Debug("Flushed relations", zap.Int("count", len(relationBatch)))
			}
			relationBatch = relationBatch[:0]
		}
	}

	for {
		select {
		case result, ok := <-p.resultCh:
			if !ok {
				flush()
				logger.Debug("Writer worker stopped")
				return
			}

			entityBatch = append(entityBatch, result.Entities...)
			relationBatch = append(relationBatch, result.Relations...)

			// Flush if batch size reached
			if len(entityBatch) >= p.config.BatchSize || len(relationBatch) >= p.config.BatchSize {
				flush()
			}

		case <-ticker.C:
			flush()

		case <-p.ctx.Done():
			flush()
			logger.Debug("Writer worker stopped (context cancelled)")
			return
		}
	}
}
