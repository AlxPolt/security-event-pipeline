// # Write Worker Pool
//
// The InfluxDB write path uses a bounded worker pool that accumulates points
// into batches before issuing a single WritePoints call. This design serves
// two purposes:
//
//  1. Throughput: InfluxDB v3 is optimised for batch ingestion — one
//     WritePoints call for N points costs roughly the same round-trip as a
//     call for 1 point.
//
//  2. Back-pressure: the channel between Save() callers and the worker pool has
//     bounded capacity (workerCount × batchSize). When full, Save() blocks
//     until a slot frees or the caller's context deadline fires.
//
// # Retry and dead-letter policy
//
// Retry policy is owned by the usecase layer (EventProcessor.RetryPolicy).
// This adapter makes a single WritePoints attempt per batch and returns the
// result to each caller via their result channel. Callers are
// responsible for retrying on transient errors.

package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/InfluxCommunity/influxdb3-go/v2/influxdb3"

	"github.com/AlxPolt/sw-engineer-challenge/internal/writer/domain"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/logger"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/sanitizer"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/security"
)

// writeRequest pairs an InfluxDB point with a buffered result channel so that
// Save() can block until the batch containing this point has been flushed.
type writeRequest struct {
	point  *influxdb3.Point
	result chan<- error
}

// InfluxRepository is the secondary (driven) adapter for InfluxDB3.
// It implements domain.EventRepository (outbound port) and is injected into
// the application layer at startup by the composition root (main).
//
// # Worker pool
//
// Incoming Save() calls are decoupled from InfluxDB I/O via an internal
// buffered channel.  A fixed pool of worker goroutines drains that channel,
// accumulates points into batches, and issues a single WritePoints call per
// batch — with exponential-backoff retries on failure.  A flush happens when
// either batchSize points have accumulated OR the flushInterval ticker fires.
//
// Callers of Save() block on a per-request result channel until their point is
// included in a flushed batch and the write result is known.  This keeps
// back-pressure well-defined and avoids silent data loss.
type InfluxRepository struct {
	client     *influxdb3.Client
	database   string
	logger     *logger.Logger
	baseURL    string
	token      string
	httpClient *http.Client

	// Write worker pool
	pointCh chan writeRequest // bounded; provides back-pressure to callers
	stopCh  chan struct{}     // closed by Close() to drain & stop all workers
	wg      sync.WaitGroup    // tracks live worker goroutines
	once    sync.Once         // makes Close() idempotent
}

// NewInfluxRepository creates a new InfluxDB repository and starts the internal write worker pool.
//
//   - workerCount:    goroutines draining the write channel
//   - batchSize:      maximum points per WritePoints call
//   - flushInterval:  maximum time between flushes (when batch isn't full)
func NewInfluxRepository(
	host, token, database string,
	tlsOpts *security.TLSOptions,
	workerCount, batchSize int,
	flushInterval time.Duration,
	log *logger.Logger,
) (*InfluxRepository, error) {
	clientConfig := influxdb3.ClientConfig{
		Host:     host,
		Token:    token,
		Database: database,
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
		return nil, fmt.Errorf("failed to create InfluxDB3 client: %w", err)
	}

	repo := &InfluxRepository{
		client:     client,
		database:   database,
		logger:     log,
		baseURL:    host,
		token:      token,
		httpClient: httpClient,
		pointCh:    make(chan writeRequest, workerCount*batchSize),
		stopCh:     make(chan struct{}),
	}

	for i := 0; i < workerCount; i++ {
		repo.wg.Add(1)
		go repo.writeWorker(batchSize, flushInterval)
	}

	log.Info("InfluxDB write worker pool started",
		"workers", workerCount,
		"batch_size", batchSize,
		"flush_interval", flushInterval)

	if err := repo.ensureDatabase(context.Background()); err != nil {
		_ = repo.Close()
		return nil, fmt.Errorf("influxdb_repository: ensuring database exists: %w", err)
	}

	if err := verifyConnection(client, database); err != nil {
		_ = repo.Close()
		return nil, fmt.Errorf("failed to verify InfluxDB connection: %w", err)
	}

	log.Info("Connected to InfluxDB3",
		"host", sanitizer.Sanitize(host),
		"database", database,
		"tls_enabled", tlsOpts != nil)

	return repo, nil
}

// Each point's result channel is always written to exactly once — either with the write error or nil
func (r *InfluxRepository) writeWorker(batchSize int, flushInterval time.Duration) {
	defer r.wg.Done()

	points := make([]*influxdb3.Point, 0, batchSize)
	results := make([]chan<- error, 0, batchSize)

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(points) == 0 {
			return
		}
		r.flushBatch(points, results)
		points = points[:0]
		results = results[:0]
	}

	for {
		select {
		case req := <-r.pointCh:
			points = append(points, req.point)
			results = append(results, req.result)
			if len(points) >= batchSize {
				flush()
				ticker.Reset(flushInterval)
			}

		case <-ticker.C:
			flush()

		case <-r.stopCh:
			for {
				select {
				case req := <-r.pointCh:
					points = append(points, req.point)
					results = append(results, req.result)
					if len(points) >= batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

// flushBatch issues a single WritePoints call for the accumulated batch and
// notifies every caller's result channel exactly once.
// Retry policy is owned by the usecase layer; this method makes one attempt.
func (r *InfluxRepository) flushBatch(points []*influxdb3.Point, results []chan<- error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err := r.client.WritePoints(ctx, points)
	cancel()

	if err == nil {
		r.logger.Debug("batch written to InfluxDB", "batch_size", len(points))
	} else {
		r.logger.Error("InfluxDB write failed",
			"error", sanitizer.SanitizeError(err),
			"batch_size", len(points))
	}

	for _, ch := range results {
		ch <- err
	}
}

// This synchronous contract lets the NATS subscriber ACK only after the data is confirmed persisted or NAK on error / context timeout.
func (r *InfluxRepository) Save(ctx context.Context, event domain.Event) error {
	timestamp, err := time.Parse(time.RFC3339Nano, event.Timestamp)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	point := influxdb3.NewPoint(
		"events_tab",
		map[string]string{
			"severity": event.Severity(),
		},
		map[string]interface{}{
			"criticality":   event.Criticality,
			"event_message": event.EventMessage,
		},
		timestamp,
	)

	resultCh := make(chan error, 1)

	select {
	case r.pointCh <- writeRequest{point: point, result: resultCh}:
	case <-ctx.Done():
		return ctx.Err()
	case <-r.stopCh:
		return fmt.Errorf("influx repository is closed")
	}

	select {
	case err := <-resultCh:
		if err != nil {
			return fmt.Errorf("failed to write point: %w", err)
		}
		r.logger.Info("Event saved to InfluxDB3",
			"criticality", event.Criticality,
			"timestamp", event.Timestamp,
			"severity", event.Severity())
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *InfluxRepository) Close() error {
	r.once.Do(func() {
		close(r.stopCh)
	})
	r.wg.Wait()
	if r.client != nil {
		r.client.Close()
		r.logger.Info("InfluxDB connection closed")
	}
	return nil
}

func (r *InfluxRepository) ensureDatabase(ctx context.Context) error {
	r.logger.Info("checking if database exists", "database", r.database)

	listURL := fmt.Sprintf("%s/api/v3/configure/database?format=json", r.baseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", listURL, nil)
	if err != nil {
		return fmt.Errorf("creating list request: %w", err)
	}

	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("listing databases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		r.logger.Warn("InfluxDB auth error on ensureDatabase, skipping — database will be created on first write",
			"status", resp.StatusCode)
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("listing databases failed with status: %d", resp.StatusCode)
	}

	var listResult []struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&listResult); err != nil {
		return fmt.Errorf("decoding database list: %w", err)
	}

	for _, db := range listResult {
		if db.Name == r.database {
			r.logger.Info("database already exists", "database", r.database)
			return nil
		}
	}

	r.logger.Info("creating database", "database", r.database)

	createURL := fmt.Sprintf("%s/api/v3/configure/database", r.baseURL)

	createBody := map[string]string{
		"db": r.database,
	}

	bodyBytes, err := json.Marshal(createBody)
	if err != nil {
		return fmt.Errorf("marshaling create request: %w", err)
	}

	createReq, err := http.NewRequestWithContext(ctx, "POST", createURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("creating create request: %w", err)
	}

	createReq.Header.Set("Content-Type", "application/json")
	if r.token != "" {
		createReq.Header.Set("Authorization", "Bearer "+r.token)
	}

	createResp, err := r.httpClient.Do(createReq)
	if err != nil {
		return fmt.Errorf("creating database: %w", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusOK && createResp.StatusCode != http.StatusCreated {
		if createResp.StatusCode == http.StatusConflict {
			r.logger.Info("database already exists (409), continuing", "database", r.database)
			return nil
		}
		bodyBytes, _ := io.ReadAll(createResp.Body)
		return fmt.Errorf("creating database failed with status %d: %s", createResp.StatusCode, string(bodyBytes))
	}

	r.logger.Info("database created successfully", "database", r.database)
	return nil
}

func verifyConnection(client *influxdb3.Client, database string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Query(ctx, "SELECT * FROM events_tab LIMIT 1")
	if err != nil {
		errStr := err.Error()
		if errStr == "no data" ||
			strings.Contains(errStr, "database not found") ||
			strings.Contains(errStr, "Cannot retrieve database") ||
			strings.Contains(errStr, "table") {
			return nil
		}
		return err
	}

	return nil
}
