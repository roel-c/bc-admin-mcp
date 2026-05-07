package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
)

// GetGlobalCustomerSettings returns GET /v3/customers/settings (global).
func (c *Client) GetGlobalCustomerSettings(ctx context.Context) (*CustomerGlobalSettings, error) {
	body, err := c.Get(ctx, "customers/settings")
	if err != nil {
		return nil, fmt.Errorf("get global customer settings: %w", err)
	}
	var wrap struct {
		Data CustomerGlobalSettings `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("parse global customer settings: %w", err)
	}
	return &wrap.Data, nil
}

// UpdateGlobalCustomerSettings applies PUT /v3/customers/settings.
func (c *Client) UpdateGlobalCustomerSettings(ctx context.Context, payload CustomerGlobalSettings) (*CustomerGlobalSettings, error) {
	body, err := c.Put(ctx, "customers/settings", payload)
	if err != nil {
		return nil, fmt.Errorf("update global customer settings: %w", err)
	}
	var wrap struct {
		Data CustomerGlobalSettings `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("parse update global customer settings response: %w", err)
	}
	return &wrap.Data, nil
}

// GetChannelCustomerSettings returns GET /v3/customers/settings/channels/{channel_id}.
func (c *Client) GetChannelCustomerSettings(ctx context.Context, channelID int) (*CustomerChannelSettings, error) {
	if channelID <= 0 {
		return nil, fmt.Errorf("channel_id must be positive")
	}
	path := fmt.Sprintf("customers/settings/channels/%d", channelID)
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("get channel customer settings: %w", err)
	}
	var wrap struct {
		Data CustomerChannelSettings `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("parse channel customer settings: %w", err)
	}
	return &wrap.Data, nil
}

// UpdateChannelCustomerSettings applies PUT /v3/customers/settings/channels/{channel_id}.
func (c *Client) UpdateChannelCustomerSettings(ctx context.Context, channelID int, payload CustomerChannelSettings) (*CustomerChannelSettings, error) {
	if channelID <= 0 {
		return nil, fmt.Errorf("channel_id must be positive")
	}
	path := fmt.Sprintf("customers/settings/channels/%d", channelID)
	body, err := c.Put(ctx, path, payload)
	if err != nil {
		return nil, fmt.Errorf("update channel customer settings: %w", err)
	}
	var wrap struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("parse update channel customer settings response: %w", err)
	}
	// BC may return CustomerSettingsObject or CustomerChannelSettingsObject; decode flexibly.
	var ch CustomerChannelSettings
	if err := json.Unmarshal(wrap.Data, &ch); err != nil {
		return nil, fmt.Errorf("decode channel settings data: %w", err)
	}
	return &ch, nil
}

// GetCustomerConsent performs GET /v3/customers/{customerId}/consent.
func (c *Client) GetCustomerConsent(ctx context.Context, customerID int) (*CustomerConsent, error) {
	if customerID <= 0 {
		return nil, fmt.Errorf("customer_id must be positive")
	}
	path := fmt.Sprintf("customers/%d/consent", customerID)
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("get customer consent: %w", err)
	}
	var out CustomerConsent
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse customer consent: %w", err)
	}
	return &out, nil
}

// UpdateCustomerConsent applies PUT /v3/customers/{customerId}/consent.
func (c *Client) UpdateCustomerConsent(ctx context.Context, customerID int, req DeclareCustomerConsentRequest) (*CustomerConsent, error) {
	if customerID <= 0 {
		return nil, fmt.Errorf("customer_id must be positive")
	}
	path := fmt.Sprintf("customers/%d/consent", customerID)
	body, err := c.Put(ctx, path, req)
	if err != nil {
		return nil, fmt.Errorf("update customer consent: %w", err)
	}
	var out CustomerConsent
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse customer consent response: %w", err)
	}
	return &out, nil
}

// ListCustomerStoredInstruments returns GET /v3/customers/{customerId}/stored-instruments
// as raw JSON rows (array or {data: [...]}).
func (c *Client) ListCustomerStoredInstruments(ctx context.Context, customerID int) ([]json.RawMessage, error) {
	if customerID <= 0 {
		return nil, fmt.Errorf("customer_id must be positive")
	}
	path := fmt.Sprintf("customers/%d/stored-instruments", customerID)
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list stored instruments: %w", err)
	}
	return parseJSONArrayOrEnvelope(body)
}

func parseJSONArrayOrEnvelope(body []byte) ([]json.RawMessage, error) {
	if len(body) > 0 && body[0] == '[' {
		var direct []json.RawMessage
		if err := json.Unmarshal(body, &direct); err != nil {
			return nil, fmt.Errorf("parse stored instruments array: %w", err)
		}
		return direct, nil
	}
	var wrap struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("parse stored instruments envelope: %w", err)
	}
	if wrap.Data == nil {
		return nil, fmt.Errorf("stored instruments response missing data array")
	}
	return wrap.Data, nil
}

// ValidateCustomerCredentials calls POST /v3/customers/validate-credentials.
// This endpoint has strict rate limits; callers should avoid abuse.
func (c *Client) ValidateCustomerCredentials(ctx context.Context, req ValidateCustomerCredentialsRequest) (*ValidateCustomerCredentialsResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, fmt.Errorf("email and password are required")
	}
	body, err := c.Post(ctx, "customers/validate-credentials", req)
	if err != nil {
		return nil, fmt.Errorf("validate customer credentials: %w", err)
	}
	var out ValidateCustomerCredentialsResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse validate credentials response: %w", err)
	}
	return &out, nil
}
