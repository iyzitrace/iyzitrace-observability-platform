// Package main provides the entry point for the inventory service.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/iyzitrace/inventory-service/internal/api/handlers"
	"github.com/iyzitrace/inventory-service/internal/config"
	"github.com/iyzitrace/inventory-service/internal/otlp"
	"github.com/iyzitrace/inventory-service/internal/semrev/adapter"
	"github.com/iyzitrace/inventory-service/internal/semrev/pipeline"
	"github.com/iyzitrace/inventory-service/internal/storage"
	"github.com/iyzitrace/inventory-service/internal/utils"
	"github.com/iyzitrace/inventory-service/internal/worker"
)

func main() {
	// Load configuration
	configPath := utils.GetEnvOrDefault("CONFIG_PATH", "config.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		// Use default config if loading fails
		cfg = config.DefaultConfig()
	}

	// Initialize logger
	logger, err := utils.NewLogger(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting Inventory Service",
		zap.String("semconv_version", cfg.Semconv.Version),
		zap.Bool("pipeline_enabled", cfg.Pipeline.Enabled),
	)

	// Initialize storage
	store, err := storage.NewSQLiteStore(cfg.Storage.DBPath, logger)
	if err != nil {
		logger.Fatal("Failed to initialize storage", zap.Error(err))
	}
	defer store.Close()

	logger.Info("Storage initialized", zap.String("type", cfg.Storage.Type), zap.String("path", cfg.Storage.DBPath))

	// Initialize semantic conventions adapter
	semconvAdapter := adapter.GetAdapter(cfg.Semconv.Version)
	logger.Info("Semconv adapter initialized", zap.String("version", semconvAdapter.Version()))

	// Initialize classification pipeline
	var classificationPipeline *pipeline.Pipeline
	var otlpReceiver *otlp.Receiver

	if cfg.Pipeline.Enabled {
		pipelineCfg := pipeline.Config{
			Workers:       cfg.Pipeline.Workers,
			BatchSize:     cfg.Pipeline.BatchSize,
			FlushInterval: cfg.Pipeline.FlushInterval,
			ChannelSize:   cfg.Pipeline.ChannelSize,
		}

		classificationPipeline = pipeline.NewPipeline(pipelineCfg, store, semconvAdapter, logger)
		classificationPipeline.Start()
		defer classificationPipeline.Stop()

		logger.Info("Classification pipeline started",
			zap.Int("workers", cfg.Pipeline.Workers),
			zap.Int("batch_size", cfg.Pipeline.BatchSize),
		)

		// Initialize OTLP receiver
		if cfg.OTLP.Enabled {
			otlpReceiver = otlp.NewReceiver(classificationPipeline, logger)
			if err := otlpReceiver.Start(cfg.OTLP.GRPCAddress); err != nil {
				logger.Fatal("Failed to start OTLP receiver", zap.Error(err))
			}
			defer otlpReceiver.Stop()

			logger.Info("OTLP receiver started", zap.String("grpc_address", cfg.OTLP.GRPCAddress))
		}
	}

	// Start staleness worker to track entity lifecycle
	stalenessWorker := worker.NewStalenessWorker(worker.DefaultStalenessConfig(), store, logger)
	stalenessWorker.Start()
	defer stalenessWorker.Stop()

	// Initialize HTTP server
	if os.Getenv("GIN_MODE") != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())

	// Register API handlers
	apiHandlers := handlers.NewHandlers(store, logger)
	apiHandlers.RegisterRoutes(router)

	// Health endpoint at root level
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "healthy",
			"service": "inventory-service",
		})
	})

	serverAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:    serverAddr,
		Handler: router,
	}

	// Start HTTP server
	go func() {
		logger.Info("Starting HTTP server", zap.String("address", serverAddr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP server error", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}
