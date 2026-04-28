package bigcommerce

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/roel-c/bc-admin-mcp/internal/config"
)

const (
	baseURL = "https://api.bigcommerce.com/stores"

	// maxResponseBodyBytes caps the size of any single BC API response body
	// to prevent OOM from malicious or malformed upstream responses.
	maxResponseBodyBytes = 50 * 1024 * 1024 // 50 MB
)

// RateLimitInfo holds quota state parsed from BC response headers.
type RateLimitInfo struct {
	RequestsLeft  int
	RequestsQuota int
	TimeWindowMs  int
	TimeResetMs   int
}

// Client is the BigCommerce REST API client with built-in rate limiting,
// exponential backoff, and header-driven throttle awareness.
type Client struct {
	httpClient *http.Client
	storeHash  string
	authToken  string
	cfg        config.BigCommerceConfig
	logger     *slog.Logger

	mu            sync.Mutex
	lastRateLimit RateLimitInfo
	ticker        *time.Ticker
	throttle      <-chan time.Time
}

func NewClient(cfg config.BigCommerceConfig, logger *slog.Logger) *Client {
	interval := time.Duration(float64(time.Second) / cfg.RequestsPerSecond)
	ticker := time.NewTicker(interval)
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        20,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		storeHash: cfg.StoreHash,
		authToken: cfg.AuthToken,
		cfg:       cfg,
		logger:    logger,
		ticker:    ticker,
		throttle:  ticker.C,
	}
}

// Close releases resources held by the client. Call on shutdown.
func (c *Client) Close() {
	c.ticker.Stop()
}

func (c *Client) v3URL(path string) string {
	return fmt.Sprintf("%s/%s/v3/%s", baseURL, c.storeHash, path)
}

func (c *Client) v2URL(path string) string {
	return fmt.Sprintf("%s/%s/v2/%s", baseURL, c.storeHash, path)
}

// Do executes an HTTP request with throttling, rate-limit awareness, and retry.
func (c *Client) Do(ctx context.Context, method, url string, body any) (*http.Response, []byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	var lastErr error
	for attempt := range c.cfg.MaxRetries {
		if err := c.waitForQuota(ctx); err != nil {
			return nil, nil, err
		}

		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return nil, nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("X-Auth-Token", c.authToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			c.backoff(ctx, attempt)
			if body != nil {
				data, _ := json.Marshal(body)
				reqBody = bytes.NewReader(data)
			}
			continue
		}

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read response: %w", err)
			continue
		}

		c.parseRateLimitHeaders(resp)

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return resp, respBody, nil
		case resp.StatusCode == 429:
			c.logger.Warn("rate limited by BigCommerce",
				"attempt", attempt+1,
				"reset_ms", c.lastRateLimit.TimeResetMs,
			)
			c.waitForReset(ctx)
			if body != nil {
				data, _ := json.Marshal(body)
				reqBody = bytes.NewReader(data)
			}
			lastErr = fmt.Errorf("rate limited (429)")
		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("server error %d: %s", resp.StatusCode, string(respBody))
			c.logger.Warn("BigCommerce server error",
				"status", resp.StatusCode,
				"attempt", attempt+1,
			)
			c.backoff(ctx, attempt)
			if body != nil {
				data, _ := json.Marshal(body)
				reqBody = bytes.NewReader(data)
			}
		default:
			return resp, respBody, &APIError{
				StatusCode: resp.StatusCode,
				Body:       respBody,
			}
		}
	}

	return nil, nil, fmt.Errorf("max retries (%d) exceeded: %w", c.cfg.MaxRetries, lastErr)
}

// Get performs a GET request to a V3 endpoint.
func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	_, body, err := c.Do(ctx, http.MethodGet, c.v3URL(path), nil)
	return body, err
}

// GetV2 performs a GET request to a V2 endpoint.
func (c *Client) GetV2(ctx context.Context, path string) ([]byte, error) {
	_, body, err := c.Do(ctx, http.MethodGet, c.v2URL(path), nil)
	return body, err
}

// Put performs a PUT request to a V3 endpoint.
func (c *Client) Put(ctx context.Context, path string, body any) ([]byte, error) {
	_, respBody, err := c.Do(ctx, http.MethodPut, c.v3URL(path), body)
	return respBody, err
}

// Post performs a POST request to a V3 endpoint.
func (c *Client) Post(ctx context.Context, path string, body any) ([]byte, error) {
	_, respBody, err := c.Do(ctx, http.MethodPost, c.v3URL(path), body)
	return respBody, err
}

// Delete performs a DELETE request to a V3 endpoint.
func (c *Client) Delete(ctx context.Context, path string) ([]byte, error) {
	_, body, err := c.Do(ctx, http.MethodDelete, c.v3URL(path), nil)
	return body, err
}

// GetAll fetches all pages of a paginated V3 endpoint, returning the merged
// "data" arrays. It follows offset pagination using page= and limit= params.
func (c *Client) GetAll(ctx context.Context, path string) ([]json.RawMessage, error) {
	var allData []json.RawMessage
	page := 1

	for {
		separator := "?"
		if bytes.Contains([]byte(path), []byte("?")) {
			separator = "&"
		}
		pagedPath := fmt.Sprintf("%s%spage=%d&limit=%d", path, separator, page, c.cfg.DefaultPageLimit)

		body, err := c.Get(ctx, pagedPath)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}

		var resp PaginatedResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parse page %d: %w", page, err)
		}

		allData = append(allData, resp.Data...)

		if c.cfg.MaxTotalRecords > 0 && len(allData) >= c.cfg.MaxTotalRecords {
			c.logger.Warn("pagination ceiling reached — truncating results",
				"fetched", len(allData),
				"limit", c.cfg.MaxTotalRecords,
				"total_available", resp.Meta.Pagination.Total,
			)
			allData = allData[:c.cfg.MaxTotalRecords]
			break
		}

		if resp.Meta.Pagination.CurrentPage >= resp.Meta.Pagination.TotalPages {
			break
		}
		page++
	}

	return allData, nil
}

// BatchPut sends items in batches via PUT to a V3 endpoint. It respects the
// configured batch size, delay between chunks, and sequential-by-default
// write policy from BC-Tool-Boundaries.md.
func (c *Client) BatchPut(ctx context.Context, path string, items []any, batchSize int) (*BatchResult, error) {
	result := &BatchResult{}

	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}
		chunk := items[i:end]

		respBody, err := c.Put(ctx, path, chunk)
		if err != nil {
			result.Failed += len(chunk)
			result.Errors = append(result.Errors, BatchError{
				Offset: i,
				Count:  len(chunk),
				Err:    err.Error(),
			})
			c.logger.Error("batch chunk failed",
				"offset", i,
				"count", len(chunk),
				"error", err,
			)
			continue
		}

		result.Succeeded += len(chunk)
		result.Responses = append(result.Responses, respBody)

		if end < len(items) {
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(c.cfg.DelayBetweenChunks):
			}
		}
	}

	return result, nil
}

func (c *Client) waitForQuota(ctx context.Context) error {
	c.mu.Lock()
	rl := c.lastRateLimit
	c.mu.Unlock()

	if rl.RequestsLeft > 0 && rl.RequestsLeft <= c.cfg.QuotaSafetyBuffer {
		waitDur := time.Duration(rl.TimeResetMs) * time.Millisecond
		c.logger.Info("pausing for rate limit quota reset",
			"requests_left", rl.RequestsLeft,
			"wait_ms", rl.TimeResetMs,
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDur):
		}
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.throttle:
	}
	return nil
}

func (c *Client) waitForReset(ctx context.Context) {
	c.mu.Lock()
	resetMs := c.lastRateLimit.TimeResetMs
	c.mu.Unlock()

	if resetMs <= 0 {
		resetMs = 5000
	}

	select {
	case <-ctx.Done():
	case <-time.After(time.Duration(resetMs) * time.Millisecond):
	}
}

func (c *Client) backoff(ctx context.Context, attempt int) {
	wait := time.Duration(math.Pow(2, float64(attempt))) * time.Second
	if wait > 60*time.Second {
		wait = 60 * time.Second
	}
	select {
	case <-ctx.Done():
	case <-time.After(wait):
	}
}

func (c *Client) parseRateLimitHeaders(resp *http.Response) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v := resp.Header.Get("X-Rate-Limit-Requests-Left"); v != "" {
		c.lastRateLimit.RequestsLeft, _ = strconv.Atoi(v)
	}
	if v := resp.Header.Get("X-Rate-Limit-Requests-Quota"); v != "" {
		c.lastRateLimit.RequestsQuota, _ = strconv.Atoi(v)
	}
	if v := resp.Header.Get("X-Rate-Limit-Time-Window-Ms"); v != "" {
		c.lastRateLimit.TimeWindowMs, _ = strconv.Atoi(v)
	}
	if v := resp.Header.Get("X-Rate-Limit-Time-Reset-Ms"); v != "" {
		c.lastRateLimit.TimeResetMs, _ = strconv.Atoi(v)
	}
}
