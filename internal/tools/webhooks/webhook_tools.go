package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/session"
)

// Webhooks provides MCP tool handlers for the BigCommerce Webhooks API
// (/v3/hooks): list, get, view events, create, update, and delete webhook
// registrations for the connected store.
type Webhooks struct {
	bc    WebhooksAPI
	cache *session.Store
}

// NewWebhooks constructs a Webhooks handler wrapping the given BC client.
func NewWebhooks(bc WebhooksAPI, cache *session.Store) *Webhooks {
	return &Webhooks{bc: bc, cache: cache}
}

func webhookCacheKey(hookID int) string {
	return fmt.Sprintf("webhook:%d", hookID)
}

// RegisterTools wires all webhook tools into the discovery registry.
func (w *Webhooks) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "webhooks/list",
		Tier:    middleware.TierR0,
		Summary: "List webhook registrations for the connected store",
		Description: "GET /v3/hooks — returns all webhook registrations visible to this API account. " +
			"Optional filters: scope (exact event string, e.g. store/order/created), " +
			"is_active (true/false), channel_id (integer). " +
			"Requires OAuth scope store_v2_information_read_only (or store_v2_information).",
		Tool: mcp.NewTool("webhooks_list",
			mcp.WithDescription(
				"List all webhook registrations for this store. "+
					"Filter by scope, is_active, or channel_id. "+
					"Use webhooks/get to see full details for a specific webhook.",
			),
			mcp.WithString("scope",
				mcp.Description("Optional: filter by exact BC event scope, e.g. store/order/created."),
			),
			mcp.WithBoolean("is_active",
				mcp.Description("Optional: true to list only active webhooks, false for inactive."),
			),
			mcp.WithNumber("channel_id",
				mcp.Description("Optional: filter webhooks scoped to a specific channel ID."),
			),
		),
		Handler: w.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "webhooks/get",
		Tier:    middleware.TierR0,
		Summary: "Get a single webhook registration by ID",
		Description: "GET /v3/hooks/{id} — returns full details for one webhook: " +
			"scope, destination, is_active, channel_id, headers, created_at, updated_at. " +
			"Requires OAuth scope store_v2_information_read_only (or store_v2_information).",
		Tool: mcp.NewTool("webhooks_get",
			mcp.WithDescription(
				"Get full details for a single webhook by numeric ID. "+
					"Use webhooks/list to discover webhook IDs.",
			),
			mcp.WithNumber("id", mcp.Description("Webhook ID to fetch."), mcp.Required()),
		),
		Handler: w.handleGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "webhooks/events",
		Tier:    middleware.TierR0,
		Summary: "List recent delivery events for a webhook",
		Description: "GET /v3/hooks/{id}/events — returns recent delivery attempts for the given webhook. " +
			"Each event includes the event scope, store_id, producer, and timestamp. " +
			"Useful for diagnosing delivery failures or confirming successful deliveries. " +
			"Requires OAuth scope store_v2_information_read_only (or store_v2_information).",
		Tool: mcp.NewTool("webhooks_events",
			mcp.WithDescription(
				"List recent delivery events for a webhook. "+
					"Returns event scope, producer, and timestamps for each attempted delivery.",
			),
			mcp.WithNumber("id", mcp.Description("Webhook ID to fetch events for."), mcp.Required()),
		),
		Handler: w.handleEvents,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "webhooks/create",
		Tier:    middleware.TierR1,
		Summary: "Register a new webhook (preview → confirm)",
		Description: "POST /v3/hooks — creates a new webhook registration. " +
			"destination must be an HTTPS URL (BC rejects plain HTTP). " +
			"scope is passed through to BC; invalid scopes return a BC 422. " +
			"channel_id is optional — omit for store-wide webhooks, or provide a channel ID to scope delivery. " +
			"headers_json is an optional JSON object of string→string custom headers to include in each delivery. " +
			"is_active defaults to true when omitted. " +
			"WARNING: creating a webhook establishes a persistent outbound data stream to the destination URL. " +
			"Preview shows the payload before registration. Pass confirmed=true to apply. " +
			"Requires OAuth scope store_v2_information.",
		Tool: mcp.NewTool("webhooks_create",
			mcp.WithDescription(
				"Register a new BigCommerce webhook. destination must be HTTPS. "+
					"Preview shows the registration payload; pass confirmed=true to create.",
			),
			mcp.WithString("scope",
				mcp.Description("BigCommerce event scope, e.g. store/order/created, store/product/updated."),
				mcp.Required(),
			),
			mcp.WithString("destination",
				mcp.Description("HTTPS URL that BigCommerce will POST events to."),
				mcp.Required(),
			),
			mcp.WithBoolean("is_active",
				mcp.Description("Whether the webhook is active. Defaults to true when omitted."),
			),
			mcp.WithNumber("channel_id",
				mcp.Description("Optional: scope this webhook to a specific channel ID. Omit for store-wide delivery."),
			),
			mcp.WithString("headers_json",
				mcp.Description(`Optional: JSON object of custom headers to include in deliveries, e.g. {"X-Auth": "secret"}.`),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set true to register after reviewing the preview."),
			),
		),
		Handler: w.handleCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "webhooks/update",
		Tier:    middleware.TierR1,
		Summary: "Update a webhook's scope, destination, active status, or headers (preview → confirm)",
		Description: "PUT /v3/hooks/{id} — updates a webhook. " +
			"Fetches the current record then applies the provided changes (fetch-merge-PUT). " +
			"At least one of scope, destination, is_active, or headers_json must be supplied. " +
			"channel_id is immutable after creation and cannot be changed. " +
			"WARNING: updating destination changes where BC delivers store event data. " +
			"Preview shows current vs would_apply values (header values are redacted). Pass confirmed=true to apply. " +
			"Requires OAuth scope store_v2_information.",
		Tool: mcp.NewTool("webhooks_update",
			mcp.WithDescription(
				"Update a webhook. Fetch-merge-PUT: only the fields you provide are changed. "+
					"Preview shows current vs would_apply; pass confirmed=true to apply.",
			),
			mcp.WithNumber("id", mcp.Description("Webhook ID to update."), mcp.Required()),
			mcp.WithString("scope",
				mcp.Description("New BC event scope, e.g. store/order/statusUpdated."),
			),
			mcp.WithString("destination",
				mcp.Description("New HTTPS destination URL."),
			),
			mcp.WithBoolean("is_active",
				mcp.Description("Set true to activate, false to deactivate."),
			),
			mcp.WithString("headers_json",
				mcp.Description(`New custom headers JSON object, e.g. {"X-Auth": "secret"}. Replaces existing headers entirely.`),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set true to apply after reviewing the preview."),
			),
		),
		Handler: w.handleUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "webhooks/delete",
		Tier:    middleware.TierR3,
		Summary: "Permanently delete a webhook registration (preview → confirm)",
		Description: "DELETE /v3/hooks/{id} — permanently removes a webhook registration. " +
			"This cannot be undone. Preview shows the webhook's scope and destination before deletion. " +
			"Requires OAuth scope store_v2_information.",
		Tool: mcp.NewTool("webhooks_delete",
			mcp.WithDescription(
				"Permanently delete a webhook registration. This cannot be undone. "+
					"Preview shows the webhook details; pass confirmed=true to delete.",
			),
			mcp.WithNumber("id", mcp.Description("Webhook ID to delete."), mcp.Required()),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set true to permanently delete after reviewing the preview."),
			),
		),
		Handler: w.handleDelete,
	})
}

func (w *Webhooks) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := make(map[string]string)

	if v, ok := args["scope"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return toolError("scope must be a string"), nil
		}
		if t := strings.TrimSpace(s); t != "" {
			params["scope"] = t
		}
	}
	if v, ok := args["is_active"]; ok && v != nil {
		b, ok := v.(bool)
		if !ok {
			return toolError("is_active must be a boolean"), nil
		}
		if b {
			params["is_active"] = "true"
		} else {
			params["is_active"] = "false"
		}
	}
	if v, ok := args["channel_id"]; ok && v != nil {
		chID, err := argPositiveInt("channel_id", v)
		if err != nil {
			return toolError("%s", err.Error()), nil
		}
		params["channel_id"] = fmt.Sprintf("%d", chID)
	}

	var query map[string]string
	if len(params) > 0 {
		query = params
	}

	hooks, err := w.bc.ListWebhooks(ctx, query)
	if err != nil {
		return toolError("failed to list webhooks: %v", err), nil
	}

	return toolJSON(map[string]any{
		"total":    len(hooks),
		"webhooks": webhookViews(hooks),
		"api":      "GET /v3/hooks",
	})
}

func (w *Webhooks) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	hookID, err := argPositiveInt("id", args["id"])
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	hook, err := w.bc.GetWebhook(ctx, hookID)
	if err != nil {
		return toolError("failed to get webhook %d: %v", hookID, err), nil
	}

	return toolJSON(map[string]any{
		"webhook": webhookView(*hook),
		"api":     fmt.Sprintf("GET /v3/hooks/%d", hookID),
	})
}

func (w *Webhooks) handleEvents(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	hookID, err := argPositiveInt("id", args["id"])
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	events, err := w.bc.GetWebhookEvents(ctx, hookID)
	if err != nil {
		// BigCommerce returns 404 for a webhook that has no recorded delivery
		// history — that's "no events yet", not "hook not found". Return an
		// empty result with a note rather than a misleading error.
		var apiErr *bigcommerce.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return toolJSON(map[string]any{
				"hook_id": hookID,
				"total":   0,
				"events":  []any{},
				"note":    "No delivery events recorded for this webhook yet (BigCommerce returns 404 for hooks with no event history). If you expected the hook itself to exist, confirm it via webhooks/get.",
				"api":     fmt.Sprintf("GET /v3/hooks/%d/events", hookID),
			})
		}
		return toolError("failed to get events for webhook %d: %v", hookID, err), nil
	}

	return toolJSON(map[string]any{
		"hook_id": hookID,
		"total":   len(events),
		"events":  events,
		"api":     fmt.Sprintf("GET /v3/hooks/%d/events", hookID),
	})
}

func (w *Webhooks) handleCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	scope, ok := args["scope"].(string)
	if !ok || strings.TrimSpace(scope) == "" {
		return toolError("scope is required and must be a non-empty string"), nil
	}
	scope = strings.TrimSpace(scope)

	destination, ok := args["destination"].(string)
	if !ok || strings.TrimSpace(destination) == "" {
		return toolError("destination is required and must be a non-empty string"), nil
	}
	destination = strings.TrimSpace(destination)
	if !strings.HasPrefix(destination, "https://") {
		return toolError("destination must be an HTTPS URL (BigCommerce rejects plain HTTP)"), nil
	}

	isActive := true
	if v, ok := args["is_active"]; ok && v != nil {
		b, ok := v.(bool)
		if !ok {
			return toolError("is_active must be a boolean"), nil
		}
		isActive = b
	}

	var channelID int
	if v, ok := args["channel_id"]; ok && v != nil {
		id, err := argPositiveInt("channel_id", v)
		if err != nil {
			return toolError("%s", err.Error()), nil
		}
		channelID = id
	}

	var headers map[string]string
	if v, ok := args["headers_json"]; ok && v != nil {
		h, err := parseHeadersJSON(v)
		if err != nil {
			return toolError("%s", err.Error()), nil
		}
		headers = h
	}

	payload := bigcommerce.WebhookCreate{
		Scope:       scope,
		Destination: destination,
		IsActive:    isActive,
		ChannelID:   channelID,
		Headers:     headers,
	}

	if !middleware.IsConfirmed(request) {
		preview := map[string]any{
			"scope":       scope,
			"destination": destination,
			"is_active":   isActive,
		}
		if channelID > 0 {
			preview["channel_id"] = channelID
		}
		if len(headers) > 0 {
			preview["headers"] = redactHeaders(headers)
		}
		return toolJSON(map[string]any{
			"status":  "pending_confirmation",
			"payload": preview,
			"message": "Review the webhook registration above. Pass confirmed=true to create.",
			"api":     "POST /v3/hooks",
		})
	}

	created, err := w.bc.CreateWebhook(ctx, payload)
	if err != nil {
		return toolError("failed to create webhook: %v", err), nil
	}

	return toolJSON(map[string]any{
		"status":  "created",
		"webhook": webhookView(*created),
		"api":     "POST /v3/hooks",
	})
}

func (w *Webhooks) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	hookID, err := argPositiveInt("id", args["id"])
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	// Validate update fields before fetching current state.
	var (
		newScope       string
		newDestination string
		newIsActive    *bool
		newHeaders     map[string]string
		hasAnyField    bool
	)

	if v, ok := args["scope"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return toolError("scope must be a string"), nil
		}
		if strings.TrimSpace(s) == "" {
			return toolError("scope must not be empty when provided"), nil
		}
		newScope = strings.TrimSpace(s)
		hasAnyField = true
	}
	if v, ok := args["destination"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return toolError("destination must be a string"), nil
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return toolError("destination must not be empty when provided"), nil
		}
		if !strings.HasPrefix(s, "https://") {
			return toolError("destination must be an HTTPS URL (BigCommerce rejects plain HTTP)"), nil
		}
		newDestination = s
		hasAnyField = true
	}
	if v, ok := args["is_active"]; ok && v != nil {
		b, ok := v.(bool)
		if !ok {
			return toolError("is_active must be a boolean"), nil
		}
		newIsActive = &b
		hasAnyField = true
	}
	if v, ok := args["headers_json"]; ok && v != nil {
		h, err := parseHeadersJSON(v)
		if err != nil {
			return toolError("%s", err.Error()), nil
		}
		newHeaders = h
		hasAnyField = true
	}

	if !hasAnyField {
		return toolError("at least one of scope, destination, is_active, or headers_json must be provided"), nil
	}

	// Fetch current state for merge and preview (cached across preview→confirm).
	cacheKey := webhookCacheKey(hookID)
	current, err := session.CacheOrFetch(w.cache.ForContext(ctx), cacheKey, func() (*bigcommerce.Webhook, error) {
		return w.bc.GetWebhook(ctx, hookID)
	})
	if err != nil {
		return toolError("failed to fetch webhook %d for preview: %v", hookID, err), nil
	}

	// Merge: start with current values, override with any provided fields.
	merged := bigcommerce.WebhookUpdate{
		Scope:       current.Scope,
		Destination: current.Destination,
		IsActive:    current.IsActive,
		Headers:     current.Headers,
	}
	if newScope != "" {
		merged.Scope = newScope
	}
	if newDestination != "" {
		merged.Destination = newDestination
	}
	if newIsActive != nil {
		merged.IsActive = *newIsActive
	}
	if newHeaders != nil {
		merged.Headers = newHeaders
	}

	currentView := map[string]any{
		"scope":       current.Scope,
		"destination": current.Destination,
		"is_active":   current.IsActive,
	}
	if len(current.Headers) > 0 {
		currentView["headers"] = redactHeaders(current.Headers)
	}
	wouldApply := map[string]any{
		"scope":       merged.Scope,
		"destination": merged.Destination,
		"is_active":   merged.IsActive,
	}
	if len(merged.Headers) > 0 {
		wouldApply["headers"] = redactHeaders(merged.Headers)
	}

	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status":      "pending_confirmation",
			"hook_id":     hookID,
			"current":     currentView,
			"would_apply": wouldApply,
			"message":     fmt.Sprintf("Will update webhook %d. Pass confirmed=true to apply.", hookID),
			"api":         fmt.Sprintf("PUT /v3/hooks/%d", hookID),
		})
	}

	w.cache.ForContext(ctx).Delete(cacheKey)
	updated, err := w.bc.UpdateWebhook(ctx, hookID, merged)
	if err != nil {
		return toolError("failed to update webhook %d: %v", hookID, err), nil
	}

	return toolJSON(map[string]any{
		"status":  "updated",
		"webhook": webhookView(*updated),
		"api":     fmt.Sprintf("PUT /v3/hooks/%d", hookID),
	})
}

func (w *Webhooks) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	hookID, err := argPositiveInt("id", args["id"])
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	// Fetch first so the preview is informative and the ID is verified (cached across preview→confirm).
	cacheKey := webhookCacheKey(hookID)
	hook, err := session.CacheOrFetch(w.cache.ForContext(ctx), cacheKey, func() (*bigcommerce.Webhook, error) {
		return w.bc.GetWebhook(ctx, hookID)
	})
	if err != nil {
		return toolError("failed to fetch webhook %d: %v", hookID, err), nil
	}

	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status":      "pending_confirmation",
			"hook_id":     hookID,
			"scope":       hook.Scope,
			"destination": hook.Destination,
			"is_active":   hook.IsActive,
			"message":     fmt.Sprintf("Will permanently delete webhook %d (%s → %s). This cannot be undone. Pass confirmed=true to delete.", hookID, hook.Scope, hook.Destination),
			"api":         fmt.Sprintf("DELETE /v3/hooks/%d", hookID),
		})
	}

	w.cache.ForContext(ctx).Delete(cacheKey)
	if err := w.bc.DeleteWebhook(ctx, hookID); err != nil {
		return toolError("failed to delete webhook %d: %v", hookID, err), nil
	}

	return toolJSON(map[string]any{
		"status":  "deleted",
		"hook_id": hookID,
		"scope":   hook.Scope,
		"api":     fmt.Sprintf("DELETE /v3/hooks/%d", hookID),
	})
}

// redactHeaders returns a copy of headers with every value replaced by
// "(redacted)" so that delivery secrets (bearer tokens, HMAC keys, etc.)
// do not flow into the LLM context on read operations.
func redactHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for k := range headers {
		out[k] = "(redacted)"
	}
	return out
}

// webhookView converts a Webhook to a JSON-serialisable map with header values
// replaced by "(redacted)". Use this for all tool responses instead of embedding
// the raw Webhook struct directly.
func webhookView(h bigcommerce.Webhook) map[string]any {
	v := map[string]any{
		"id":          h.ID,
		"scope":       h.Scope,
		"destination": h.Destination,
		"is_active":   h.IsActive,
	}
	if h.ChannelID != 0 {
		v["channel_id"] = h.ChannelID
	}
	if h.ClientID != "" {
		v["client_id"] = h.ClientID
	}
	if h.StoreHash != "" {
		v["store_hash"] = h.StoreHash
	}
	if h.CreatedAt != 0 {
		v["created_at"] = h.CreatedAt
	}
	if h.UpdatedAt != 0 {
		v["updated_at"] = h.UpdatedAt
	}
	if len(h.Headers) > 0 {
		v["headers"] = redactHeaders(h.Headers)
	}
	return v
}

// webhookViews applies webhookView to a slice of Webhooks.
func webhookViews(hooks []bigcommerce.Webhook) []map[string]any {
	out := make([]map[string]any, len(hooks))
	for i, h := range hooks {
		out[i] = webhookView(h)
	}
	return out
}

// parseHeadersJSON parses a headers_json argument value into map[string]string.
// The argument must be a string containing a JSON object of string→string pairs.
func parseHeadersJSON(v any) (map[string]string, error) {
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("headers_json must be a string containing a JSON object")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("headers_json must not be empty when provided")
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, fmt.Errorf("headers_json: %w", err)
	}
	headers := make(map[string]string, len(raw))
	for k, val := range raw {
		sv, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("headers_json: value for key %q must be a string", k)
		}
		headers[k] = sv
	}
	return headers, nil
}

// argPositiveInt extracts a positive integer from a tool argument value.
func argPositiveInt(field string, v any) (int, error) {
	switch n := v.(type) {
	case float64:
		if n <= 0 || n != float64(int(n)) {
			return 0, fmt.Errorf("%s must be a positive integer", field)
		}
		return int(n), nil
	case int:
		if n <= 0 {
			return 0, fmt.Errorf("%s must be a positive integer", field)
		}
		return n, nil
	case int64:
		if n <= 0 {
			return 0, fmt.Errorf("%s must be a positive integer", field)
		}
		return int(n), nil
	default:
		return 0, fmt.Errorf("%s must be a number", field)
	}
}
