package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/nats-io/nats.go"

	"github.com/AlxPolt/sw-engineer-challenge/internal/daemon/adapters"
	"github.com/AlxPolt/sw-engineer-challenge/internal/daemon/ports"
	"github.com/AlxPolt/sw-engineer-challenge/internal/daemon/usecase"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/config"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/logger"
)

func main() {
	log, err := logger.New("daemon", os.Getenv("LOG_LEVEL"))
	if err != nil {
		panic(err)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Error("failed to load config", "err", err)
		return
	}

	opts := []nats.Option{
		nats.ClientCert(cfg.NATS.TLS.ClientCert, cfg.NATS.TLS.ClientKey),
		nats.RootCAs(cfg.NATS.TLS.CACert),
	}

	natsConn, err := nats.Connect(cfg.NATS.URL, opts...)
	if err != nil {
		log.Error("failed to connect to NATS", "err", err)
		return
	}

	var publisher ports.Publisher
	publisher, err = adapters.NewNATSPublisher(
		natsConn,
		cfg.Daemon.PublishSubject,
		cfg.Daemon.StreamName,
	)
	if err != nil {
		log.Error("failed to create JetStream publisher", "err", err)
		return
	}
	defer publisher.Close()

	emitter := usecase.NewEventEmitter(publisher, float64(cfg.Daemon.EventsPerSecond), cfg.Daemon.EventsBurst, log)
	emitter.Run(ctx)
}
