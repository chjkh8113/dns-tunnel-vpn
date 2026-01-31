// Package cloudflare provides Cloudflare DNS API integration for resolver management.
package cloudflare

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/chjkh8113/dns-tunnel-vpn/internal/config"
)

// Client provides Cloudflare DNS API operations.
type Client struct {
	config     *config.CloudflareConfig
	httpClient *http.Client
	baseURL    string
}

// TXTRecord represents a DNS TXT record.
type TXTRecord struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
}

// APIResponse represents a Cloudflare API response.
type APIResponse struct {
	Success  bool        `json:"success"`
	Errors   []APIError  `json:"errors"`
	Messages []string    `json:"messages"`
	Result   interface{} `json:"result"`
}

// APIError represents a Cloudflare API error.
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// New creates a new Cloudflare Client.
func New(cfg *config.CloudflareConfig) *Client {
	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: "https://api.cloudflare.com/client/v4",
	}
}

// FetchResolvers fetches the resolver list from the configured TXT record.
func (c *Client) FetchResolvers(ctx context.Context) ([]string, error) {
	if !c.config.Enabled {
		return nil, fmt.Errorf("cloudflare integration disabled")
	}

	content, err := c.getTXTRecord(ctx, c.config.TXTRecord)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch TXT record: %w", err)
	}

	// Parse resolvers from TXT content (comma-separated)
	resolvers := strings.Split(content, ",")
	result := make([]string, 0, len(resolvers))
	for _, r := range resolvers {
		r = strings.TrimSpace(r)
		if r != "" {
			result = append(result, r)
		}
	}

	log.Printf("Fetched %d resolvers from TXT record", len(result))
	return result, nil
}

// UpdateResolvers updates the TXT record with the new resolver list.
func (c *Client) UpdateResolvers(ctx context.Context, resolvers []string) error {
	if !c.config.Enabled {
		return fmt.Errorf("cloudflare integration disabled")
	}

	content := strings.Join(resolvers, ",")
	return c.setTXTRecord(ctx, c.config.TXTRecord, content)
}

// getTXTRecord retrieves a TXT record's content.
func (c *Client) getTXTRecord(ctx context.Context, name string) (string, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records?type=TXT&name=%s",
		c.baseURL, c.config.ZoneID, name)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var apiResp struct {
		Success bool        `json:"success"`
		Errors  []APIError  `json:"errors"`
		Result  []TXTRecord `json:"result"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !apiResp.Success {
		if len(apiResp.Errors) > 0 {
			return "", fmt.Errorf("API error: %s", apiResp.Errors[0].Message)
		}
		return "", fmt.Errorf("API request failed")
	}

	if len(apiResp.Result) == 0 {
		return "", fmt.Errorf("TXT record not found: %s", name)
	}

	return apiResp.Result[0].Content, nil
}

// setTXTRecord creates or updates a TXT record.
func (c *Client) setTXTRecord(ctx context.Context, name, content string) error {
	// First, try to get existing record
	existingID, _ := c.getRecordID(ctx, name, "TXT")

	var method, url string
	if existingID != "" {
		// Update existing record
		method = "PUT"
		url = fmt.Sprintf("%s/zones/%s/dns_records/%s",
			c.baseURL, c.config.ZoneID, existingID)
	} else {
		// Create new record
		method = "POST"
		url = fmt.Sprintf("%s/zones/%s/dns_records",
			c.baseURL, c.config.ZoneID)
	}

	payload := map[string]interface{}{
		"type":    "TXT",
		"name":    name,
		"content": content,
		"ttl":     300,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(string(payloadBytes)))
	if err != nil {
		return err
	}

	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !apiResp.Success {
		if len(apiResp.Errors) > 0 {
			return fmt.Errorf("API error: %s", apiResp.Errors[0].Message)
		}
		return fmt.Errorf("API request failed")
	}

	log.Printf("Updated TXT record: %s", name)
	return nil
}

// getRecordID finds the ID of an existing record.
func (c *Client) getRecordID(ctx context.Context, name, recordType string) (string, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records?type=%s&name=%s",
		c.baseURL, c.config.ZoneID, recordType, name)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var apiResp struct {
		Success bool        `json:"success"`
		Result  []TXTRecord `json:"result"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", err
	}

	if !apiResp.Success || len(apiResp.Result) == 0 {
		return "", fmt.Errorf("record not found")
	}

	return apiResp.Result[0].ID, nil
}

// setHeaders sets the required Cloudflare API headers.
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.config.APIToken)
	req.Header.Set("Content-Type", "application/json")
}

// IsEnabled returns true if Cloudflare integration is enabled.
func (c *Client) IsEnabled() bool {
	return c.config.Enabled && c.config.APIToken != "" && c.config.ZoneID != ""
}
