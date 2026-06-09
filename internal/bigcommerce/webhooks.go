package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// Webhook represents a BigCommerce webhook registration (GET /v3/hooks).
type Webhook struct {
	ID          int               `json:"id"`
	ClientID    string            `json:"client_id,omitempty"`
	StoreHash   string            `json:"store_hash,omitempty"`
	Scope       string            `json:"scope"`
	Destination string            `json:"destination"`
	IsActive    bool              `json:"is_active"`
	ChannelID   int               `json:"channel_id,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	CreatedAt   int64             `json:"created_at,omitempty"`
	UpdatedAt   int64             `json:"updated_at,omitempty"`
}

// WebhookEvent is a delivery-attempt record from GET /v3/hooks/{id}/events.
type WebhookEvent struct {
	ID        int    `json:"id"`
	Scope     string `json:"scope"`
	StoreID   string `json:"store_id,omitempty"`
	Producer  string `json:"producer,omitempty"`
	CreatedAt int64  `json:"created_at,omitempty"`
}

// WebhookCreate is the request body for POST /v3/hooks.
type WebhookCreate struct {
	Scope       string            `json:"scope"`
	Destination string            `json:"destination"`
	IsActive    bool              `json:"is_active"`
	ChannelID   int               `json:"channel_id,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// WebhookUpdate is the merged-full-record body for PUT /v3/hooks/{id}.
// All mutable fields are included (scope + destination are required by BC;
// channel_id is immutable after creation and excluded).
type WebhookUpdate struct {
	Scope       string            `json:"scope"`
	Destination string            `json:"destination"`
	IsActive    bool              `json:"is_active"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// ListWebhooks lists webhook registrations via GET /v3/hooks with optional
// query parameters (e.g. scope, is_active, channel_id).
func (c *Client) ListWebhooks(ctx context.Context, params map[string]string) ([]Webhook, error) {
	path := "hooks"
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
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	out := make([]Webhook, 0, len(raw))
	for _, r := range raw {
		var wh Webhook
		if err := json.Unmarshal(r, &wh); err != nil {
			return nil, fmt.Errorf("unmarshal webhook: %w", err)
		}
		out = append(out, wh)
	}
	return out, nil
}

// GetWebhook fetches a single webhook via GET /v3/hooks/{id}.
func (c *Client) GetWebhook(ctx context.Context, hookID int) (*Webhook, error) {
	body, err := c.Get(ctx, fmt.Sprintf("hooks/%d", hookID))
	if err != nil {
		return nil, fmt.Errorf("get webhook %d: %w", hookID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse get webhook %d response: %w", hookID, err)
	}
	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		return nil, fmt.Errorf("get webhook %d response missing data", hookID)
	}
	var wh Webhook
	if err := json.Unmarshal(resp.Data, &wh); err != nil {
		return nil, fmt.Errorf("unmarshal webhook %d: %w", hookID, err)
	}
	return &wh, nil
}

// GetWebhookEvents fetches recent delivery events via GET /v3/hooks/{id}/events.
func (c *Client) GetWebhookEvents(ctx context.Context, hookID int) ([]WebhookEvent, error) {
	raw, err := c.GetAll(ctx, fmt.Sprintf("hooks/%d/events", hookID))
	if err != nil {
		return nil, fmt.Errorf("get webhook %d events: %w", hookID, err)
	}
	out := make([]WebhookEvent, 0, len(raw))
	for _, r := range raw {
		var ev WebhookEvent
		if err := json.Unmarshal(r, &ev); err != nil {
			return nil, fmt.Errorf("unmarshal webhook event: %w", err)
		}
		out = append(out, ev)
	}
	return out, nil
}

// CreateWebhook registers a new webhook via POST /v3/hooks.
func (c *Client) CreateWebhook(ctx context.Context, payload WebhookCreate) (*Webhook, error) {
	body, err := c.Post(ctx, "hooks", payload)
	if err != nil {
		return nil, fmt.Errorf("create webhook: %w", err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse create webhook response: %w", err)
	}
	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		return nil, fmt.Errorf("create webhook response missing data")
	}
	var wh Webhook
	if err := json.Unmarshal(resp.Data, &wh); err != nil {
		return nil, fmt.Errorf("unmarshal created webhook: %w", err)
	}
	return &wh, nil
}

// UpdateWebhook updates a webhook's mutable fields via PUT /v3/hooks/{id}.
// The payload must include scope and destination (BC requires both on PUT).
func (c *Client) UpdateWebhook(ctx context.Context, hookID int, payload WebhookUpdate) (*Webhook, error) {
	body, err := c.Put(ctx, fmt.Sprintf("hooks/%d", hookID), payload)
	if err != nil {
		return nil, fmt.Errorf("update webhook %d: %w", hookID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse update webhook %d response: %w", hookID, err)
	}
	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		return nil, fmt.Errorf("update webhook %d response missing data", hookID)
	}
	var wh Webhook
	if err := json.Unmarshal(resp.Data, &wh); err != nil {
		return nil, fmt.Errorf("unmarshal updated webhook %d: %w", hookID, err)
	}
	return &wh, nil
}

// DeleteWebhook removes a webhook registration via DELETE /v3/hooks/{id}.
func (c *Client) DeleteWebhook(ctx context.Context, hookID int) error {
	_, err := c.Delete(ctx, fmt.Sprintf("hooks/%d", hookID))
	if err != nil {
		return fmt.Errorf("delete webhook %d: %w", hookID, err)
	}
	return nil
}
