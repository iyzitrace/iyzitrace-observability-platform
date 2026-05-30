package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/getlawrence/lawrence-oss/internal/api"
	"github.com/getlawrence/lawrence-oss/internal/config"
	"github.com/getlawrence/lawrence-oss/internal/metrics"
	"github.com/getlawrence/lawrence-oss/internal/opamp"
	"github.com/getlawrence/lawrence-oss/internal/otlp/receiver"
	"github.com/getlawrence/lawrence-oss/internal/services"
	"github.com/getlawrence/lawrence-oss/internal/storage/applicationstore"
	"github.com/getlawrence/lawrence-oss/internal/utils"
	"github.com/getlawrence/lawrence-oss/internal/worker"
)

const (
	appName = "Lawrence OSS"
	version = "0.1.0"
)

func main() {
	// Create root command
	rootCmd := &cobra.Command{
		Use:   "lawrence",
		Short: "Lawrence OSS - OpenTelemetry observability platform",
		Long: `Lawrence OSS is a comprehensive observability platform that provides:
- OpenTelemetry data collection and processing
- Agent management via OpAMP protocol
- Real-time telemetry analysis
- Modern web interface for monitoring and management`,
		RunE: runLawrence,
	}

	// Add subcommands
	rootCmd.AddCommand(versionCommand())
	rootCmd.AddCommand(configCommand())

	// Add flags
	rootCmd.PersistentFlags().String("config", "./lawrence.yaml", "Path to configuration file")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("log-format", "json", "Log format (json, console)")

	// Bind flags to viper
	_ = viper.BindPFlags(rootCmd.PersistentFlags())

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func runLawrence(cmd *cobra.Command, args []string) error {
	// Load configuration
	configPath := viper.GetString("config")
	config, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize logger
	logger, err := utils.NewLogger(config.Logging.Level, config.Logging.Format)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	logger.Info("Starting Lawrence OSS",
		zap.String("version", version),
		zap.String("config", configPath))

	// Create application store using meta factory
	appStoreFactory, err := applicationstore.NewFactoryFromAppConfig(config)
	if err != nil {
		logger.Fatal("Failed to create application store factory", zap.Error(err))
	}

	// Initialize the factory
	if err := appStoreFactory.Initialize(logger); err != nil {
		logger.Fatal("Failed to initialize application store factory", zap.Error(err))
	}

	// Create application store
	appStore, err := appStoreFactory.CreateApplicationStore()
	if err != nil {
		logger.Fatal("Failed to create application store", zap.Error(err))
	}

	// Ensure application store factory is properly closed on shutdown
	defer func() {
		if err := appStoreFactory.Close(); err != nil {
			logger.Error("Failed to close application store factory", zap.Error(err))
		}
	}()

	// Initialize OTLP exporter for the worker pool
	telemetryExporter := worker.NewOTLPExporter(config.ExternalPlatform.OTLPExporterURL, logger)

	registry := prometheus.NewRegistry()
	metricsFactory := metrics.NewPrometheusFactory("lawrence", registry)
	opampMetrics := metrics.NewOpAMPMetrics(metricsFactory)
	otlpMetrics := metrics.NewOTLPMetrics(metricsFactory)

	agents := opamp.NewAgents(logger)

	// Determine which OTLP endpoints to offer to agents
	agentGRPCEndpoint := config.OTLP.AgentGRPCEndpoint
	if agentGRPCEndpoint == "" {
		agentGRPCEndpoint = config.OTLP.GRPCEndpoint
	}
	agentHTTPEndpoint := config.OTLP.AgentHTTPEndpoint
	if agentHTTPEndpoint == "" {
		agentHTTPEndpoint = config.OTLP.HTTPEndpoint
	}

	// Create agent service
	agentService := services.NewAgentService(appStore, logger)

	// Create config sender
	configSender := opamp.NewConfigSender(agents, logger)

	// Create OpAMP server
	opampServer, err := opamp.NewServer(agents, agentService, opampMetrics, agentGRPCEndpoint, agentHTTPEndpoint, logger)
	if err != nil {
		logger.Fatal("Failed to create OpAMP server", zap.Error(err))
	}

	// Create telemetry query service (Proxy mode)
	telemetryService := services.NewTelemetryQueryService(config, agentService, logger)

	// Process worker pool timeout
	workerTimeout, err := time.ParseDuration(config.Worker.Timeout)
	if err != nil {
		workerTimeout = 5 * time.Second
	}

	// Initialize worker pool for async telemetry forwarding (using OTLP exporter)
	workerPool := worker.NewPool(config.Worker.QueueSize, config.Worker.Workers, workerTimeout, telemetryExporter, agentService, logger)
	workerPool.Start()
	defer func() {
		if err := workerPool.Stop(30 * time.Second); err != nil {
			logger.Error("Failed to stop worker pool", zap.Error(err))
		}
	}()

	// Start OpAMP server
	if err := opampServer.Start(config.Server.OpAMPPort); err != nil {
		logger.Fatal("Failed to start OpAMP server", zap.Error(err))
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = opampServer.Stop(ctx)
	}()

	// Initialize OTLP receivers
	grpcServer, err := receiver.NewGRPCServer(4317, otlpMetrics, workerPool, logger)
	if err != nil {
		logger.Fatal("Failed to create gRPC server", zap.Error(err))
	}
	if err := grpcServer.Start(); err != nil {
		logger.Fatal("Failed to start gRPC server", zap.Error(err))
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = grpcServer.Stop(ctx)
	}()

	httpServer, err := receiver.NewHTTPServer(4318, otlpMetrics, workerPool, logger)
	if err != nil {
		logger.Fatal("Failed to create HTTP server", zap.Error(err))
	}
	if err := httpServer.Start(); err != nil {
		logger.Fatal("Failed to start HTTP server", zap.Error(err))
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Stop(ctx)
	}()

	// Initialize HTTP API server
	apiServer := api.NewServer(agentService, telemetryService, configSender, logger)

	// Start API server
	go func() {
		if err := apiServer.Start(fmt.Sprintf("%d", config.Server.HTTPPort)); err != nil {
			logger.Fatal("Failed to start API server", zap.Error(err))
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = apiServer.Stop(ctx)
	}()

	logger.Info("Lawrence OSS is running in Proxy mode",
		zap.Int("opamp_port", config.Server.OpAMPPort),
		zap.Int("api_port", config.Server.HTTPPort))

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down Lawrence OSS...")
	return nil
}

// versionCommand returns the version subcommand
func versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s v%s\n", appName, version)
		},
	}
}

// configCommand returns the config subcommand
func configCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Print current configuration",
		Run: func(cmd *cobra.Command, args []string) {
			configPath := viper.GetString("config")
			_, err := config.LoadConfig(configPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
				os.Exit(1)
			}
			// TODO: Pretty print configuration
			fmt.Printf("Configuration loaded from: %s\n", configPath)
		},
	}
}
