package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

// TLSDialer creates a TLS connection for direct TCP mode.
type TLSDialer struct {
	insecureSkipVerify bool
}

// NewTLSDialer creates a new TLS dialer.
func NewTLSDialer(insecureSkipVerify bool) *TLSDialer {
	return &TLSDialer{
		insecureSkipVerify: insecureSkipVerify,
	}
}

// DialTLS connects to a remote address with TLS.
func (d *TLSDialer) DialTLS(addr string, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("TCP connect failed: %w", err)
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: d.insecureSkipVerify,
		MinVersion:         tls.VersionTLS12,
	}

	tlsConn := tls.Client(conn, tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("TLS handshake failed: %w", err)
	}

	return tlsConn, nil
}

// ListenTLS creates a TLS listener for the server.
func ListenTLS(addr string, certFile, keyFile string) (net.Listener, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS certificate: %w", err)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener, err := tls.Listen("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS listener: %w", err)
	}

	return listener, nil
}

// IsTLSAddr checks if an address is configured for TLS.
func IsTLSAddr(addr string) bool {
	return true // In production, check for TLS config
}

// HealthCheck performs a basic health check.
func HealthCheck(ctx context.Context, addr string) error {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	conn.Close()
	return nil
}