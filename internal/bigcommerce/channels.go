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

// StoreChannelUpdate is the request body for PUT /v3/channels/{id}.
// platform and type are immutable after create and must not be included.
// Only fields with non-zero values are serialised (omitempty).
type StoreChannelUpdate struct {
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"`
}

// GetStoreChannel fetches a single channel by ID via GET /v3/channels/{id}.
func (c *Client) GetStoreChannel(ctx context.Context, channelID int) (*StoreChannel, error) {
	body, err := c.Get(ctx, fmt.Sprintf("channels/%d", channelID))
	if err != nil {
		return nil, fmt.Errorf("get channel %d: %w", channelID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse get channel %d response: %w", channelID, err)
	}
	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		return nil, fmt.Errorf("get channel %d response missing data", channelID)
	}
	var ch StoreChannel
	if err := json.Unmarshal(resp.Data, &ch); err != nil {
		return nil, fmt.Errorf("unmarshal channel %d: %w", channelID, err)
	}
	return &ch, nil
}

// UpdateStoreChannel updates a channel's mutable fields via PUT /v3/channels/{id}.
// platform and type cannot be updated after creation (BigCommerce API constraint).
func (c *Client) UpdateStoreChannel(ctx context.Context, channelID int, payload StoreChannelUpdate) (*StoreChannel, error) {
	body, err := c.Put(ctx, fmt.Sprintf("channels/%d", channelID), payload)
	if err != nil {
		return nil, fmt.Errorf("update channel %d: %w", channelID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse update channel %d response: %w", channelID, err)
	}
	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		return nil, fmt.Errorf("update channel %d response missing data", channelID)
	}
	var ch StoreChannel
	if err := json.Unmarshal(resp.Data, &ch); err != nil {
		return nil, fmt.Errorf("unmarshal updated channel %d: %w", channelID, err)
	}
	return &ch, nil
}

// ListStoreChannels lists the store's sales channels via GET /v3/channels
// (Management API for the client's configured store hash) with optional query
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
