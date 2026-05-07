package customers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// CustomerSettings provides MCP handlers for GET/PUT /v3/customers/settings
// and /v3/customers/settings/channels/{channel_id}.
type CustomerSettings struct {
	bc BigCommerceCustomersAPI
}

// NewCustomerSettings constructs customer settings tool handlers.
func NewCustomerSettings(bc BigCommerceCustomersAPI) *CustomerSettings {
	return &CustomerSettings{bc: bc}
}

// RegisterTools registers customers/settings/global/* and customers/settings/channel/*.
func (s *CustomerSettings) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/settings/global/get",
		Tier:    middleware.TierR0,
		Summary: "Get global customer settings (V3)",
		Description: "GET /v3/customers/settings — privacy and default customer group configuration " +
			"that channels inherit when they have no channel-specific override.",
		Tool:    mcp.NewTool("customers_settings_global_get", mcp.WithDescription("Fetch store-wide customer settings.")),
		Handler: s.handleGlobalGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/settings/global/update",
		Tier:    middleware.TierR2,
		Summary: "Update global customer settings (V3)",
		Description: "PUT /v3/customers/settings — merges your `settings` object into the current configuration " +
			"before sending (shallow merge for privacy_settings and customer_group_settings). " +
			"Preview first; pass confirmed=true to execute (R2).",
		Tool: mcp.NewTool("customers_settings_global_update",
			mcp.WithDescription("Update global customer settings. Provide settings object; preview then confirmed=true."),
			mcp.WithObject("settings", mcp.Description("Partial or full settings: privacy_settings, customer_group_settings (maps per BigCommerce)."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview.")),
		),
		Handler: s.handleGlobalUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/settings/channel/get",
		Tier:    middleware.TierR0,
		Summary: "Get customer settings for one sales channel (V3)",
		Description: "GET /v3/customers/settings/channels/{channel_id} — includes allow_global_logins " +
			"(shared storefront logins for customers without explicit channel_ids). Null nested values mean inherit from global.",
		Tool: mcp.NewTool("customers_settings_channel_get",
			mcp.WithDescription("Fetch customer settings for a channel."),
			mcp.WithNumber("channel_id", mcp.Description("Sales channel ID."), mcp.Required()),
		),
		Handler: s.handleChannelGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/settings/channel/update",
		Tier:    middleware.TierR2,
		Summary: "Update customer settings for one sales channel (V3)",
		Description: "PUT /v3/customers/settings/channels/{channel_id}. Merges `settings` into the current channel configuration. " +
			"If your payload includes allow_global_logins (shared logins across storefront channels under this store), " +
			"you must also pass confirm_allow_global_logins=true together with confirmed=true after preview — " +
			"this names the cross-channel login behavior explicitly.",
		Tool: mcp.NewTool("customers_settings_channel_update",
			mcp.WithDescription("Update channel customer settings. preview → confirmed=true; allow_global_logins changes need confirm_allow_global_logins=true."),
			mcp.WithNumber("channel_id", mcp.Description("Sales channel ID."), mcp.Required()),
			mcp.WithObject("settings", mcp.Description("Fields to set: privacy_settings, customer_group_settings, allow_global_logins, …"), mcp.Required()),
			mcp.WithBoolean("confirm_allow_global_logins", mcp.Description("Must be true when changing allow_global_logins in settings (explicit cross-channel acknowledgement).")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview.")),
		),
		Handler: s.handleChannelUpdate,
	})
}

func (s *CustomerSettings) handleGlobalGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = request
	data, err := s.bc.GetGlobalCustomerSettings(ctx)
	if err != nil {
		return shared.ToolError("failed to get settings: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"settings": data})
}

func (s *CustomerSettings) handleGlobalUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	patch, err := parseSettingsObject(args, "settings")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	cur, err := s.bc.GetGlobalCustomerSettings(ctx)
	if err != nil {
		return shared.ToolError("failed to read current settings: %v", err), nil
	}
	curMap, err := settingsToMap(cur)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	merged, err := mergeSettingsMaps(curMap, patch)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	var payload bigcommerce.CustomerGlobalSettings
	if err := mapToSettings(merged, &payload); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":         "preview",
			"action":         "update_global_customer_settings",
			"current":        cur,
			"patch":          patch,
			"merged_preview": merged,
			"message":        "Review merged_preview then pass confirmed=true to execute (R2).",
		})
	}

	out, err := s.bc.UpdateGlobalCustomerSettings(ctx, payload)
	if err != nil {
		return shared.ToolError("update failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "settings": out})
}

func (s *CustomerSettings) handleChannelGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	chID, err := shared.ReadPositiveInt(args, "channel_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	data, err := s.bc.GetChannelCustomerSettings(ctx, chID)
	if err != nil {
		return shared.ToolError("failed to get channel settings: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"channel_id": chID, "settings": data})
}

func (s *CustomerSettings) handleChannelUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	chID, err := shared.ReadPositiveInt(args, "channel_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	patch, err := parseSettingsObject(args, "settings")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	cur, err := s.bc.GetChannelCustomerSettings(ctx, chID)
	if err != nil {
		return shared.ToolError("failed to read current channel settings: %v", err), nil
	}
	curMap, err := settingsToMap(cur)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	merged, err := mergeSettingsMaps(curMap, patch)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		msg := "Review merged_preview then pass confirmed=true to execute (R2)."
		if patchTouchesAllowGlobalLogins(patch) {
			msg += " Your patch touches allow_global_logins — when confirming you must also pass confirm_allow_global_logins=true " +
				"to acknowledge shared customer logins across storefront channels."
		}
		return shared.ToolJSON(map[string]any{
			"status":         "preview",
			"action":         "update_channel_customer_settings",
			"channel_id":     chID,
			"current":        cur,
			"patch":          patch,
			"merged_preview": merged,
			"message":        msg,
		})
	}

	if patchTouchesAllowGlobalLogins(patch) && !shared.ReadBool(args, "confirm_allow_global_logins") {
		return shared.ToolError(
			"settings include allow_global_logins — set confirm_allow_global_logins=true together with confirmed=true " +
				"to acknowledge cross-channel shared-login behavior.",
		), nil
	}

	var payload bigcommerce.CustomerChannelSettings
	if err := mapToSettings(merged, &payload); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	out, err := s.bc.UpdateChannelCustomerSettings(ctx, chID, payload)
	if err != nil {
		return shared.ToolError("update failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "channel_id": chID, "settings": out})
}

func parseSettingsObject(args map[string]any, key string) (map[string]any, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil, fmt.Errorf("%s is required", key)
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", key)
	}
	return m, nil
}

func settingsToMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func mergeSettingsMaps(base, patch map[string]any) (map[string]any, error) {
	out := make(map[string]any)
	for k, v := range base {
		out[k] = v
	}
	for k, v := range patch {
		switch k {
		case "privacy_settings", "customer_group_settings":
			pm, ok := v.(map[string]any)
			if !ok {
				out[k] = v
				continue
			}
			cur, _ := out[k].(map[string]any)
			if cur == nil {
				cur = make(map[string]any)
			}
			for pk, pv := range pm {
				cur[pk] = pv
			}
			out[k] = cur
		default:
			out[k] = v
		}
	}
	return out, nil
}

func mapToSettings(m map[string]any, dst any) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func patchTouchesAllowGlobalLogins(patch map[string]any) bool {
	_, ok := patch["allow_global_logins"]
	return ok
}

func maskEmailForPreview(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return "(redacted)"
	}
	return "***@" + email[at+1:]
}
