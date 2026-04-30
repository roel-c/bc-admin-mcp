package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ListProductChannelAssignments returns rows from GET /v3/catalog/products/channel-assignments.
// Typical query keys: product_id:in, channel_id:in (comma-separated IDs per BigCommerce).
func (c *Client) ListProductChannelAssignments(ctx context.Context, params map[string]string) ([]ProductChannelAssignment, error) {
	path := "catalog/products/channel-assignments"
	if len(params) > 0 {
		vals := url.Values{}
		for k, v := range params {
			if v != "" {
				vals.Set(k, v)
			}
		}
		if enc := vals.Encode(); enc != "" {
			path += "?" + enc
		}
	}

	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list product channel assignments: %w", err)
	}

	out := make([]ProductChannelAssignment, 0, len(raw))
	for _, r := range raw {
		var row ProductChannelAssignment
		if err := json.Unmarshal(r, &row); err != nil {
			return nil, fmt.Errorf("unmarshal channel assignment: %w", err)
		}
		out = append(out, row)
	}
	return out, nil
}

// UpsertProductChannelAssignments creates assignments via PUT /v3/catalog/products/channel-assignments.
// Uses configured ProductBatchSize chunks. Avoid overlapping parallel calls for the same product IDs.
func (c *Client) UpsertProductChannelAssignments(ctx context.Context, assignments []ProductChannelAssignment) error {
	if len(assignments) == 0 {
		return fmt.Errorf("no assignments to upsert")
	}
	items := make([]any, len(assignments))
	for i := range assignments {
		items[i] = assignments[i]
	}
	result, err := c.BatchPut(ctx, "catalog/products/channel-assignments", items, c.cfg.ProductBatchSize)
	if err != nil {
		return err
	}
	if result.Failed > 0 {
		var b strings.Builder
		for _, e := range result.Errors {
			b.WriteString(e.Err)
			b.WriteString("; ")
		}
		return fmt.Errorf("channel assignment batch failed for %d row(s): %s", result.Failed, strings.TrimSuffix(b.String(), "; "))
	}
	return nil
}

// DeleteProductChannelAssignments removes assignments via DELETE /v3/catalog/products/channel-assignments.
// BigCommerce requires at least one of product_id:in or channel_id:in in the query string.
func (c *Client) DeleteProductChannelAssignments(ctx context.Context, productIDs, channelIDs []int) error {
	if len(productIDs) == 0 && len(channelIDs) == 0 {
		return fmt.Errorf("at least one of product IDs or channel IDs is required for delete")
	}
	vals := url.Values{}
	if len(productIDs) > 0 {
		vals.Set("product_id:in", joinInts(productIDs))
	}
	if len(channelIDs) > 0 {
		vals.Set("channel_id:in", joinInts(channelIDs))
	}
	path := "catalog/products/channel-assignments?" + vals.Encode()
	_, err := c.Delete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete product channel assignments: %w", err)
	}
	return nil
}

func joinInts(ids []int) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}
