package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// Script kind constants.
const (
	ScriptKindSrc       = "src"
	ScriptKindScriptTag = "script_tag"
)

// Script load_method constants.
const (
	ScriptLoadDefault = "default"
	ScriptLoadAsync   = "async"
	ScriptLoadDefer   = "defer"
)

// Script location constants.
const (
	ScriptLocationHead   = "head"
	ScriptLocationFooter = "footer"
)

// Script visibility constants.
const (
	ScriptVisibilityStorefront        = "storefront"
	ScriptVisibilityAllPages          = "all_pages"
	ScriptVisibilityCheckout          = "checkout"
	ScriptVisibilityOrderConfirmation = "order_confirmation"
)

// Script consent_category constants.
const (
	ScriptConsentEssential  = "essential"
	ScriptConsentFunctional = "functional"
	ScriptConsentAnalytics  = "analytics"
	ScriptConsentTargeting  = "targeting"
)

// Script is the read shape for GET /v3/content/scripts and /v3/content/scripts/{uuid}.
type Script struct {
	UUID            string `json:"uuid,omitempty"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	Kind            string `json:"kind,omitempty"`
	Src             string `json:"src,omitempty"`
	HTML            string `json:"html,omitempty"`
	LoadMethod      string `json:"load_method,omitempty"`
	Location        string `json:"location,omitempty"`
	Visibility      string `json:"visibility,omitempty"`
	ConsentCategory string `json:"consent_category,omitempty"`
	AutoUninstall   bool   `json:"auto_uninstall"`
	Enabled         bool   `json:"enabled"`
	ChannelID       int    `json:"channel_id,omitempty"`
	APIClientID     string `json:"api_client_id,omitempty"`
	DateCreated     string `json:"date_created,omitempty"`
	DateModified    string `json:"date_modified,omitempty"`
}

// ScriptCreate is the POST body for /v3/content/scripts.
type ScriptCreate struct {
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	Kind            string `json:"kind,omitempty"`
	Src             string `json:"src,omitempty"`
	HTML            string `json:"html,omitempty"`
	LoadMethod      string `json:"load_method,omitempty"`
	Location        string `json:"location,omitempty"`
	Visibility      string `json:"visibility,omitempty"`
	ConsentCategory string `json:"consent_category,omitempty"`
	AutoUninstall   *bool  `json:"auto_uninstall,omitempty"`
	Enabled         *bool  `json:"enabled,omitempty"`
	ChannelID       int    `json:"channel_id,omitempty"`
}

// ScriptUpdate is the PUT body for /v3/content/scripts/{uuid}.
// Pointer fields distinguish "not supplied" from "explicitly set/cleared".
type ScriptUpdate struct {
	Name            *string `json:"name,omitempty"`
	Description     *string `json:"description,omitempty"`
	Src             *string `json:"src,omitempty"`
	HTML            *string `json:"html,omitempty"`
	LoadMethod      *string `json:"load_method,omitempty"`
	Location        *string `json:"location,omitempty"`
	Visibility      *string `json:"visibility,omitempty"`
	ConsentCategory *string `json:"consent_category,omitempty"`
	AutoUninstall   *bool   `json:"auto_uninstall,omitempty"`
	Enabled         *bool   `json:"enabled,omitempty"`
}

// ScriptListParams encodes GET /v3/content/scripts query parameters.
type ScriptListParams struct {
	Page      int
	Limit     int
	Sort      string // "name" | "description" | "date_created" | "date_modified"
	Direction string // "asc" | "desc"
	ChannelID int    // maps to channel_id:in
}

// ListScripts fetches scripts from /v3/content/scripts.
func (c *Client) ListScripts(ctx context.Context, params ScriptListParams) ([]Script, error) {
	q := url.Values{}
	if params.Page > 0 {
		q.Set("page", strconv.Itoa(params.Page))
	}
	if params.Limit > 0 {
		q.Set("limit", strconv.Itoa(params.Limit))
	}
	if params.Sort != "" {
		q.Set("sort", params.Sort)
	}
	if params.Direction != "" {
		q.Set("direction", params.Direction)
	}
	if params.ChannelID > 0 {
		q.Set("channel_id:in", strconv.Itoa(params.ChannelID))
	}

	path := "content/scripts"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []Script `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse scripts list: %w", err)
	}
	return resp.Data, nil
}

// GetScript fetches a single script by UUID from /v3/content/scripts/{uuid}.
func (c *Client) GetScript(ctx context.Context, uuid string) (*Script, error) {
	body, err := c.Get(ctx, "content/scripts/"+uuid)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data Script `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse script: %w", err)
	}
	return &resp.Data, nil
}

// CreateScript creates a new script via POST /v3/content/scripts.
func (c *Client) CreateScript(ctx context.Context, payload ScriptCreate) (*Script, error) {
	body, err := c.Post(ctx, "content/scripts", payload)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data Script `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse created script: %w", err)
	}
	return &resp.Data, nil
}

// UpdateScript updates an existing script via PUT /v3/content/scripts/{uuid}.
func (c *Client) UpdateScript(ctx context.Context, uuid string, payload ScriptUpdate) (*Script, error) {
	body, err := c.Put(ctx, "content/scripts/"+uuid, payload)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data Script `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse updated script: %w", err)
	}
	return &resp.Data, nil
}

// DeleteScript deletes a script by UUID via DELETE /v3/content/scripts/{uuid}.
func (c *Client) DeleteScript(ctx context.Context, uuid string) error {
	_, err := c.Delete(ctx, "content/scripts/"+uuid)
	return err
}
