package storefront

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

// Scripts provides MCP tool handlers for the BigCommerce Scripts API
// (/v3/content/scripts), enabling LLM-driven script injection on storefronts.
type Scripts struct {
	bc ScriptAPI
}

// NewScripts constructs a Scripts handler wrapping the given BC client.
func NewScripts(bc ScriptAPI) *Scripts {
	return &Scripts{bc: bc}
}

// RegisterTools wires all script tools into the discovery registry.
func (s *Scripts) RegisterTools(reg *discovery.Registry) {
	// list
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "storefront/scripts/list",
		Tier:    middleware.TierR0,
		Summary: "List storefront scripts managed by this API account",
		Description: "Returns scripts created by the current API account via GET /v3/content/scripts. " +
			"Scripts created by other apps or via the control panel are not visible. " +
			"Optionally filter by channel_id, sort by name or date, and paginate.",
		Tool: mcp.NewTool("storefront_scripts_list",
			mcp.WithDescription(
				"List storefront scripts visible to this API account. "+
					"Each script shows its name, kind (src or script_tag), visibility scope, "+
					"consent_category, location (head/footer), and enabled status.",
			),
			mcp.WithNumber("channel_id",
				mcp.Description("Filter scripts to a specific channel ID."),
			),
			mcp.WithString("sort",
				mcp.Description("Sort field: name, description, date_created, or date_modified."),
			),
			mcp.WithString("direction",
				mcp.Description("Sort direction: asc or desc."),
			),
			mcp.WithNumber("page",
				mcp.Description("Page number for pagination (1-based)."),
			),
			mcp.WithNumber("limit",
				mcp.Description("Results per page (max 250)."),
			),
		),
		Handler: s.handleList,
	})

	// get
	reg.RegisterTool(&discovery.ToolDef{
		Path:        "storefront/scripts/get",
		Tier:        middleware.TierR0,
		Summary:     "Get a single storefront script by UUID",
		Description: "Fetches full details for a specific script via GET /v3/content/scripts/{uuid}.",
		Tool: mcp.NewTool("storefront_scripts_get",
			mcp.WithDescription("Get full details for a storefront script by UUID."),
			mcp.WithString("uuid",
				mcp.Description("Script UUID (e.g. '2bf4b197-1ce3-4c9c-a6e1-3a4e30ae7f99')."),
				mcp.Required(),
			),
		),
		Handler: s.handleGet,
	})

	// create
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "storefront/scripts/create",
		Tier:    middleware.TierR1,
		Summary: "Inject a new script onto the storefront (preview then confirm)",
		Description: "Creates a storefront script via POST /v3/content/scripts. " +
			"Two kinds: 'src' (external URL — supports async/defer and SRI integrity hashes) and " +
			"'script_tag' (inline HTML — can embed Handlebars context variables like {{page_type}}, {{cart_id}}). " +
			"Visibility scopes: storefront (all pages except checkout/order_confirmation), " +
			"all_pages (every page including checkout — requires Modify Checkout Content OAuth scope), " +
			"checkout (checkout only — requires Modify Checkout Content scope + SRI hash for PCI 4.0), " +
			"order_confirmation. " +
			"Scripts render via {{head.scripts}} or {{footer.scripts}} in Stencil themes. " +
			"Returns a preview first; pass confirmed=true to inject.",
		Tool: mcp.NewTool("storefront_scripts_create",
			mcp.WithDescription(
				"Inject a new script onto the storefront. "+
					"kind='src' for an external URL; kind='script_tag' for inline HTML. "+
					"Returns a preview; pass confirmed=true to execute.",
			),
			mcp.WithString("name",
				mcp.Description("Script name (1–255 chars). Appears in the control panel Script Manager."),
				mcp.Required(),
			),
			mcp.WithString("description",
				mcp.Description("Human-readable description shown in Script Manager."),
			),
			mcp.WithString("kind",
				mcp.Description("Script kind: 'src' (external URL, supports async/defer + SRI) or "+
					"'script_tag' (inline HTML, can use Handlebars vars like {{page_type}}, {{cart_id}})."),
			),
			mcp.WithString("src",
				mcp.Description("External script URL. Required when kind=src. Max 65,536 chars. Omit for kind=script_tag."),
			),
			mcp.WithString("html",
				mcp.Description("Inline script HTML. Required when kind=script_tag. Max 65,536 chars. "+
					"May reference Handlebars context vars ({{page_type}}, {{cart_id}}, {{customer_group_id}}, etc.). "+
					"Omit for kind=src."),
			),
			mcp.WithString("load_method",
				mcp.Description("How to load the script: 'default' (blocking), 'async', or 'defer'. "+
					"Only applies to kind=src. Defaults to 'default'."),
			),
			mcp.WithString("location",
				mcp.Description("Injection point: 'head' (renders via {{head.scripts}}) or "+
					"'footer' (renders via {{footer.scripts}}). Defaults to 'footer'."),
			),
			mcp.WithString("visibility",
				mcp.Description("Which pages receive the script: "+
					"'storefront' (all pages except checkout/order_confirmation), "+
					"'all_pages' (every page including checkout — requires Modify Checkout Content OAuth scope), "+
					"'checkout' (checkout only — requires Modify Checkout Content scope + SRI hash for PCI 4.0), "+
					"'order_confirmation' (thank-you page only). Defaults to 'storefront'."),
			),
			mcp.WithString("consent_category",
				mcp.Description("Cookie consent category: 'essential', 'functional', 'analytics', or 'targeting'. "+
					"Consent management platforms use this to gate script execution."),
			),
			mcp.WithBoolean("auto_uninstall",
				mcp.Description("If true, the script is automatically removed when the owning app is uninstalled. "+
					"Strongly recommended: true."),
			),
			mcp.WithBoolean("enabled",
				mcp.Description("Whether the script is active on the storefront. Defaults to true."),
			),
			mcp.WithNumber("channel_id",
				mcp.Description("Scope this script to a specific storefront channel. Omit for the default channel."),
			),
			mcp.WithBoolean("b2be_portal",
				mcp.Description("Set true when this script targets the B2B Edition buyer portal. "+
					"The scaffold will include iframe + hash-route detection (window.B3 check, "+
					"active-frame contentDocument access, hashchange listener) instead of the "+
					"standard MutationObserver pattern."),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Pass true to execute the injection. Omit to receive a preview."),
			),
		),
		Handler: s.handleCreate,
	})

	// update
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "storefront/scripts/update",
		Tier:    middleware.TierR1,
		Summary: "Update a storefront script's settings (preview then confirm)",
		Description: "Updates an existing script via PUT /v3/content/scripts/{uuid}. " +
			"Only the fields you provide are changed — all others remain untouched. " +
			"Returns a preview first; pass confirmed=true to apply.",
		Tool: mcp.NewTool("storefront_scripts_update",
			mcp.WithDescription(
				"Update a storefront script. Only supplied fields are changed. "+
					"Returns a preview; pass confirmed=true to execute.",
			),
			mcp.WithString("uuid",
				mcp.Description("UUID of the script to update."),
				mcp.Required(),
			),
			mcp.WithString("name",
				mcp.Description("New name (1–255 chars)."),
			),
			mcp.WithString("description",
				mcp.Description("New description."),
			),
			mcp.WithString("src",
				mcp.Description("New external script URL (only for kind=src scripts)."),
			),
			mcp.WithString("html",
				mcp.Description("New inline HTML (only for kind=script_tag scripts)."),
			),
			mcp.WithString("load_method",
				mcp.Description("New load method: 'default', 'async', or 'defer'."),
			),
			mcp.WithString("location",
				mcp.Description("New injection location: 'head' or 'footer'."),
			),
			mcp.WithString("visibility",
				mcp.Description("New visibility scope: 'storefront', 'all_pages', 'checkout', or 'order_confirmation'."),
			),
			mcp.WithString("consent_category",
				mcp.Description("New consent category: 'essential', 'functional', 'analytics', or 'targeting'."),
			),
			mcp.WithBoolean("auto_uninstall",
				mcp.Description("Update auto-uninstall behavior."),
			),
			mcp.WithBoolean("enabled",
				mcp.Description("Enable or disable the script."),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Pass true to apply the update. Omit to preview."),
			),
		),
		Handler: s.handleUpdate,
	})

	// delete
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "storefront/scripts/delete",
		Tier:    middleware.TierR3,
		Summary: "Permanently delete a storefront script (requires confirmed=true)",
		Description: "Deletes a script via DELETE /v3/content/scripts/{uuid}. " +
			"Only scripts created by this API account can be deleted. Irreversible. " +
			"Pass confirmed=true to proceed.",
		Tool: mcp.NewTool("storefront_scripts_delete",
			mcp.WithDescription(
				"Permanently delete a storefront script. Cannot be undone. "+
					"Pass confirmed=true to execute.",
			),
			mcp.WithString("uuid",
				mcp.Description("UUID of the script to delete."),
				mcp.Required(),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Must be true to delete. Omitting returns a confirmation prompt."),
			),
		),
		Handler: s.handleDelete,
	})

	// toggle (convenience)
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "storefront/scripts/toggle",
		Tier:    middleware.TierR1,
		Summary: "Enable or disable a storefront script without a full update",
		Description: "Convenience tool that flips a script's enabled state via PUT /v3/content/scripts/{uuid}. " +
			"Equivalent to update with only enabled changed. Returns a preview; pass confirmed=true to apply.",
		Tool: mcp.NewTool("storefront_scripts_toggle",
			mcp.WithDescription(
				"Enable or disable a storefront script. "+
					"Pass confirmed=true to apply the change.",
			),
			mcp.WithString("uuid",
				mcp.Description("UUID of the script to enable or disable."),
				mcp.Required(),
			),
			mcp.WithBoolean("enabled",
				mcp.Description("true to enable the script on the storefront, false to disable it."),
				mcp.Required(),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Pass true to apply the toggle. Omit to preview."),
			),
		),
		Handler: s.handleToggle,
	})
}

// --------------------------------------------------------------------------
// Handlers
// --------------------------------------------------------------------------

func (s *Scripts) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	params := bigcommerce.ScriptListParams{}
	if v, ok := args["channel_id"].(float64); ok && v > 0 {
		params.ChannelID = int(v)
	}
	if v, ok := args["sort"].(string); ok {
		params.Sort = v
	}
	if v, ok := args["direction"].(string); ok {
		params.Direction = v
	}
	if v, ok := args["page"].(float64); ok && v > 0 {
		params.Page = int(v)
	}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params.Limit = int(v)
	}

	scripts, err := s.bc.ListScripts(ctx, params)
	if err != nil {
		return toolError("failed to list scripts: %v", err), nil
	}

	views := make([]map[string]any, len(scripts))
	for i, sc := range scripts {
		views[i] = scriptView(sc)
	}
	return toolJSON(map[string]any{
		"total":   len(scripts),
		"scripts": views,
	})
}

func (s *Scripts) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	uuid, ok := args["uuid"].(string)
	if !ok || strings.TrimSpace(uuid) == "" {
		return toolError("uuid is required"), nil
	}

	script, err := s.bc.GetScript(ctx, uuid)
	if err != nil {
		return toolError("failed to get script %s: %v", uuid, err), nil
	}
	return toolJSON(scriptView(*script))
}

func (s *Scripts) handleCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	name, ok := args["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return toolError("name is required"), nil
	}

	payload := bigcommerce.ScriptCreate{Name: name}
	if v, ok := args["description"].(string); ok {
		payload.Description = v
	}
	if v, ok := args["kind"].(string); ok {
		payload.Kind = v
	}
	if v, ok := args["src"].(string); ok {
		payload.Src = v
	}
	if v, ok := args["html"].(string); ok {
		payload.HTML = v
	}
	if v, ok := args["load_method"].(string); ok {
		payload.LoadMethod = v
	}
	// BC requires load_method on every POST — default to "default" when omitted.
	// For script_tag scripts this has no behavioural effect; for src scripts it
	// controls whether the <script> tag gets async/defer attributes.
	if payload.LoadMethod == "" {
		payload.LoadMethod = bigcommerce.ScriptLoadDefault
	}
	if v, ok := args["location"].(string); ok {
		payload.Location = v
	}
	if v, ok := args["visibility"].(string); ok {
		payload.Visibility = v
	}
	if v, ok := args["consent_category"].(string); ok {
		payload.ConsentCategory = v
	}
	if v, ok := args["auto_uninstall"].(bool); ok {
		payload.AutoUninstall = &v
	}
	if v, ok := args["enabled"].(bool); ok {
		payload.Enabled = &v
	}
	if v, ok := args["channel_id"].(float64); ok && v > 0 {
		payload.ChannelID = int(v)
	}

	if err := validateScriptKind(payload.Kind, payload.Src, payload.HTML); err != nil {
		return toolError("%s", err.Error()), nil
	}

	warnings := collectScopeWarnings(payload.Visibility)

	confirmed := middleware.IsConfirmedFromArgs(args)
	if !confirmed {
		preview := map[string]any{
			"status":  "pending_confirmation",
			"message": "Review the script details below. Pass confirmed=true to inject this script onto the storefront.",
			"script":  payload,
		}
		// Surface a ready-to-fill script skeleton when html has not yet been
		// provided. For B2B Edition portal scripts the scaffold includes iframe
		// + hash-route detection; for checkout/all_pages it uses the standard
		// IIFE+MutationObserver pattern; for storefront it uses DOMContentLoaded.
		if payload.HTML == "" && payload.Kind != bigcommerce.ScriptKindSrc {
			b2bePortal, _ := args["b2be_portal"].(bool)
			if b2bePortal {
				preview["script_scaffold"] = b2bePortalScaffold()
			} else {
				preview["script_scaffold"] = scriptScaffold(payload.Visibility)
			}
		}
		if len(warnings) > 0 {
			preview["warnings"] = warnings
		}
		return toolJSON(preview)
	}

	script, err := s.bc.CreateScript(ctx, payload)
	if err != nil {
		if apiErr, ok := err.(*bigcommerce.APIError); ok {
			return toolError("failed to create script (BC %d): %s", apiErr.StatusCode, string(apiErr.Body)), nil
		}
		return toolError("failed to create script: %v", err), nil
	}

	resp := map[string]any{
		"status": "created",
		"script": scriptView(*script),
	}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	return toolJSON(resp)
}

func (s *Scripts) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	uuid, ok := args["uuid"].(string)
	if !ok || strings.TrimSpace(uuid) == "" {
		return toolError("uuid is required"), nil
	}

	payload := bigcommerce.ScriptUpdate{}
	if v, ok := args["name"].(string); ok {
		payload.Name = &v
	}
	if v, ok := args["description"].(string); ok {
		payload.Description = &v
	}
	if v, ok := args["src"].(string); ok {
		payload.Src = &v
	}
	if v, ok := args["html"].(string); ok {
		payload.HTML = &v
	}
	if v, ok := args["load_method"].(string); ok {
		payload.LoadMethod = &v
	}
	if v, ok := args["location"].(string); ok {
		payload.Location = &v
	}
	if v, ok := args["visibility"].(string); ok {
		payload.Visibility = &v
	}
	if v, ok := args["consent_category"].(string); ok {
		payload.ConsentCategory = &v
	}
	if v, ok := args["auto_uninstall"].(bool); ok {
		payload.AutoUninstall = &v
	}
	if v, ok := args["enabled"].(bool); ok {
		payload.Enabled = &v
	}

	// Guard: reject the call if no updateable field was provided.
	if payload == (bigcommerce.ScriptUpdate{}) {
		return toolError("no fields to update — supply at least one of: name, description, src, html, load_method, location, visibility, consent_category, auto_uninstall, enabled"), nil
	}

	var warnings []string
	if payload.Visibility != nil {
		warnings = collectScopeWarnings(*payload.Visibility)
	}

	confirmed := middleware.IsConfirmedFromArgs(args)
	if !confirmed {
		preview := map[string]any{
			"status":  "pending_confirmation",
			"uuid":    uuid,
			"message": "Review the proposed changes below. Pass confirmed=true to apply.",
			"changes": payload,
		}
		if len(warnings) > 0 {
			preview["warnings"] = warnings
		}
		return toolJSON(preview)
	}

	script, err := s.bc.UpdateScript(ctx, uuid, payload)
	if err != nil {
		return toolError("failed to update script %s: %v", uuid, err), nil
	}

	resp := map[string]any{
		"status": "updated",
		"script": scriptView(*script),
	}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	return toolJSON(resp)
}

func (s *Scripts) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	uuid, ok := args["uuid"].(string)
	if !ok || strings.TrimSpace(uuid) == "" {
		return toolError("uuid is required"), nil
	}

	confirmed := middleware.IsConfirmedFromArgs(args)
	if !confirmed {
		return toolJSON(map[string]any{
			"status":  "pending_confirmation",
			"uuid":    uuid,
			"message": fmt.Sprintf("This will permanently delete script %s. Pass confirmed=true to proceed.", uuid),
		})
	}

	if err := s.bc.DeleteScript(ctx, uuid); err != nil {
		return toolError("failed to delete script %s: %v", uuid, err), nil
	}

	return toolJSON(map[string]any{
		"status":  "deleted",
		"uuid":    uuid,
		"message": fmt.Sprintf("Script %s has been permanently deleted.", uuid),
	})
}

func (s *Scripts) handleToggle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	uuid, ok := args["uuid"].(string)
	if !ok || strings.TrimSpace(uuid) == "" {
		return toolError("uuid is required"), nil
	}

	enabled, ok := args["enabled"].(bool)
	if !ok {
		return toolError("enabled (true/false) is required"), nil
	}

	action := "enable"
	if !enabled {
		action = "disable"
	}

	confirmed := middleware.IsConfirmedFromArgs(args)
	if !confirmed {
		return toolJSON(map[string]any{
			"status":  "pending_confirmation",
			"uuid":    uuid,
			"enabled": enabled,
			"message": fmt.Sprintf("This will %s script %s. Pass confirmed=true to apply.", action, uuid),
		})
	}

	payload := bigcommerce.ScriptUpdate{Enabled: &enabled}
	script, err := s.bc.UpdateScript(ctx, uuid, payload)
	if err != nil {
		return toolError("failed to %s script %s: %v", action, uuid, err), nil
	}

	status := "enabled"
	if !enabled {
		status = "disabled"
	}
	return toolJSON(map[string]any{
		"status": status,
		"script": scriptView(*script),
	})
}

// --------------------------------------------------------------------------
// Validation helpers
// --------------------------------------------------------------------------

// validateScriptKind enforces kind/src/html mutual exclusivity.
func validateScriptKind(kind, src, html string) error {
	if kind == "" {
		return nil // BC defaults to script_tag when kind is omitted
	}
	switch kind {
	case bigcommerce.ScriptKindSrc:
		if html != "" {
			return fmt.Errorf("kind=src scripts must not include html; use the src field instead")
		}
		if src == "" {
			return fmt.Errorf("kind=src requires a src URL")
		}
	case bigcommerce.ScriptKindScriptTag:
		if src != "" {
			return fmt.Errorf("kind=script_tag scripts must not include src; use the html field instead")
		}
		// html may be omitted in preview mode so the scaffold can be returned;
		// the BC API enforces html presence on the actual POST.
	default:
		return fmt.Errorf("kind must be 'src' or 'script_tag', got %q", kind)
	}
	return nil
}

// scriptView returns a token-efficient summary of a Script for list/get
// responses. The full html/src body is omitted; has_html and has_src flags
// indicate whether those fields are populated so the LLM knows what to expect
// before calling update/delete.
func scriptView(s bigcommerce.Script) map[string]any {
	v := map[string]any{
		"uuid":              s.UUID,
		"name":              s.Name,
		"kind":              s.Kind,
		"location":          s.Location,
		"visibility":        s.Visibility,
		"load_method":       s.LoadMethod,
		"consent_category":  s.ConsentCategory,
		"enabled":           s.Enabled,
		"auto_uninstall":    s.AutoUninstall,
		"has_html":          s.HTML != "",
		"has_src":           s.Src != "",
		"channel_id":        s.ChannelID,
		"date_created":      s.DateCreated,
		"date_modified":     s.DateModified,
	}
	if s.Description != "" {
		v["description"] = s.Description
	}
	if s.Src != "" {
		v["src"] = s.Src
	}
	return v
}

// b2bePortalScaffold returns a ready-to-fill script skeleton for targeting the
// B2B Edition buyer portal (b2be_portal=true on create). It encapsulates the
// detection patterns documented in docs/b2be-page-detection.md:
//   - window.B3.setting check (synchronous, all B2BE channel pages)
//   - iframe.active-frame contentDocument access for portal DOM injection
//   - Outer-page hash (default BC-hosted scripts) + iframe hash fallback
//     (custom/self-hosted scripts) for route detection
//   - hashchange listener for SPA navigation + init polling for history.replaceState
func b2bePortalScaffold() string {
	return `(function() {
  // ── B2BE Detection ───────────────────────────────────────────────────────
  // Signal 1: window.B3.setting is set synchronously on all B2BE channel pages.
  if (!(window.B3 && window.B3.setting)) return;

  // Known portal routes — check most-specific first to avoid substring false-matches.
  var ROUTES = [
    ['quoteDraft',     'Quote Draft'],
    ['quoteDetail',    'Quote Detail'],
    ['company-orders', 'Company Orders'],
    ['shoppingLists',  'Shopping Lists'],
    ['quickOrder',     'Quick Order'],
    ['accountSettings','Account Settings'],
    ['dashboard',      'Dashboard'],
    ['invoices',       'Invoices'],
    ['invoice',        'Invoice'],
    ['orders',         'My Orders'],
    ['quotes',         'Quotes'],
    ['addresses',      'Addresses'],
    ['users',          'User Management'],
  ];

  function getPageLabel(hash) {
    for (var i = 0; i < ROUTES.length; i++) {
      if (hash.indexOf(ROUTES[i][0]) !== -1) return ROUTES[i][1];
    }
    return hash.replace('#/', '');
  }

  function isPortalHash(hash) {
    return !!(hash && hash.indexOf('#/') === 0 && hash.length > 2);
  }

  // Returns the portal document (iframe.active-frame) or falls back to outer document.
  function getPortalDoc() {
    var host = window.location.hostname;
    var iframes = document.querySelectorAll('iframe');
    for (var f = 0; f < iframes.length; f++) {
      var fr = iframes[f];
      var isActive = fr.className && fr.className.indexOf('active-frame') !== -1;
      try {
        var iDoc = fr.contentDocument || fr.contentWindow.document;
        var url = (iDoc.location && iDoc.location.href) || '';
        var isSameOrigin = url.indexOf(host) !== -1 && url !== 'about:blank';
        if (isActive || isSameOrigin) return iDoc;
      } catch(e) {}
    }
    return document;
  }

  function detectCurrentPage() {
    var host = window.location.hostname;
    // Check iframe hash first (custom/self-hosted B2BE deployment).
    var iframes = document.querySelectorAll('iframe');
    for (var f = 0; f < iframes.length; f++) {
      try {
        var iDoc = iframes[f].contentDocument || iframes[f].contentWindow.document;
        var url = (iDoc.location && iDoc.location.href) || '';
        var iHash = (iDoc.location && iDoc.location.hash) || '';
        var isSameOrigin = url.indexOf(host) !== -1 && url !== 'about:blank';
        if ((isSameOrigin || (iframes[f].className || '').indexOf('active-frame') !== -1) && isPortalHash(iHash)) {
          return { page: getPageLabel(iHash), hash: iHash, source: 'iframe' };
        }
      } catch(e) {}
    }
    // Fall back to outer page hash (default BC-hosted B2BE deployment).
    var outer = window.location.hash || '';
    if (isPortalHash(outer)) {
      return { page: getPageLabel(outer), hash: outer, source: 'outer-page' };
    }
    return null;
  }

  // ── Page handlers — fill in your logic ───────────────────────────────────
  function onPortalPage(info) {
    console.log('[B2BE] Portal page:', info.page, '| hash:', info.hash);
    var doc = getPortalDoc();
    // TODO: inject custom DOM or run page-specific logic using doc.
    //   e.g. doc.querySelector('h3') — find portal headings
    //   Use MutationObserver on doc.body for React re-render resilience.
  }

  function onNonPortalPage() {
    // B2BE channel, but not currently on a portal page.
  }

  // ── Init: poll for history.replaceState, then switch to hashchange ────────
  var _init = setInterval(function() {
    var info = detectCurrentPage();
    if (info) { clearInterval(_init); onPortalPage(info); }
  }, 500);
  setTimeout(function() { clearInterval(_init); }, 15000);

  window.addEventListener('hashchange', function() {
    var info = detectCurrentPage();
    if (info) onPortalPage(info); else onNonPortalPage();
  });
})();`
}

// scriptScaffold returns a ready-to-fill script skeleton for kind=script_tag
// scripts when the user has not yet supplied html. The scaffold is visibility-aware:
// checkout/all_pages scripts require the IIFE+MutationObserver+debounce pattern
// to survive React re-renders; storefront/order_confirmation scripts use a
// simpler DOMContentLoaded wrapper.
func scriptScaffold(visibility string) string {
	switch visibility {
	case bigcommerce.ScriptVisibilityCheckout, bigcommerce.ScriptVisibilityAllPages:
		return `(function () {
  'use strict';
  var DEBOUNCE_MS = 300;
  var debounceTimer = null;

  function applyCustomization() {
    // YOUR LOGIC HERE — called on init AND after every React re-render.
    //
    // GraphQL token — embed via Handlebars (rendered server-side by BC):
    //   var TOKEN = '{{settings.storefront_api.token}}';
    //   fetch('/graphql', { method: 'POST', credentials: 'same-origin',
    //     headers: { 'Content-Type': 'application/json',
    //                'Authorization': 'Bearer ' + TOKEN },
    //     body: JSON.stringify({ query: QUERY, variables: VARS }) })
    //
    // Sidebar target : document.querySelector('aside.layout-cart') ||
    //                  document.querySelector('[data-test="cart"]') ||
    //                  document.querySelector('.cart-section');
    // Cart REST read : fetch('/api/storefront/carts', { credentials: 'same-origin' })
    // Payment labels : document.querySelectorAll('[data-test="payment-method-name"]')
    // Escape output  : replace &, <, >, ", ' before inserting into innerHTML
  }

  function debouncedApply() {
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(applyCustomization, DEBOUNCE_MS);
  }

  function startObserver() {
    var root = document.getElementById('checkout-app') || document.body;
    new MutationObserver(debouncedApply).observe(root, {
      childList: true, subtree: true,
      attributes: true, attributeFilter: ['class']
    });
    applyCustomization(); // required: run immediately on init
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', startObserver);
  } else {
    startObserver();
  }
})();`

	default: // storefront, order_confirmation, or unset
		return `(function () {
  'use strict';
  document.addEventListener('DOMContentLoaded', function () {
    // YOUR LOGIC HERE
    //
    // GraphQL token — embed via Handlebars (rendered server-side by BC):
    //   var TOKEN = '{{settings.storefront_api.token}}';
    //   fetch('/graphql', { method: 'POST', credentials: 'same-origin',
    //     headers: { 'Content-Type': 'application/json',
    //                'Authorization': 'Bearer ' + TOKEN },
    //     body: JSON.stringify({ query: QUERY, variables: VARS }) })
    //
    // Cart REST read : fetch('/api/storefront/carts', { credentials: 'same-origin' })
    // Escape output  : replace &, <, >, ", ' before inserting into innerHTML
  });
})();`
	}
}

// collectScopeWarnings returns advisory messages for restricted visibility values.
func collectScopeWarnings(visibility string) []string {
	switch visibility {
	case bigcommerce.ScriptVisibilityCheckout:
		return []string{
			"checkout visibility requires the 'Modify Checkout Content' OAuth scope on this API token.",
			"PCI 4.0 compliance: checkout scripts must include at least one SRI integrity hash " +
				"(SHA-256, SHA-384, or SHA-512). The script will fail silently if the hash is missing or does not match.",
		}
	case bigcommerce.ScriptVisibilityAllPages:
		return []string{
			"all_pages visibility includes the checkout page and requires the 'Modify Checkout Content' OAuth scope on this API token.",
		}
	}
	return nil
}
