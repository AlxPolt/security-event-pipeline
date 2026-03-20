package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AlxPolt/sw-engineer-challenge/internal/writer/adapters"
	"github.com/AlxPolt/sw-engineer-challenge/internal/writer/usecase"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/config"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/logger"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/sanitizer"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/security"
)

const (
	shutdownTimeout     = 30 * time.Second
	workerCount         = 10
	influxWorkerCount   = 5
	influxBatchSize     = 50
	influxFlushInterval = 500 * time.Millisecond
)

func main() {
	log, err := logger.New("writer", os.Getenv("LOG_LEVEL"))
	if err != nil {
		panic(err)
	}
	defer log.Sync()

	if err := run(log); err != nil {
		log.Error("Writer service failed", "error", err.Error())
		os.Exit(1)
	}
}

func run(log *logger.Logger) error {
	log.Info("Starting Writer service...")

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := security.ValidateTLSConfig(
		cfg.NATS.TLS.ClientCert,
		cfg.NATS.TLS.ClientKey,
		cfg.NATS.TLS.CACert,
	); err != nil {
		return fmt.Errorf("NATS TLS validation failed: %w", err)
	}

	var natsTLS *tls.Config
	natsTLS, err = security.LoadClientTLSConfig(
		cfg.NATS.TLS.ClientCert,
		cfg.NATS.TLS.ClientKey,
		cfg.NATS.TLS.CACert,
	)
	if err != nil {
		return fmt.Errorf("failed to load NATS TLS config: %s", sanitizer.SanitizeError(err))
	}
	log.Info("NATS TLS configuration loaded successfully")

	influxCACert := os.Getenv("INFLUX_CA_CERT")

	var influxTLS *security.TLSOptions
	if influxCACert != "" {
		if err := security.ValidateTLSConfig("", "", influxCACert); err != nil {
			log.Warn("Failed to validate InfluxDB TLS config",
				"error", sanitizer.SanitizeError(err))
		} else {
			influxTLS = &security.TLSOptions{
				CACert: influxCACert,
			}
			log.Info("InfluxDB TLS enabled", "ca_cert", influxCACert)
		}
	}

	influxRepo, err := adapters.NewInfluxRepository(
		cfg.InfluxDB.URL,
		cfg.InfluxDB.Token,
		cfg.InfluxDB.Database,
		influxTLS,
		influxWorkerCount,
		influxBatchSize,
		influxFlushInterval,
		log,
	)
	if err != nil {
		return fmt.Errorf("failed to create InfluxDB repository: %w", err)
	}
	defer influxRepo.Close()

	processor := usecase.NewEventProcessor(influxRepo, usecase.DefaultRetryPolicy, log)

	subscriber, err := adapters.NewNATSSubscriber(
		cfg.NATS.URL,
		cfg.Writer.SubscribeSubject,
		cfg.Writer.StreamName,
		cfg.Writer.ConsumerName,
		cfg.Writer.MaxDeliveries,
		cfg.Writer.AckWaitSeconds,
		workerCount,
		natsTLS,
		processor,
		log,
	)
	if err != nil {
		return fmt.Errorf("failed to create NATS subscriber: %w", err)
	}

	log.Info("Writer service started",
		"workers", workerCount,
		"nats_url", sanitizer.Sanitize(cfg.NATS.URL),
		"influx_host", sanitizer.Sanitize(cfg.InfluxDB.URL),
		"tls", "mutual")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	log.Info("Shutdown signal received", "signal", sig.String())

	return gracefulShutdown(subscriber, log)
}

func gracefulShutdown(subscriber *adapters.NATSSubscriber, log *logger.Logger) error {
	log.Info("Starting graceful shutdown...")

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := subscriber.Close(); err != nil {
			log.Error("Error closing NATS subscriber", "error", err.Error())
		}
	}()

	select {
	case <-done:
		log.Info("Graceful shutdown completed")
		return nil
	case <-ctx.Done():
		log.Warn("Shutdown timeout reached, forcing exit")
		return fmt.Errorf("shutdown timeout exceeded")
	}
}
