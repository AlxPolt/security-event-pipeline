package adapters

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	influxdb3 "github.com/InfluxCommunity/influxdb3-go/v2/influxdb3"
	"go.uber.org/zap"

	"github.com/AlxPolt/sw-engineer-challenge/internal/reader/domain"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/logger"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/security"
)

type InfluxDBQuerier struct {
	client *influxdb3.Client
	db     string
	log    *logger.Logger
}

type InfluxDBQuerierConfig struct {
	URL      string
	Token    string
	Database string
}

func NewInfluxDBQuerier(cfg InfluxDBQuerierConfig, tlsOpts *security.TLSOptions, log logger.Logger) (*InfluxDBQuerier, error) {

	clientConfig := influxdb3.ClientConfig{
		Host:     cfg.URL,
		Token:    cfg.Token,
		Database: cfg.Database,
	}

	var httpClient *http.Client

	if tlsOpts != nil {
		tlsConf, err := security.NewTLSConfig(*tlsOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS config: %w", err)
		}

		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConf,
			},
			Timeout: 30 * time.Second,
		}

		clientConfig.HTTPClient = httpClient
		log.Info("TLS configuration applied",
			"insecure_skip_verify", tlsOpts.InsecureSkipVerify,
			"has_ca_cert", tlsOpts.CACert != "",
			"has_client_cert", tlsOpts.ClientCert != "")
	} else {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	client, err := influxdb3.New(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("influxdb_querier: creating client: %w", err)
	}

	return &InfluxDBQuerier{
		client: client,
		db:     cfg.Database,
		log:    log.With(zap.String("adapter", "influxdb_querier")),
	}, nil
}

func (q *InfluxDBQuerier) Query(ctx context.Context, params domain.QueryParams) ([]domain.EventResult, error) {

	sql := `
        SELECT time, criticality, event_message
        FROM events_tab
        WHERE criticality >= $1
        ORDER BY time DESC
        LIMIT $2
    `

	iterator, err := q.client.QueryWithParameters(ctx, sql, influxdb3.QueryParameters{
		"1": params.X_MinCriticality,
		"2": params.Y_Limit,
	})
	if err != nil {
		if strings.Contains(err.Error(), "table") || strings.Contains(err.Error(), "not found") {
			q.log.Debug("events_tab not yet created (no data written)", zap.Error(err))
			return []domain.EventResult{}, nil
		}
		return nil, fmt.Errorf("influxdb_querier: executing query: %w", err)
	}
	defer func() { _ = iterator.Done() }()

	var results []domain.EventResult

	for iterator.Next() {
		row := iterator.Value()

		var ts time.Time
		var criticality int64
		var message string

		if v, ok := row["time"]; ok {
			if t, ok := v.(time.Time); ok {
				ts = t
			}
		}
		if v, ok := row["criticality"]; ok {
			switch val := v.(type) {
			case int64:
				criticality = val
			case float64:
				criticality = int64(val)
			}
		}
		if v, ok := row["event_message"]; ok {
			if s, ok := v.(string); ok {
				message = s
			}
		}

		results = append(results, domain.EventResult{
			Criticality:  int(criticality),
			Timestamp:    ts.UTC(),
			EventMessage: message,
		})
	}

	if err := iterator.Err(); err != nil {
		return nil, fmt.Errorf("influxdb_querier: iterating results: %w", err)
	}

	q.log.Debug("query completed", zap.Int("results", len(results)))
	return results, nil
}

func (q *InfluxDBQuerier) Close() error {
	return q.client.Close()
}
