package bigcommerce

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"
	"time"
)

// multipartQuoteEscaper matches the escaping mime/multipart applies to
// Content-Disposition field/file names.
var multipartQuoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

// b2bBaseURL is the B2B Edition Management REST API base. All paths are
// appended directly: e.g. "companies", "users?companyId=42".
const b2bBaseURL = "https://api-b2b.bigcommerce.com/api/v3/io"

// b2bDefaultPageSize is the page size used when fetching all records from
// paginated B2B endpoints (B2B API max is 100).
const b2bDefaultPageSize = 100

// B2BListResponse wraps a paginated B2B Edition list response.
//
// Actual format (confirmed against live API):
//
//	{"code":200,"data":[...],"meta":{"pagination":{"totalCount":N,"offset":0,"limit":20}}}
type B2BListResponse struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
	Meta B2BResponseMeta `json:"meta"`
}

// B2BResponseMeta is the top-level meta envelope in B2B list responses.
type B2BResponseMeta struct {
	Pagination B2BPagination `json:"pagination"`
}

// B2BPagination holds B2B Edition offset-based pagination metadata.
type B2BPagination struct {
	TotalCount int `json:"totalCount"`
	Offset     int `json:"offset"`
	Limit      int `json:"limit"`
}

// B2BSingleResponse wraps a single-item B2B Edition response.
//
//	{"code":200,"data":{…}}
type B2BSingleResponse struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

// B2BClient is an HTTP client for the B2B Edition Management REST API
// (https://api-b2b.bigcommerce.com/api/v3/io/). It uses the same
// X-Auth-Token as the core Management API, plus an X-Store-Hash header
// required since the unified auth migration (September 30 2025).
type B2BClient struct {
	storeHash  string
	authToken  string
	maxRetries int
	httpClient *http.Client
	logger     *slog.Logger
	ticker     *time.Ticker
	throttle   <-chan time.Time
}

// NewB2BClient constructs a B2BClient using the store credentials already
// present in BigCommerceConfig. No additional env vars are needed.
func NewB2BClient(storeHash, authToken string, maxRetries int, logger *slog.Logger) *B2BClient {
	// Conservative 1 req/s default — B2B calls are low-frequency admin
	// operations. Shared quota with the core client is not yet coordinated.
	ticker := time.NewTicker(time.Second)
	return &B2BClient{
		storeHash:  storeHash,
		authToken:  authToken,
		maxRetries: maxRetries,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		logger:   logger,
		ticker:   ticker,
		throttle: ticker.C,
	}
}

// Close releases the rate-limiter ticker. Call on server shutdown.
func (c *B2BClient) Close() { c.ticker.Stop() }

func (c *B2BClient) url(path string) string {
	return fmt.Sprintf("%s/%s", b2bBaseURL, path)
}

// Do executes an HTTP request against the B2B Edition API with throttling
// and exponential-backoff retry on transient errors.
func (c *B2BClient) Do(ctx context.Context, method, url string, body any) ([]byte, error) {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal B2B request body: %w", err)
		}
	}

	newReqBody := func() io.Reader {
		if bodyBytes == nil {
			return nil
		}
		return bytes.NewReader(bodyBytes)
	}

	var lastErr error
	for attempt := range c.maxRetries {
		// Throttle
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.throttle:
		}

		req, err := http.NewRequestWithContext(ctx, method, url, newReqBody())
		if err != nil {
			return nil, fmt.Errorf("create B2B request: %w", err)
		}
		req.Header.Set("X-Auth-Token", c.authToken)
		req.Header.Set("X-Store-Hash", c.storeHash)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("B2B request failed: %w", err)
			c.backoff(ctx, attempt)
			continue
		}

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read B2B response: %w", err)
			continue
		}

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return respBody, nil
		case resp.StatusCode == 429:
			c.logger.Warn("B2B rate limited", "attempt", attempt+1)
			c.backoff(ctx, attempt)
			lastErr = fmt.Errorf("B2B rate limited (429)")
		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("B2B server error %d: %s", resp.StatusCode, string(respBody))
			c.logger.Warn("B2B server error", "status", resp.StatusCode, "attempt", attempt+1)
			c.backoff(ctx, attempt)
		default:
			return nil, &APIError{
				StatusCode: resp.StatusCode,
				Body:       respBody,
				Path:       url,
				Method:     method,
			}
		}
	}
	return nil, fmt.Errorf("B2B max retries (%d) exceeded: %w", c.maxRetries, lastErr)
}

func (c *B2BClient) backoff(ctx context.Context, attempt int) {
	wait := time.Duration(math.Pow(2, float64(attempt))) * time.Second
	if wait > 60*time.Second {
		wait = 60 * time.Second
	}
	select {
	case <-ctx.Done():
	case <-time.After(wait):
	}
}

// B2BGet performs a GET to a B2B Edition path.
func (c *B2BClient) B2BGet(ctx context.Context, path string) ([]byte, error) {
	return c.Do(ctx, http.MethodGet, c.url(path), nil)
}

// B2BPost performs a POST to a B2B Edition path.
func (c *B2BClient) B2BPost(ctx context.Context, path string, body any) ([]byte, error) {
	return c.Do(ctx, http.MethodPost, c.url(path), body)
}

// B2BPut performs a PUT to a B2B Edition path.
func (c *B2BClient) B2BPut(ctx context.Context, path string, body any) ([]byte, error) {
	return c.Do(ctx, http.MethodPut, c.url(path), body)
}

// B2BDelete performs a DELETE to a B2B Edition path.
func (c *B2BClient) B2BDelete(ctx context.Context, path string) ([]byte, error) {
	return c.Do(ctx, http.MethodDelete, c.url(path), nil)
}

// B2BPostMultipart POSTs a single file to a B2B Edition path as
// multipart/form-data. Used for file-upload endpoints (e.g. company
// attachments) that do not accept JSON bodies. Applies the same throttling and
// retry policy as Do.
func (c *B2BClient) B2BPostMultipart(ctx context.Context, path, fieldName, fileName string, fileData []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	// The B2B API rejects the default application/octet-stream part type with a
	// 422, so set an explicit Content-Type derived from the file extension
	// (falling back to content sniffing).
	fileContentType := mime.TypeByExtension(filepath.Ext(fileName))
	if fileContentType == "" {
		fileContentType = http.DetectContentType(fileData)
	}
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
			multipartQuoteEscaper.Replace(fieldName), multipartQuoteEscaper.Replace(fileName)))
	partHeader.Set("Content-Type", fileContentType)
	fw, err := w.CreatePart(partHeader)
	if err != nil {
		return nil, fmt.Errorf("create multipart field: %w", err)
	}
	if _, err := fw.Write(fileData); err != nil {
		return nil, fmt.Errorf("write multipart file: %w", err)
	}
	contentType := w.FormDataContentType()
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}
	bodyBytes := buf.Bytes()

	reqURL := c.url(path)
	var lastErr error
	for attempt := range c.maxRetries {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.throttle:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("create B2B multipart request: %w", err)
		}
		req.Header.Set("X-Auth-Token", c.authToken)
		req.Header.Set("X-Store-Hash", c.storeHash)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("B2B multipart request failed: %w", err)
			c.backoff(ctx, attempt)
			continue
		}

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read B2B response: %w", err)
			continue
		}

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return respBody, nil
		case resp.StatusCode == 429:
			c.logger.Warn("B2B rate limited", "attempt", attempt+1)
			c.backoff(ctx, attempt)
			lastErr = fmt.Errorf("B2B rate limited (429)")
		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("B2B server error %d: %s", resp.StatusCode, string(respBody))
			c.logger.Warn("B2B server error", "status", resp.StatusCode, "attempt", attempt+1)
			c.backoff(ctx, attempt)
		default:
			return nil, &APIError{
				StatusCode: resp.StatusCode,
				Body:       respBody,
				Path:       reqURL,
				Method:     http.MethodPost,
			}
		}
	}
	return nil, fmt.Errorf("B2B max retries (%d) exceeded: %w", c.maxRetries, lastErr)
}

// B2BGetAll fetches all pages of a B2B Edition list endpoint using the
// offset-based pagination format: data.list + data.pagination.totalCount.
func (c *B2BClient) B2BGetAll(ctx context.Context, path string) ([]json.RawMessage, error) {
	var all []json.RawMessage
	offset := 0

	sep := "?"
	if bytes.ContainsRune([]byte(path), '?') {
		sep = "&"
	}

	for {
		paged := fmt.Sprintf("%s%soffset=%d&limit=%d", path, sep, offset, b2bDefaultPageSize)
		body, err := c.B2BGet(ctx, paged)
		if err != nil {
			return nil, fmt.Errorf("B2B page offset=%d: %w", offset, err)
		}

		var resp B2BListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parse B2B list response: %w", err)
		}

		var page []json.RawMessage
		if err := json.Unmarshal(resp.Data, &page); err != nil {
			return nil, fmt.Errorf("parse B2B list items: %w", err)
		}
		all = append(all, page...)

		fetched := offset + len(page)
		if fetched >= resp.Meta.Pagination.TotalCount || len(page) < b2bDefaultPageSize {
			break
		}
		offset = fetched
	}
	return all, nil
}

// b2bUnmarshalSingle parses a B2BSingleResponse and unmarshals the inner data
// into dest. Returns an error if the response body is missing data.
func b2bUnmarshalSingle(body []byte, dest any, op string) error {
	var resp B2BSingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("%s: parse response: %w", op, err)
	}
	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		return fmt.Errorf("%s: response missing data", op)
	}
	if err := json.Unmarshal(resp.Data, dest); err != nil {
		return fmt.Errorf("%s: unmarshal: %w", op, err)
	}
	return nil
}

// b2bUnmarshalList parses a B2B response whose `data` field is a JSON array
// (e.g. bulk-create endpoints) and unmarshals it into dest. A null or missing
// data field yields an empty result rather than an error.
func b2bUnmarshalList(body []byte, dest any, op string) error {
	var resp B2BSingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("%s: parse response: %w", op, err)
	}
	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(resp.Data, dest); err != nil {
		return fmt.Errorf("%s: unmarshal: %w", op, err)
	}
	return nil
}
