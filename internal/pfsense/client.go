// Package pfsense provides a client for interacting with the pfSense REST API.
package pfsense

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Client represents a pfSense API client with retry logic and circuit breaker.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	logger     *slog.Logger
	maxRetries int
	backoff    []time.Duration

	// Circuit breaker state
	failures         int
	circuitOpenUntil time.Time
}

// ClientConfig holds configuration for creating a new Client.
type ClientConfig struct {
	BaseURL            string
	APIKey             string
	Timeout            time.Duration
	InsecureSkipVerify bool
	Logger             *slog.Logger
}

// NewClient creates a new pfSense API client.
func NewClient(config ClientConfig) *Client {
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}

	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	// Configure TLS
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.InsecureSkipVerify,
	}

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: config.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	return &Client{
		baseURL:    strings.TrimSuffix(config.BaseURL, "/"),
		apiKey:     config.APIKey,
		httpClient: httpClient,
		logger:     config.Logger,
		maxRetries: 3,
		backoff:    []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second},
	}
}

// Do executes an HTTP request with retry logic and circuit breaker.
func (c *Client) Do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	// Check circuit breaker
	if time.Now().Before(c.circuitOpenUntil) {
		return nil, fmt.Errorf("circuit breaker is open: retrying after %v", time.Until(c.circuitOpenUntil))
	}

	url := fmt.Sprintf("%s%s", c.baseURL, path)
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			c.logger.Info("Retrying request", "attempt", attempt, "url", url)
			time.Sleep(c.backoff[attempt-1])
		}

		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers
		req.Header.Set("X-API-Key", c.apiKey)
		// Only set Content-Type for requests with bodies (POST, PATCH, PUT, DELETE)
		// pfSense API v2 rejects GET requests that have Content-Type header set
		if method != http.MethodGet {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")

		c.logger.Debug("Making API request",
			"method", method,
			"url", url,
			"attempt", attempt+1,
			"headers", req.Header,
		)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			c.logger.Warn("Request failed", "error", err, "attempt", attempt+1)
			continue
		}

		// Success: reset circuit breaker
		if resp.StatusCode < 500 {
			c.failures = 0
			c.logger.Debug("Request succeeded", "status", resp.StatusCode)
			return resp, nil
		}

		// Server error: retry
		_ = resp.Body.Close()
		lastErr = fmt.Errorf("server error: HTTP %d", resp.StatusCode)
		c.logger.Warn("Server error", "status", resp.StatusCode, "attempt", attempt+1)
	}

	// All retries failed: update circuit breaker
	c.failures++
	if c.failures >= 3 {
		c.circuitOpenUntil = time.Now().Add(60 * time.Second)
		c.logger.Error("Circuit breaker opened",
			"failures", c.failures,
			"open_until", c.circuitOpenUntil,
		)
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", c.maxRetries+1, lastErr)
}

// Get performs a GET request.
func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	resp, err := c.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Warn("Failed to close response body", "error", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	c.logger.Debug("Response received",
		"status", resp.StatusCode,
		"body_length", len(body),
		"body_preview", string(body[:min(len(body), 200)]),
	)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: HTTP %d - %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Post performs a POST request.
func (c *Client) Post(ctx context.Context, path string, body []byte) ([]byte, error) {
	resp, err := c.Do(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Warn("Failed to close response body", "error", err)
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("API error: HTTP %d - %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// Delete performs a DELETE request.
func (c *Client) Delete(ctx context.Context, path string) ([]byte, error) {
	resp, err := c.Do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Warn("Failed to close response body", "error", err)
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: HTTP %d - %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// Patch performs a PATCH request.
func (c *Client) Patch(ctx context.Context, path string, body []byte) ([]byte, error) {
	resp, err := c.Do(ctx, http.MethodPatch, path, body)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Warn("Failed to close response body", "error", err)
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: HTTP %d - %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
