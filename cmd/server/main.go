package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/dasmlab/iskoces/pkg/proto/v1"
	"github.com/dasmlab/iskoces/pkg/service"
	"github.com/dasmlab/iskoces/pkg/translate"
	"github.com/sirupsen/logrus"
)

var (
	// Server configuration flags
	port         = flag.Int("port", 50051, "gRPC server port")
	insecureMode = flag.Bool("insecure", true, "Run server in insecure mode (no TLS)")

	// Translation engine configuration
	mtEngine = flag.String("mt-engine", "libretranslate", "Translation engine: libretranslate or argos")
	mtURL    = flag.String("mt-url", "http://localhost:5000", "Base URL for translation engine API")

	// TLS configuration flags (for future use)
	tlsCertPath = flag.String("tls-cert", "", "Path to TLS server certificate")
	tlsKeyPath  = flag.String("tls-key", "", "Path to TLS server private key")
	tlsCAPath   = flag.String("tls-ca", "", "Path to CA certificate for client verification (mTLS)")

	// Logging configuration
	logLevel = flag.String("log-level", "info", "Log level: debug, info, warn, error")
)

func main() {
	flag.Parse()

	// Initialize logger
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		TimestampFormat: time.RFC3339,
	})

	// Set log level
	level, err := logrus.ParseLevel(*logLevel)
	if err != nil {
		logger.WithError(err).Warn("Invalid log level, using info")
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	logger.WithFields(logrus.Fields{
		"port":      *port,
		"insecure":  *insecureMode,
		"mt_engine": *mtEngine,
		"mt_url":    *mtURL,
		"log_level": level.String(),
	}).Info("Starting Iskoces gRPC server")

	// Parse translation engine type
	engineType, err := translate.ParseEngineType(*mtEngine)
	if err != nil {
		logger.WithError(err).Fatal("Failed to parse translation engine type")
	}

	// Create translator instance
	translator, err := translate.NewTranslator(translate.Config{
		Engine:  engineType,
		BaseURL:  *mtURL,
		Logger:   logger,
	})
	if err != nil {
		logger.WithError(err).Fatal("Failed to create translator")
	}

	// Verify translator is healthy
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Info("Checking translator health...")
	if err := translator.CheckHealth(ctx); err != nil {
		logger.WithError(err).Warn("Translator health check failed, but continuing anyway")
		logger.Warn("Server will start, but translation requests may fail until translator is ready")
	} else {
		logger.Info("Translator health check passed")
	}

	// Create listener
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		logger.WithError(err).WithFields(logrus.Fields{
			"port": *port,
		}).Fatal("Failed to listen on port")
	}

	// Create gRPC server with options
	var opts []grpc.ServerOption

	// TODO: Configure TLS/mTLS when certificates are available
	if !*insecureMode {
		// TODO: Load TLS credentials from flags
		// For now, log warning and continue with insecure
		logger.Warn("TLS requested but not yet implemented, using insecure mode")
		opts = append(opts, grpc.Creds(insecure.NewCredentials()))
	} else {
		opts = append(opts, grpc.Creds(insecure.NewCredentials()))
	}

	// Configure server-side keepalive enforcement to match client settings
	// Client sends pings every 30s, so we allow up to 60s between pings
	// This prevents "too many pings" errors
	opts = append(opts, grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
		MinTime:             15 * time.Second, // Minimum time between pings (client sends every 30s)
		PermitWithoutStream: true,              // Allow pings even when no active streams
	}))
	opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionIdle:     5 * time.Minute, // Close idle connections after 5 minutes
		MaxConnectionAge:      30 * time.Minute, // Close connections after 30 minutes
		MaxConnectionAgeGrace: 5 * time.Second, // Grace period for closing
		Time:                  30 * time.Second, // Send keepalive pings every 30s if there's activity
		Timeout:               10 * time.Second, // Wait 10s for ping ack before considering connection dead
	}))

	logger.WithFields(logrus.Fields{
		"min_time":              "15s",
		"permit_without_stream": true,
		"max_connection_idle":   "5m",
		"max_connection_age":   "30m",
		"time":                  "30s",
		"timeout":               "10s",
	}).Debug("Configured gRPC server keepalive settings")

	// Create gRPC server
	s := grpc.NewServer(opts...)

	// Register health check service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	// Create and register translation service
	translationService := service.NewTranslationService(translator, logger)
	nanabushv1.RegisterTranslationServiceServer(s, translationService)

	// Enable reflection for grpcurl/debugging (can be disabled in production)
	reflection.Register(s)

	// Start periodic cleanup goroutine for expired clients
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()

	go func() {
		// Run cleanup every 30 seconds (more frequent to catch disconnected clients quickly)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		// Remove clients that haven't sent a heartbeat in 2x the heartbeat interval
		// Since heartbeat interval is typically 30s, this is 60 seconds
		// This is aggressive to catch clients that stopped sending heartbeats quickly
		maxIdleTime := 2 * 30 * time.Second // 60 seconds (2x heartbeat interval)

		for {
			select {
			case <-ticker.C:
				translationService.CleanupExpiredClients(maxIdleTime)
			case <-cleanupCtx.Done():
				return
			}
		}
	}()
	logger.WithFields(logrus.Fields{
		"cleanup_interval": "30 seconds",
		"max_idle_time":    "60 seconds (2x heartbeat interval)",
	}).Info("Started client cleanup goroutine")

	// Start periodic metrics logging
	metricsCtx, metricsCancel := context.WithCancel(context.Background())
	defer metricsCancel()

	go func() {
		ticker := time.NewTicker(1 * time.Minute) // Log metrics every minute
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Get registered clients
				clients := translationService.GetRegisteredClients()
				logger.WithFields(logrus.Fields{
					"total_clients": len(clients),
				}).Debug("Client metrics")

				if len(clients) > 0 {
					// Log namespace distribution
					nsCount := make(map[string]int)
					for _, client := range clients {
						ns := client.Namespace
						if ns == "" {
							ns = "unknown"
						}
						nsCount[ns]++
					}

					for ns, count := range nsCount {
						logger.WithFields(logrus.Fields{
							"namespace": ns,
							"count":     count,
						}).Debug("Clients by namespace")
					}
				}
			case <-metricsCtx.Done():
				return
			}
		}
	}()
	logger.Info("Started metrics logging goroutine (logs every minute)")

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		logger.WithFields(logrus.Fields{
			"port": *port,
		}).Info("gRPC server listening")
		if err := s.Serve(lis); err != nil {
			errChan <- fmt.Errorf("failed to serve: %w", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errChan:
		logger.WithError(err).Fatal("Server error")
	case sig := <-sigChan:
		logger.WithFields(logrus.Fields{
			"signal": sig.String(),
		}).Info("Received signal, shutting down gracefully...")

		// Graceful shutdown with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Set health status to NOT_SERVING
		healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

		// Graceful stop
		stopped := make(chan struct{})
		go func() {
			s.GracefulStop()
			close(stopped)
		}()

		select {
		case <-stopped:
			logger.Info("Server stopped gracefully")
		case <-ctx.Done():
			logger.Warn("Graceful shutdown timeout, forcing stop...")
			s.Stop()
		}
	}
}

