package b2b

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
)

// Default backoff for B2B Edition's async order index after a Management API
// order create. Live-observed gaps of up to ~25s; this budget covers typical
// cases without hanging tool calls indefinitely (~15.75s worst case).
var b2bOrderIndexBackoff = []time.Duration{
	250 * time.Millisecond,
	500 * time.Millisecond,
	1 * time.Second,
	2 * time.Second,
	3 * time.Second,
	4 * time.Second,
	5 * time.Second,
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (ct *CompanyTools) sleep(ctx context.Context, d time.Duration) error {
	if ct.sleepFn != nil {
		return ct.sleepFn(ctx, d)
	}
	return sleepWithContext(ctx, d)
}

func isB2BOrderNotIndexed(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *bigcommerce.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "status 404") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "not found")
}

func b2bOrderCompanyID(order map[string]any) int {
	raw, ok := order["companyId"]
	if !ok || raw == nil {
		return 0
	}
	switch v := raw.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 0 {
			return 0
		}
		return n
	default:
		return 0
	}
}

// waitForB2BOrder polls GetB2BOrder until the BC order appears in B2B Edition
// (and optionally until companyId is set). Retries only on not-indexed / 404
// style failures; other errors fail immediately.
func (ct *CompanyTools) waitForB2BOrder(ctx context.Context, bcOrderID int, requireCompanyID bool) (map[string]any, error) {
	var lastErr error
	for attempt := 0; attempt <= len(b2bOrderIndexBackoff); attempt++ {
		order, err := ct.bc.GetB2BOrder(ctx, bcOrderID)
		if err == nil {
			if !requireCompanyID || b2bOrderCompanyID(order) > 0 {
				return order, nil
			}
			lastErr = fmt.Errorf("B2B order %d is indexed but companyId is not set yet", bcOrderID)
		} else if !isB2BOrderNotIndexed(err) {
			return nil, err
		} else {
			lastErr = err
		}
		if attempt == len(b2bOrderIndexBackoff) {
			break
		}
		if err := ct.sleep(ctx, b2bOrderIndexBackoff[attempt]); err != nil {
			return nil, err
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("timed out waiting for B2B Edition to index order %d: %w", bcOrderID, lastErr)
	}
	return nil, fmt.Errorf("timed out waiting for B2B Edition to index order %d", bcOrderID)
}
