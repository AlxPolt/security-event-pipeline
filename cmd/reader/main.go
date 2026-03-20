// Package main is the entry point for the Reader service.
// @title           EventSys Reader API
// @version         1.0
// @description     Real-time security event query API backed by InfluxDB v3
// @termsOfService  http://example.com/terms/
// @contact.name    Platform Security Team
// @contact.email   security@example.com
// @license.name    Apache 2.0
// @license.url     http://www.apache.org/licenses/LICENSE-2.0.html
// @host            localhost:8080
// @BasePath        /api/v1
// @schemes         http https
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/AlxPolt/sw-engineer-challenge/internal/reader/adapters"
	"github.com/AlxPolt/sw-engineer-challenge/internal/reader/usecase"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/config"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/logger"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/sanitizer"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/security"
)

func main() {

	log, err := logger.New("reader", os.Getenv("LOG_LEVEL"))
	if err != nil {
		panic(err)
	}
	defer log.Sync()

	if err := run(log); err != nil {
		fmt.Fprintf(os.Stderr, "reader: fatal: %v\n", err)
		os.Exit(1)
	}
}

func run(log *logger.Logger) error {
	log.Info("Starting Reader service...")

	config, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := security.ValidateTLSConfig(
		config.NATS.TLS.ClientCert,
		config.NATS.TLS.ClientKey,
		config.NATS.TLS.CACert,
	); err != nil {
		return fmt.Errorf("NATS TLS validation failed: %w", err)
	}

	influxCACert := config.InfluxDB.TLS.CACert

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

	defer func() { _ = log.Sync() }()

	querier, err := adapters.NewInfluxDBQuerier(adapters.InfluxDBQuerierConfig{
		URL:      config.InfluxDB.URL,
		Token:    config.InfluxDB.Token,
		Database: config.InfluxDB.Database,
	}, influxTLS, *log)

	if err != nil {
		return fmt.Errorf("reader: creating influxdb querier: %w", err)
	}
	defer func() { _ = querier.Close() }()

	svc := usecase.NewQueryService(querier, log)
	handler := adapters.NewGinHandler(svc, *log)
	router := adapters.NewRouter(handler, *log, config.AppEnv, config.HTTP.CORSAllowedOrigins)

	server := &http.Server{
		Addr:         config.HTTP.Addr,
		Handler:      router,
		ReadTimeout:  config.HTTP.ReadTimeout,
		WriteTimeout: config.HTTP.WriteTimeout,
		IdleTimeout:  config.HTTP.IdleTimeout,
	}

	opts := []nats.Option{
		nats.ClientCert(config.NATS.TLS.ClientCert, config.NATS.TLS.ClientKey),
		nats.RootCAs(config.NATS.TLS.CACert),
	}
	nc, err := nats.Connect(config.NATS.URL, opts...)
	if err != nil {
		return fmt.Errorf("reader: connecting to NATS: %w", err)
	}
	defer func() { _ = nc.Drain() }()

	responder := adapters.NewNATSResponder(nc, adapters.NATSResponderConfig{
		Subject: config.Reader.QuerySubject,
	}, svc, *log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		log.Info("HTTP server starting", zap.String("addr", config.HTTP.Addr))
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server error: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		<-gCtx.Done()
		log.Info("shutting down HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), config.HTTP.ReadTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http server shutdown: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		return responder.Listen(gCtx)
	})

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("reader: service error: %w", err)
	}

	log.Info("reader shutdown complete")
	return nil
}
