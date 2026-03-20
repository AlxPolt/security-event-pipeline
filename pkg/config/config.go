package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type AppConfig struct {
	AppEnv   string
	LogLevel string
	NATS     NATSConfig
	InfluxDB InfluxDBConfig
	HTTP     HTTPConfig

	// Service-specific settings
	Daemon DaemonConfig
	Writer WriterConfig
	Reader ReaderConfig
	Client ClientConfig
}

type NATSConfig struct {
	URL      string
	User     string
	Password string
	TLS      TLSConfig
}

type DaemonConfig struct {
	PublishSubject  string
	StreamName      string
	EventsPerSecond int
	EventsBurst     int
}

type WriterConfig struct {
	SubscribeSubject string
	StreamName       string
	ConsumerName     string
	MaxDeliveries    int
	AckWaitSeconds   int
}

type ReaderConfig struct {
	QuerySubject string
}

type ClientConfig struct {
	// Subject for request-reply queries
	QuerySubject string

	// Query parameters
	X_MinCriticality int
	EventLimit       int

	// Request timeout
	RequestTimeout time.Duration
}

type InfluxDBConfig struct {
	URL      string
	Token    string
	Database string
	TLS      TLSConfig
}

type TLSConfig struct {
	InsecureSkipVerify bool
	CACert             string
	ClientCert         string
	ClientKey          string
}

type HTTPConfig struct {
	Addr               string
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
	CORSAllowedOrigins []string
}

func Load() (*AppConfig, error) {

	cfg := &AppConfig{
		LogLevel: getEnv("LOG_LEVEL", "info"),

		// NATS settings
		NATS: NATSConfig{
			URL:      getEnv("NATS_URL", "nats://nats:4222"),
			User:     os.Getenv("NATS_USER"),
			Password: os.Getenv("NATS_PASSWORD"),
			TLS: TLSConfig{
				InsecureSkipVerify: getEnvBool("NATS_TLS_INSECURE", false),
				CACert:             os.Getenv("NATS_CA_CERT"),
				ClientCert:         os.Getenv("NATS_TLS_CERT"),
				ClientKey:          os.Getenv("NATS_TLS_KEY"),
			},
		},

		Daemon: DaemonConfig{
			PublishSubject:  getEnv("DAEMON_PUBLISH_SUBJECT", "events_subject"),
			StreamName:      getEnv("DAEMON_STREAM_NAME", "events_tab"),
			EventsPerSecond: getEnvInt("DAEMON_EVENTS_PER_SECOND", 2),
			EventsBurst:     getEnvInt("DAEMON_EVENTS_BURST", 10),
		},

		Writer: WriterConfig{
			SubscribeSubject: getEnv("WRITER_SUBSCRIBE_SUBJECT", "events_subject"),
			StreamName:       getEnv("WRITER_STREAM_NAME", "events_tab"),
			ConsumerName:     getEnv("WRITER_CONSUMER_NAME", "writer-consumer"),
			MaxDeliveries:    getEnvInt("WRITER_MAX_DELIVERIES", 5),
			AckWaitSeconds:   getEnvInt("WRITER_ACK_WAIT_SECONDS", 30),
		},

		Reader: ReaderConfig{
			QuerySubject: getEnv("READER_QUERY_SUBJECT", "events_subject.query"),
		},

		Client: ClientConfig{
			QuerySubject:     getEnv("CLIENT_QUERY_SUBJECT", "events.query"),
			X_MinCriticality: getEnvInt("X_CLIENT_MIN_CRITICALITY", 5),
			EventLimit:       getEnvInt("CLIENT_EVENT_LIMIT", 10),
			RequestTimeout:   getEnvDuration("CLIENT_REQUEST_TIMEOUT", 5*time.Second),
		},

		// InfluxDB
		InfluxDB: InfluxDBConfig{
			URL:      getEnv("INFLUX_URL", "https://localhost:8181"),
			Token:    os.Getenv("INFLUX_TOKEN"),
			Database: getEnv("INFLUX_DB_EVENTS", "events_db"),
			TLS: TLSConfig{
				InsecureSkipVerify: getEnvBool("INFLUX_TLS_INSECURE", false),
				CACert:             os.Getenv("INFLUX_CA_CERT"),
			},
		},

		// HTTP — reader service only
		HTTP: HTTPConfig{
			Addr:               getEnv("HTTP_ADDR", ":8080"),
			ReadTimeout:        getEnvDuration("HTTP_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:       getEnvDuration("HTTP_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:        getEnvDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
			CORSAllowedOrigins: getEnvStringSlice("CORS_ALLOWED_ORIGINS", nil),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *AppConfig) Validate() error {
	if c.NATS.URL == "" {
		return fmt.Errorf("NATS_URL is required")
	}

	if c.AppEnv == "production" && c.NATS.User == "" {
		return fmt.Errorf("NATS_USER is required")
	}
	if c.AppEnv == "production" && c.NATS.Password == "" {
		return fmt.Errorf("NATS_PASSWORD is required")
	}

	if c.InfluxDB.URL == "" {
		return fmt.Errorf("INFLUX_URL is required")
	}
	if c.InfluxDB.Database == "" {
		return fmt.Errorf("INFLUX_DB_EVENTS is required")
	}

	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %s=%q is not a valid integer, using default %d\n", key, value, defaultValue)
		return defaultValue
	}
	return intValue
}

func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	boolValue, err := strconv.ParseBool(value)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %s=%q is not a valid boolean, using default %v\n", key, value, defaultValue)
		return defaultValue
	}
	return boolValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %s=%q is not a valid duration, using default %s\n", key, value, defaultValue)
		return defaultValue
	}
	return d
}

func getEnvStringSlice(key string, defaultValue []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	var result []string
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
