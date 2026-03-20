package security

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

func LoadClientTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	if certFile == "" && keyFile == "" && caFile == "" {
		return nil, nil // No TLS
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client cert: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if caFile != "" {
		caCert, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to append CA cert to pool")
		}
		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}

type TLSOptions struct {
	CACert             string
	ClientCert         string
	ClientKey          string
	InsecureSkipVerify bool
}

func NewTLSConfig(opts TLSOptions) (*tls.Config, error) {

	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: opts.InsecureSkipVerify,
	}

	if opts.CACert != "" {
		caCert, err := os.ReadFile(opts.CACert)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsCfg.RootCAs = pool
	}

	if opts.ClientCert != "" && opts.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(opts.ClientCert, opts.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return tlsCfg, nil
}

func ValidateTLSConfig(certFile, keyFile, caFile string) error {
	if certFile != "" || keyFile != "" {
		if certFile == "" || keyFile == "" {
			return fmt.Errorf("both cert and key files must be provided")
		}

		if _, err := os.Stat(certFile); os.IsNotExist(err) {
			return fmt.Errorf("cert file not found: %s", certFile)
		}

		if _, err := os.Stat(keyFile); os.IsNotExist(err) {
			return fmt.Errorf("key file not found: %s", keyFile)
		}
	}

	if caFile != "" {
		if _, err := os.Stat(caFile); os.IsNotExist(err) {
			return fmt.Errorf("CA file not found: %s", caFile)
		}
	}

	return nil
}
