package catalog

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// ImageURLProber checks that a product image URL is publicly fetchable before
// BigCommerce attempts to download it (BC returns opaque 422s for bad URLs).
type ImageURLProber interface {
	Probe(ctx context.Context, imageURL string) error
}

type httpImageURLProber struct {
	client *http.Client
}

func newHTTPImageURLProber() *httpImageURLProber {
	return &httpImageURLProber{
		client: &http.Client{Timeout: 3 * time.Second},
	}
}

func (h *httpImageURLProber) Probe(ctx context.Context, imageURL string) error {
	if err := h.probeMethod(ctx, http.MethodHead, imageURL); err == nil {
		return nil
	}
	// Some CDNs reject HEAD; fall back to GET.
	return h.probeMethod(ctx, http.MethodGet, imageURL)
}

func (h *httpImageURLProber) probeMethod(ctx context.Context, method, imageURL string) error {
	req, err := http.NewRequestWithContext(ctx, method, imageURL, nil)
	if err != nil {
		return fmt.Errorf("invalid image_url %q: %w", imageURL, err)
	}
	req.Header.Set("User-Agent", "bc-admin-mcp-image-probe/1.0")
	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("image_url not reachable (%s %s): %v", method, imageURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("image_url returned HTTP %d (%s %s) — use a publicly fetchable URL (e.g. resolve Wikimedia Special:FilePath to the final upload.wikimedia.org URL and verify HTTP 200)", resp.StatusCode, method, imageURL)
	}
	return nil
}

// noopImageURLProber always succeeds — used in unit tests that do not hit the network.
type noopImageURLProber struct{}

func (noopImageURLProber) Probe(context.Context, string) error { return nil }

// NoopImageURLProber returns a prober that never fails (for unit tests).
func NoopImageURLProber() ImageURLProber { return noopImageURLProber{} }
