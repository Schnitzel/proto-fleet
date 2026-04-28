package mujina

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// DefaultPort is the default TCP port Mujina listens on (ASCII 'M'=77, 'U'=85).
	DefaultPort = 7785

	defaultTimeout     = 15 * time.Second
	maxResponseBodyLen = 1 << 20 // 1 MiB
	apiPrefix          = "/api/v0"
)

// Client is an HTTP client for the Mujina REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient constructs a Client targeting the given host and port.
func NewClient(host string, port int) *Client {
	if port == 0 {
		port = DefaultPort
	}
	return &Client{
		baseURL: fmt.Sprintf("http://%s:%d", host, port),
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// NewTestClient creates a Client with a custom base URL and http.Client.
// Intended for use in tests with httptest servers.
func NewTestClient(baseURL string, hc *http.Client) *Client {
	return &Client{baseURL: baseURL, httpClient: hc}
}

// Health calls GET /api/v0/health and returns nil on success.
func (c *Client) Health(ctx context.Context) error {
	_, err := c.getText(ctx, "health")
	return err
}

// GetMiner calls GET /api/v0/miner and returns the full telemetry snapshot.
func (c *Client) GetMiner(ctx context.Context) (*MinerTelemetry, error) {
	var t MinerTelemetry
	if err := c.getJSON(ctx, "miner", &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// PatchMiner calls PATCH /api/v0/miner to apply a partial update.
func (c *Client) PatchMiner(ctx context.Context, req MinerPatchRequest) error {
	return c.patch(ctx, "miner", req)
}

// GetBoards calls GET /api/v0/boards and returns the list of connected boards.
func (c *Client) GetBoards(ctx context.Context) ([]BoardTelemetry, error) {
	var boards []BoardTelemetry
	if err := c.getJSON(ctx, "boards", &boards); err != nil {
		return nil, err
	}
	return boards, nil
}

// GetSources calls GET /api/v0/sources and returns the list of job sources.
func (c *Client) GetSources(ctx context.Context) ([]SourceTelemetry, error) {
	var sources []SourceTelemetry
	if err := c.getJSON(ctx, "sources", &sources); err != nil {
		return nil, err
	}
	return sources, nil
}

// ---- internal helpers -------------------------------------------------------

func (c *Client) url(path string) string {
	return fmt.Sprintf("%s%s/%s", c.baseURL, apiPrefix, path)
}

func (c *Client) getText(ctx context.Context, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path), nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyLen))
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GET %s returned HTTP %d: %s", path, resp.StatusCode, body)
	}
	return string(body), nil
}

func (c *Client) getJSON(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path), nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyLen))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s returned HTTP %d: %s", path, resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
	}
	return nil
}

func (c *Client) patch(ctx context.Context, path string, payload interface{}) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.url(path), bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("PATCH %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyLen))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("PATCH %s returned HTTP %d: %s", path, resp.StatusCode, body)
	}
	return nil
}
