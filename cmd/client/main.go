package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/AlxPolt/sw-engineer-challenge/internal/client/adapters"
	"github.com/AlxPolt/sw-engineer-challenge/internal/client/domain"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/config"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/logger"

	"github.com/nats-io/nats.go"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "client error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	log, err := logger.New("client", os.Getenv("LOG_LEVEL"))
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Client.RequestTimeout)
	defer cancel()

	opts := []nats.Option{
		nats.ClientCert(cfg.NATS.TLS.ClientCert, cfg.NATS.TLS.ClientKey),
		nats.RootCAs(cfg.NATS.TLS.CACert),
	}
	natsConn, err := nats.Connect(cfg.NATS.URL, opts...)
	if err != nil {
		log.Error("failed to connect to NATS", "err", err)
		return fmt.Errorf("connecting to NATS: %w", err)
	}
	defer natsConn.Drain()

	requester := adapters.NewNATSRequester(
		natsConn,
		adapters.NATSRequesterConfig{
			QuerySubject:   cfg.Client.QuerySubject,
			RequestTimeout: cfg.Client.RequestTimeout,
		},
		log,
	)

	queryReq := domain.QueryRequest{
		MinCriticality: cfg.Client.X_MinCriticality,
		Limit:          cfg.Client.EventLimit,
	}

	response, err := requester.Request(ctx, queryReq)
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	displayList := make([]domain.Event, len(response.Events))
	for i, res := range response.Events {
		displayList[i] = domain.Event{
			Timestamp:    res.Timestamp.String(),
			Criticality:  res.Criticality,
			EventMessage: res.EventMessage,
		}
	}
	return displayTable(displayList)

}

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorGreen  = "\033[32m"
	colorWhite  = "\033[37m"
	colorOrange = "\033[91m"
)

func displayTable(events []domain.Event) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	// Header: TIME + MESSAGE + one column per criticality level 1..10
	fmt.Fprintln(w, "TIME\tMESSAGE\t 1\t 2\t 3\t 4\t 5\t 6\t 7\t 8\t 9\t10")
	fmt.Fprintln(w, "----\t-------\t--\t--\t--\t--\t--\t--\t--\t--\t--\t--")

	for _, event := range events {
		ts, err := time.Parse(time.RFC3339, event.Timestamp)
		if err != nil {
			ts = time.Now()
		}

		cols := [10]string{}
		c := event.Criticality
		if c >= 1 && c <= 10 {
			cols[c-1] = critColor(c) + "█" + colorReset
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			ts.Format("15:04:05"),
			truncate(event.EventMessage, 35),
			cols[0], cols[1], cols[2], cols[3], cols[4],
			cols[5], cols[6], cols[7], cols[8], cols[9],
		)
	}

	fmt.Fprintf(w, "\nTotal: %d events\n", len(events))
	return nil
}

func critColor(c int) string {
	switch {
	case c >= 9:
		return colorRed
	case c >= 7:
		return colorOrange
	case c >= 5:
		return colorYellow
	case c >= 2:
		return colorGreen
	default:
		return colorWhite
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
