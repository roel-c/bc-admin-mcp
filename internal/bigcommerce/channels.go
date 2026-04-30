package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// StoreChannel is a row from GET /v3/channels (Management API).
// Extra JSON fields from BigCommerce are ignored until needed.
type StoreChannel struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Platform     string `json:"platform,omitempty"`
	Type         string `json:"type,omitempty"`
	Status       string `json:"status,omitempty"`
	DateCreated  string `json:"date_created,omitempty"`
	DateModified string `json:"date_modified,omitempty"`
}

// ListStoreChannels lists the store’s sales channels via GET /v3/channels
// (Management API for the client’s configured store hash) with optional query
// parameters (e.g. type, type:in, status, available).
func (c *Client) ListStoreChannels(ctx context.Context, params map[string]string) ([]StoreChannel, error) {
	path := "channels"
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
		return nil, fmt.Errorf("list channels: %w", err)
	}
	out := make([]StoreChannel, 0, len(raw))
	for _, r := range raw {
		var ch StoreChannel
		if err := json.Unmarshal(r, &ch); err != nil {
			return nil, fmt.Errorf("unmarshal channel: %w", err)
		}
		out = append(out, ch)
	}
	return out, nil
}
