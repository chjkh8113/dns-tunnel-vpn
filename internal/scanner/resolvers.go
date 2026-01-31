// Package scanner provides DNS resolver testing implementations.
package scanner

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// dnsQuery is a minimal DNS query for "example.com" A record.
var dnsQuery = []byte{
	0x00, 0x01, // Transaction ID
	0x01, 0x00, // Standard query
	0x00, 0x01, // Questions: 1
	0x00, 0x00, // Answer RRs: 0
	0x00, 0x00, // Authority RRs: 0
	0x00, 0x00, // Additional RRs: 0
	// Query: example.com
	0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e',
	0x03, 'c', 'o', 'm',
	0x00,       // null terminator
	0x00, 0x01, // Type A
	0x00, 0x01, // Class IN
}

// testUDPResolver tests a UDP DNS resolver.
func (s *Scanner) testUDPResolver(ctx context.Context, address string) error {
	dialer := net.Dialer{Timeout: s.config.Timeout}
	conn, err := dialer.DialContext(ctx, "udp", address)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}
	defer conn.Close()

	if _, err := conn.Write(dnsQuery); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	conn.SetReadDeadline(time.Now().Add(s.config.Timeout))
	response := make([]byte, 512)
	n, err := conn.Read(response)
	if err != nil {
		return fmt.Errorf("read failed: %w", err)
	}

	if n < 12 {
		return fmt.Errorf("response too short: %d bytes", n)
	}

	if response[2]&0x80 == 0 {
		return fmt.Errorf("not a DNS response")
	}

	return nil
}

// testDoHResolver tests a DNS-over-HTTPS resolver.
func (s *Scanner) testDoHResolver(ctx context.Context, url string) error {
	if url == "" {
		return fmt.Errorf("empty DoH URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(dnsQuery))
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	client := &http.Client{Timeout: s.config.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("read response failed: %w", err)
	}

	if len(body) < 12 {
		return fmt.Errorf("response too short: %d bytes", len(body))
	}

	if body[2]&0x80 == 0 {
		return fmt.Errorf("not a DNS response")
	}

	return nil
}

// testDoTResolver tests a DNS-over-TLS resolver.
func (s *Scanner) testDoTResolver(ctx context.Context, address string) error {
	if address == "" {
		return fmt.Errorf("empty DoT address")
	}

	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: s.config.Timeout},
		Config:    &tls.Config{MinVersion: tls.VersionTLS12},
	}

	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("TLS dial failed: %w", err)
	}
	defer conn.Close()

	// DNS over TLS uses TCP framing: 2-byte length prefix
	msgLen := uint16(len(dnsQuery))
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, msgLen)

	if _, err := conn.Write(lenBuf); err != nil {
		return fmt.Errorf("write length failed: %w", err)
	}
	if _, err := conn.Write(dnsQuery); err != nil {
		return fmt.Errorf("write query failed: %w", err)
	}

	conn.SetReadDeadline(time.Now().Add(s.config.Timeout))
	respLenBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, respLenBuf); err != nil {
		return fmt.Errorf("read response length failed: %w", err)
	}

	respLen := binary.BigEndian.Uint16(respLenBuf)
	if respLen < 12 || respLen > 4096 {
		return fmt.Errorf("invalid response length: %d", respLen)
	}

	response := make([]byte, respLen)
	if _, err := io.ReadFull(conn, response); err != nil {
		return fmt.Errorf("read response failed: %w", err)
	}

	if response[2]&0x80 == 0 {
		return fmt.Errorf("not a DNS response")
	}

	return nil
}
