package customers

import (
	"context"
	"fmt"
	"math"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// CustomerGroupSearchFilters maps tool parameters to BigCommerce
// /v2/customer_groups query keys. The V2 endpoint does not document a `sort`
// param, so all entries here are data filters and any present key counts as
// "the agent supplied a real filter".
var CustomerGroupSearchFilters = []shared.SearchFilter{
	{ToolKey: "name", BCKey: "name", Kind: "string"},
	{ToolKey: "name_like", BCKey: "name:like", Kind: "string"},
	{ToolKey: "is_default", BCKey: "is_default", Kind: "bool"},
	{ToolKey: "is_group_for_guests", BCKey: "is_group_for_guests", Kind: "bool"},
	{ToolKey: "date_created", BCKey: "date_created", Kind: "string"},
	{ToolKey: "date_created_min", BCKey: "date_created:min", Kind: "string"},
	{ToolKey: "date_created_max", BCKey: "date_created:max", Kind: "string"},
	{ToolKey: "date_modified", BCKey: "date_modified", Kind: "string"},
	{ToolKey: "date_modified_min", BCKey: "date_modified:min", Kind: "string"},
	{ToolKey: "date_modified_max", BCKey: "date_modified:max", Kind: "string"},
}

// Groups provides MCP tool handlers for /v2/customer_groups CRUD.
type Groups struct {
	bc BigCommerceCustomersAPI
}

// NewGroups constructs the Customer Group tool handlers.
func NewGroups(bc BigCommerceCustomersAPI) *Groups {
	return &Groups{bc: bc}
}

// RegisterTools registers all Customer Group tools (list, get, count, create,
// update, delete) into the discovery registry.
func (g *Groups) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/groups/list",
		Tier:    middleware.TierR0,
		Summary: "List or search customer groups",
		Description: "Fetches customer groups from GET /v2/customer_groups. Provide at least one " +
			"filter (name, name_like, is_default, is_group_for_guests, date_created*, date_modified*) " +
			"or set list_all=true to return every group.",
		Tool: mcp.NewTool("customers_groups_list",
			mcp.WithDescription(
				"List or search customer groups. Pass list_all=true for every group, "+
					"or filter by name, name_like, is_default, is_group_for_guests, "+
					"or date_created/date_modified (and *_min / *_max variants).",
			),
			mcp.WithBoolean("list_all",
				mcp.Description("Set to true to return all customer groups in the store."),
			),
			mcp.WithString("name",
				mcp.Description("Exact group name match."),
			),
			mcp.WithString("name_like",
				mcp.Description("Partial name match (SQL LIKE)."),
			),
			mcp.WithBoolean("is_default",
				mcp.Description("Filter by whether the group is the default group for new customers."),
			),
			mcp.WithBoolean("is_group_for_guests",
				mcp.Description("Filter by the guests group flag (only one group can hold this)."),
			),
			mcp.WithString("date_created",
				mcp.Description("Filter by exact date_created (e.g. 2024-09-05T13:43:54)."),
			),
			mcp.WithString("date_created_min",
				mcp.Description("Filter by minimum date_created (date_created:min)."),
			),
			mcp.WithString("date_created_max",
				mcp.Description("Filter by maximum date_created (date_created:max)."),
			),
			mcp.WithString("date_modified",
				mcp.Description("Filter by exact date_modified."),
			),
			mcp.WithString("date_modified_min",
				mcp.Description("Filter by minimum date_modified (date_modified:min)."),
			),
			mcp.WithString("date_modified_max",
				mcp.Description("Filter by maximum date_modified (date_modified:max)."),
			),
		),
		Handler: g.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "customers/groups/get",
		Tier:        middleware.TierR0,
		Summary:     "Get a single customer group by ID",
		Description: "Fetches one group via GET /v2/customer_groups/{id}, including category_access and discount_rules.",
		Tool: mcp.NewTool("customers_groups_get",
			mcp.WithDescription("Get customer group details by numeric group_id."),
			mcp.WithNumber("group_id", mcp.Description("Customer group ID."), mcp.Required()),
		),
		Handler: g.handleGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "customers/groups/count",
		Tier:        middleware.TierR0,
		Summary:     "Get the total count of customer groups",
		Description: "Returns the total number of customer groups via GET /v2/customer_groups/count.",
		Tool: mcp.NewTool("customers_groups_count",
			mcp.WithDescription("Return the total count of customer groups in the store."),
		),
		Handler: g.handleCount,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/groups/create",
		Tier:    middleware.TierR1,
		Summary: "Create a new customer group",
		Description: "POST /v2/customer_groups. Required: name. Optional: is_default, is_group_for_guests, " +
			"category_access (type + categories), discount_rules. " +
			"Note: discount rules using type='price_list' are exclusive — when mixed with other " +
			"rule types, only the price_list rule is kept and a warning is emitted. " +
			"Preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("customers_groups_create",
			mcp.WithDescription(
				"Create a customer group. Required: name. "+
					"Preview first; pass confirmed=true to execute.",
			),
			mcp.WithString("name", mcp.Description("Customer group name (required)."), mcp.Required()),
			mcp.WithBoolean("is_default",
				mcp.Description("Auto-assign new customers to this group."),
			),
			mcp.WithBoolean("is_group_for_guests",
				mcp.Description("Mark this as the guests group (only one allowed at a time)."),
			),
			mcp.WithString("category_access_type",
				mcp.Description("Category access mode: 'all', 'specific', or 'none'."),
			),
			mcp.WithArray("category_access_categories",
				mcp.Description("Category IDs accessible to this group; only used when category_access_type='specific'."),
				mcp.Items(map[string]any{"type": "number"}),
			),
			mcp.WithArray("discount_rules",
				mcp.Description(
					"Discount rules array. Each rule is an object: "+
						"{type: 'price_list'|'all'|'category'|'product', method?: 'percent'|'fixed'|'price', "+
						"amount?: '5.0000', price_list_id?, category_id?, product_id?}. "+
						"price_list rules are exclusive — they cannot be combined with other rule types.",
				),
				mcp.Items(map[string]any{"type": "object"}),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set to true to execute after reviewing the preview."),
			),
		),
		Handler: g.handleCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/groups/update",
		Tier:    middleware.TierR1,
		Summary: "Update an existing customer group",
		Description: "PUT /v2/customer_groups/{id}. Only supplied fields are changed, with one " +
			"important exception: BigCommerce treats discount_rules in bulk — sending the field " +
			"overwrites the entire set. Omit discount_rules to leave existing rules untouched. " +
			"Preview first; pass confirmed=true to apply.",
		Tool: mcp.NewTool("customers_groups_update",
			mcp.WithDescription(
				"Update a customer group by group_id. Only supplied fields are changed. "+
					"Note: discount_rules are overwritten in bulk by BigCommerce — omit the field "+
					"to preserve existing rules. Preview first; pass confirmed=true to apply.",
			),
			mcp.WithNumber("group_id", mcp.Description("Customer group ID to update."), mcp.Required()),
			mcp.WithString("name", mcp.Description("New group name.")),
			mcp.WithBoolean("is_default",
				mcp.Description("Whether new customers are auto-assigned to this group."),
			),
			mcp.WithBoolean("is_group_for_guests",
				mcp.Description("Whether this is the guests group."),
			),
			mcp.WithString("category_access_type",
				mcp.Description("New category access mode: 'all', 'specific', or 'none'."),
			),
			mcp.WithArray("category_access_categories",
				mcp.Description("Category IDs accessible to this group; only used when category_access_type='specific'."),
				mcp.Items(map[string]any{"type": "number"}),
			),
			mcp.WithArray("discount_rules",
				mcp.Description(
					"Replacement discount rules array. Sending this field OVERWRITES all existing "+
						"rules on the group. Omit to keep current rules. Same shape as create. "+
						"price_list rules are exclusive — they cannot be combined with other rule types.",
				),
				mcp.Items(map[string]any{"type": "object"}),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set to true to execute after reviewing the preview."),
			),
		),
		Handler: g.handleUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/groups/delete",
		Tier:    middleware.TierR3,
		Summary: "Delete a customer group (irreversible)",
		Description: "DELETE /v2/customer_groups/{id}. This is irreversible. All customers " +
			"currently in the group will be unassigned automatically by BigCommerce. " +
			"Preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("customers_groups_delete",
			mcp.WithDescription(
				"Delete a customer group by group_id. Irreversible: all members are unassigned "+
					"automatically. Preview first; pass confirmed=true to execute.",
			),
			mcp.WithNumber("group_id", mcp.Description("Customer group ID to delete."), mcp.Required()),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set to true to execute the destructive delete after reviewing the preview."),
			),
		),
		Handler: g.handleDelete,
	})
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (g *Groups) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	listAll := shared.ReadBool(args, "list_all")

	params, err := shared.ExtractFilters(args, CustomerGroupSearchFilters)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !listAll && len(params) == 0 {
		return shared.ToolError(
			"provide at least one filter (name, name_like, is_default, is_group_for_guests, " +
				"date_created*, date_modified*) or set list_all=true to return every group.",
		), nil
	}

	groups, err := g.bc.ListCustomerGroups(ctx, params)
	if err != nil {
		return shared.ToolError("failed to list customer groups: %v", err), nil
	}

	type groupSummary struct {
		ID               int    `json:"id"`
		Name             string `json:"name"`
		IsDefault        bool   `json:"is_default"`
		IsGroupForGuests bool   `json:"is_group_for_guests"`
		CategoryAccess   string `json:"category_access,omitempty"`
		DiscountRules    int    `json:"discount_rules"`
		DateModified     string `json:"date_modified,omitempty"`
	}
	summaries := make([]groupSummary, len(groups))
	for i, gr := range groups {
		s := groupSummary{
			ID:               gr.ID,
			Name:             gr.Name,
			IsDefault:        gr.IsDefault,
			IsGroupForGuests: gr.IsGroupForGuests,
			DiscountRules:    len(gr.DiscountRules),
			DateModified:     gr.DateModified,
		}
		if gr.CategoryAccess != nil {
			s.CategoryAccess = gr.CategoryAccess.Type
		}
		summaries[i] = s
	}

	return shared.ToolJSON(map[string]any{
		"total":  len(groups),
		"groups": summaries,
	})
}

func (g *Groups) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := shared.ReadPositiveInt(request.GetArguments(), "group_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	group, err := g.bc.GetCustomerGroup(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get customer group %d: %v", id, err), nil
	}
	return shared.ToolJSON(group)
}

func (g *Groups) handleCount(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	count, err := g.bc.CountCustomerGroups(ctx)
	if err != nil {
		return shared.ToolError("failed to count customer groups: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"count": count})
}

func (g *Groups) handleCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params, err := ParseGroupCreateParams(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if params.Confirmed {
		created, err := g.bc.CreateCustomerGroup(ctx, params.Payload)
		if err != nil {
			return shared.ToolError("failed to create customer group: %v", err), nil
		}
		out := map[string]any{
			"status":  "created",
			"message": fmt.Sprintf("Customer group %q created successfully with ID %d.", created.Name, created.ID),
			"group": map[string]any{
				"id":   created.ID,
				"name": created.Name,
			},
		}
		if len(params.Warnings) > 0 {
			out["warnings"] = params.Warnings
		}
		return shared.ToolJSON(out)
	}

	return previewGroupCreate(params)
}

func (g *Groups) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params, err := ParseGroupUpdateParams(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if !params.HasFields() {
		return shared.ToolError(
			"provide at least one field to update (name, is_default, is_group_for_guests, " +
				"category_access_type, category_access_categories, discount_rules)",
		), nil
	}

	if params.Confirmed {
		updated, err := g.bc.UpdateCustomerGroup(ctx, params.GroupID, params.Update)
		if err != nil {
			return shared.ToolError("failed to update customer group %d: %v", params.GroupID, err), nil
		}
		out := map[string]any{
			"status":  "updated",
			"message": fmt.Sprintf("Customer group %d updated successfully.", params.GroupID),
			"group": map[string]any{
				"id":   updated.ID,
				"name": updated.Name,
			},
		}
		if len(params.Warnings) > 0 {
			out["warnings"] = params.Warnings
		}
		return shared.ToolJSON(out)
	}

	return previewGroupUpdate(params)
}

func (g *Groups) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "group_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	confirmed := shared.ReadBool(args, "confirmed")

	if !confirmed {
		// Preview must surface that this is destructive and members will be unassigned.
		group, err := g.bc.GetCustomerGroup(ctx, id)
		if err != nil {
			// We don't block the preview if the read fails — surface the error
			// so the agent can decide whether to retry or proceed without context.
			return shared.ToolJSON(map[string]any{
				"status":   "preview",
				"action":   "delete",
				"group_id": id,
				"warning":  "Could not pre-fetch group details — verify the ID is correct before confirming.",
				"detail":   err.Error(),
				"message":  fmt.Sprintf("Pass confirmed=true to delete customer group %d. This is irreversible.", id),
			})
		}
		return shared.ToolJSON(map[string]any{
			"status":   "preview",
			"action":   "delete",
			"group_id": id,
			"group": map[string]any{
				"id":                  group.ID,
				"name":                group.Name,
				"is_default":          group.IsDefault,
				"is_group_for_guests": group.IsGroupForGuests,
				"discount_rules":      len(group.DiscountRules),
			},
			"warning": "Deleting this group is irreversible. All currently-assigned customers will be unassigned automatically by BigCommerce.",
			"message": fmt.Sprintf("Pass confirmed=true to delete customer group %d (%q).", id, group.Name),
		})
	}

	if err := g.bc.DeleteCustomerGroup(ctx, id); err != nil {
		return shared.ToolError("failed to delete customer group %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":   "deleted",
		"group_id": id,
		"message":  fmt.Sprintf("Customer group %d deleted. All members have been unassigned.", id),
	})
}

// ---------------------------------------------------------------------------
// Param parsing
// ---------------------------------------------------------------------------

// GroupCreateParams holds parsed customers_groups_create arguments.
type GroupCreateParams struct {
	Payload   bigcommerce.CustomerGroupCreate
	Confirmed bool
	Warnings  []string
}

// GroupUpdateParams holds parsed customers_groups_update arguments.
type GroupUpdateParams struct {
	GroupID   int
	Update    bigcommerce.CustomerGroupUpdate
	Confirmed bool
	Warnings  []string

	// RulesProvided distinguishes "discount_rules omitted" from "explicit empty list".
	// Pre-pruning value, captured at parse time.
	RulesProvided bool
}

// HasFields reports whether the parsed params contain at least one field to
// update. Exported so tool tests can verify the omit-vs-clear distinction
// for discount_rules without going through the full handler path.
func (p *GroupUpdateParams) HasFields() bool {
	u := p.Update
	return u.Name != nil ||
		u.IsDefault != nil ||
		u.IsGroupForGuests != nil ||
		u.CategoryAccess != nil ||
		p.RulesProvided
}

// ParseGroupCreateParams parses the customers_groups_create tool arguments.
func ParseGroupCreateParams(args map[string]any) (*GroupCreateParams, error) {
	p := &GroupCreateParams{}

	nameRaw, ok := args["name"]
	if !ok {
		return nil, fmt.Errorf("name is required")
	}
	name, ok := nameRaw.(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("name must be a non-empty string")
	}
	p.Payload.Name = name

	if v, ok := args["is_default"]; ok {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("is_default must be a boolean")
		}
		p.Payload.IsDefault = &b
	}
	if v, ok := args["is_group_for_guests"]; ok {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("is_group_for_guests must be a boolean")
		}
		p.Payload.IsGroupForGuests = &b
	}

	access, err := parseCategoryAccess(args)
	if err != nil {
		return nil, err
	}
	if access != nil {
		p.Payload.CategoryAccess = access
	}

	if rawRules, ok := args["discount_rules"]; ok {
		rules, warns, err := parseDiscountRules(rawRules)
		if err != nil {
			return nil, err
		}
		p.Payload.DiscountRules = rules
		p.Warnings = append(p.Warnings, warns...)
	}

	p.Confirmed = middleware.IsConfirmedFromArgs(args)
	return p, nil
}

// ParseGroupUpdateParams parses the customers_groups_update tool arguments.
func ParseGroupUpdateParams(args map[string]any) (*GroupUpdateParams, error) {
	p := &GroupUpdateParams{}

	id, err := shared.ReadPositiveInt(args, "group_id")
	if err != nil {
		return nil, err
	}
	p.GroupID = id

	if v, ok := args["name"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("name must be a string")
		}
		p.Update.Name = &s
	}
	if v, ok := args["is_default"]; ok {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("is_default must be a boolean")
		}
		p.Update.IsDefault = &b
	}
	if v, ok := args["is_group_for_guests"]; ok {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("is_group_for_guests must be a boolean")
		}
		p.Update.IsGroupForGuests = &b
	}

	access, err := parseCategoryAccess(args)
	if err != nil {
		return nil, err
	}
	if access != nil {
		p.Update.CategoryAccess = access
	}

	if rawRules, ok := args["discount_rules"]; ok {
		rules, warns, err := parseDiscountRules(rawRules)
		if err != nil {
			return nil, err
		}
		// Empty slice (explicitly cleared) and nil (kept untouched) must be
		// distinguishable. Use a non-nil empty slice to send "[]" on the wire.
		if rules == nil {
			rules = []bigcommerce.CustomerGroupDiscountRule{}
		}
		p.Update.DiscountRules = rules
		p.Warnings = append(p.Warnings, warns...)
		p.RulesProvided = true
	}

	p.Confirmed = middleware.IsConfirmedFromArgs(args)
	return p, nil
}

// parseCategoryAccess reads category_access_type and category_access_categories
// into a *CategoryAccess. Returns (nil, nil) when neither key is provided so
// the caller can leave the field untouched on update payloads.
func parseCategoryAccess(args map[string]any) (*bigcommerce.CategoryAccess, error) {
	typeRaw, hasType := args["category_access_type"]
	catsRaw, hasCats := args["category_access_categories"]
	if !hasType && !hasCats {
		return nil, nil
	}

	out := &bigcommerce.CategoryAccess{}
	if hasType {
		s, ok := typeRaw.(string)
		if !ok {
			return nil, fmt.Errorf("category_access_type must be a string")
		}
		switch s {
		case bigcommerce.CategoryAccessAll, bigcommerce.CategoryAccessSpecific, bigcommerce.CategoryAccessNone:
			out.Type = s
		case "":
			return nil, fmt.Errorf("category_access_type must be one of: all, specific, none")
		default:
			return nil, fmt.Errorf("category_access_type must be one of: all, specific, none (got %q)", s)
		}
	}

	if hasCats {
		raw, ok := catsRaw.([]any)
		if !ok {
			return nil, fmt.Errorf("category_access_categories must be an array of category IDs")
		}
		for _, item := range raw {
			f, ok := item.(float64)
			if !ok {
				return nil, fmt.Errorf("each category_access_categories entry must be a number")
			}
			if f != math.Trunc(f) {
				return nil, fmt.Errorf("each category_access_categories entry must be an integer")
			}
			id := int(f)
			if id <= 0 {
				continue
			}
			out.Categories = append(out.Categories, id)
		}
	}

	if out.Type == bigcommerce.CategoryAccessSpecific && len(out.Categories) == 0 {
		return nil, fmt.Errorf("category_access_type='specific' requires a non-empty category_access_categories array")
	}
	if out.Type != bigcommerce.CategoryAccessSpecific && len(out.Categories) > 0 {
		// BigCommerce ignores categories when type != specific; warn-and-drop is
		// safer than passing junk through, but since we have no warnings channel
		// here, return an explicit error so the agent corrects it.
		return nil, fmt.Errorf("category_access_categories may only be set when category_access_type='specific'")
	}
	return out, nil
}

// parseDiscountRules reads the discount_rules tool argument into a slice of
// CustomerGroupDiscountRule, applying validation and the project's
// silent-prune policy: when a price_list rule is mixed with other rule
// types, the price_list rule is kept and the others are dropped (with a
// warning surfaced through the second return value).
func parseDiscountRules(raw any) ([]bigcommerce.CustomerGroupDiscountRule, []string, error) {
	if raw == nil {
		return nil, nil, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, nil, fmt.Errorf("discount_rules must be an array of rule objects")
	}

	rules := make([]bigcommerce.CustomerGroupDiscountRule, 0, len(arr))
	allCount := 0
	priceListIdx := -1

	for i, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("discount_rules[%d] must be an object", i)
		}
		rule, err := parseDiscountRule(obj, i)
		if err != nil {
			return nil, nil, err
		}
		if rule.Type == bigcommerce.DiscountRuleTypePriceList && priceListIdx == -1 {
			priceListIdx = len(rules)
		}
		if rule.Type == bigcommerce.DiscountRuleTypeAll {
			allCount++
			if allCount > 1 {
				return nil, nil, fmt.Errorf("discount_rules may include at most one rule with type='all'")
			}
		}
		rules = append(rules, rule)
	}

	// Silent-prune-and-warn: if any price_list rule is present, drop everything
	// else. Per BC, price_list is exclusive and mixing produces a 4xx anyway.
	var warnings []string
	if priceListIdx >= 0 && len(rules) > 1 {
		kept := rules[priceListIdx]
		dropped := len(rules) - 1
		rules = []bigcommerce.CustomerGroupDiscountRule{kept}
		warnings = append(warnings,
			fmt.Sprintf(
				"discount_rules: type='price_list' is mutually exclusive with other rule types — kept the price_list rule (price_list_id=%d) and dropped %d other rule(s).",
				kept.PriceListID, dropped,
			),
		)
	}

	return rules, warnings, nil
}

// parseDiscountRule validates a single discount-rule object and returns its
// strongly-typed form. The position index is included in error messages so
// the agent can find the offending rule.
func parseDiscountRule(obj map[string]any, idx int) (bigcommerce.CustomerGroupDiscountRule, error) {
	var rule bigcommerce.CustomerGroupDiscountRule

	typeRaw, ok := obj["type"]
	if !ok {
		return rule, fmt.Errorf("discount_rules[%d].type is required", idx)
	}
	t, ok := typeRaw.(string)
	if !ok {
		return rule, fmt.Errorf("discount_rules[%d].type must be a string", idx)
	}
	switch t {
	case bigcommerce.DiscountRuleTypePriceList,
		bigcommerce.DiscountRuleTypeAll,
		bigcommerce.DiscountRuleTypeCategory,
		bigcommerce.DiscountRuleTypeProduct:
		rule.Type = t
	default:
		return rule, fmt.Errorf(
			"discount_rules[%d].type must be one of: price_list, all, category, product (got %q)",
			idx, t,
		)
	}

	// price_list rules use price_list_id only — method/amount are not sent.
	if rule.Type == bigcommerce.DiscountRuleTypePriceList {
		idRaw, ok := obj["price_list_id"]
		if !ok {
			return rule, fmt.Errorf("discount_rules[%d]: price_list_id is required for type='price_list'", idx)
		}
		f, ok := idRaw.(float64)
		if !ok {
			return rule, fmt.Errorf("discount_rules[%d].price_list_id must be a number", idx)
		}
		if f != math.Trunc(f) {
			return rule, fmt.Errorf("discount_rules[%d].price_list_id must be an integer", idx)
		}
		if int(f) <= 0 {
			return rule, fmt.Errorf("discount_rules[%d].price_list_id must be positive", idx)
		}
		rule.PriceListID = int(f)
		return rule, nil
	}

	// Non-price_list rules require method + amount.
	methodRaw, ok := obj["method"]
	if !ok {
		return rule, fmt.Errorf("discount_rules[%d]: method is required for type='%s'", idx, rule.Type)
	}
	m, ok := methodRaw.(string)
	if !ok {
		return rule, fmt.Errorf("discount_rules[%d].method must be a string", idx)
	}
	switch m {
	case bigcommerce.DiscountRuleMethodPercent,
		bigcommerce.DiscountRuleMethodFixed,
		bigcommerce.DiscountRuleMethodPrice:
		rule.Method = m
	default:
		return rule, fmt.Errorf(
			"discount_rules[%d].method must be one of: percent, fixed, price (got %q)",
			idx, m,
		)
	}

	amtRaw, ok := obj["amount"]
	if !ok {
		return rule, fmt.Errorf("discount_rules[%d]: amount is required for type='%s'", idx, rule.Type)
	}
	switch a := amtRaw.(type) {
	case string:
		if a == "" {
			return rule, fmt.Errorf("discount_rules[%d].amount must be a non-empty string", idx)
		}
		rule.Amount = a
	case float64:
		// Accept numeric input as a convenience and normalize to BC's string shape.
		rule.Amount = fmt.Sprintf("%.4f", a)
	default:
		return rule, fmt.Errorf("discount_rules[%d].amount must be a string or number", idx)
	}

	switch rule.Type {
	case bigcommerce.DiscountRuleTypeCategory:
		idRaw, ok := obj["category_id"]
		if !ok {
			return rule, fmt.Errorf("discount_rules[%d]: category_id is required for type='category'", idx)
		}
		f, ok := idRaw.(float64)
		if !ok {
			return rule, fmt.Errorf("discount_rules[%d].category_id must be a number", idx)
		}
		if f != math.Trunc(f) || int(f) <= 0 {
			return rule, fmt.Errorf("discount_rules[%d].category_id must be a positive integer", idx)
		}
		rule.CategoryID = int(f)
	case bigcommerce.DiscountRuleTypeProduct:
		idRaw, ok := obj["product_id"]
		if !ok {
			return rule, fmt.Errorf("discount_rules[%d]: product_id is required for type='product'", idx)
		}
		f, ok := idRaw.(float64)
		if !ok {
			return rule, fmt.Errorf("discount_rules[%d].product_id must be a number", idx)
		}
		if f != math.Trunc(f) || int(f) <= 0 {
			return rule, fmt.Errorf("discount_rules[%d].product_id must be a positive integer", idx)
		}
		rule.ProductID = int(f)
	}

	return rule, nil
}

// ---------------------------------------------------------------------------
// Preview helpers
// ---------------------------------------------------------------------------

func previewGroupCreate(params *GroupCreateParams) (*mcp.CallToolResult, error) {
	out := map[string]any{
		"status":  "preview",
		"action":  "create",
		"message": "Review the customer group below. Pass confirmed=true with the same parameters to create it.",
		"group":   summarizeCreatePayload(params.Payload),
	}
	if len(params.Warnings) > 0 {
		out["warnings"] = params.Warnings
	}
	return shared.ToolJSON(out)
}

func previewGroupUpdate(params *GroupUpdateParams) (*mcp.CallToolResult, error) {
	out := map[string]any{
		"status":   "preview",
		"action":   "update",
		"group_id": params.GroupID,
		"message":  fmt.Sprintf("Review changes for customer group %d. Pass confirmed=true to apply.", params.GroupID),
		"changes":  summarizeUpdatePayload(params.Update, params.RulesProvided),
	}
	if params.RulesProvided {
		out["note_discount_rules"] = "BigCommerce overwrites the entire discount_rules set on update — the rules below will replace any existing rules."
	}
	if len(params.Warnings) > 0 {
		out["warnings"] = params.Warnings
	}
	return shared.ToolJSON(out)
}

func summarizeCreatePayload(p bigcommerce.CustomerGroupCreate) map[string]any {
	m := map[string]any{"name": p.Name}
	if p.IsDefault != nil {
		m["is_default"] = *p.IsDefault
	}
	if p.IsGroupForGuests != nil {
		m["is_group_for_guests"] = *p.IsGroupForGuests
	}
	if p.CategoryAccess != nil {
		m["category_access"] = p.CategoryAccess
	}
	if len(p.DiscountRules) > 0 {
		m["discount_rules"] = p.DiscountRules
	}
	return m
}

func summarizeUpdatePayload(u bigcommerce.CustomerGroupUpdate, rulesProvided bool) map[string]any {
	m := map[string]any{}
	if u.Name != nil {
		m["name"] = *u.Name
	}
	if u.IsDefault != nil {
		m["is_default"] = *u.IsDefault
	}
	if u.IsGroupForGuests != nil {
		m["is_group_for_guests"] = *u.IsGroupForGuests
	}
	if u.CategoryAccess != nil {
		m["category_access"] = u.CategoryAccess
	}
	if rulesProvided {
		// Always include discount_rules in the preview when the agent supplied
		// it — even if empty — so the preview accurately reflects the wire.
		if u.DiscountRules == nil {
			m["discount_rules"] = []bigcommerce.CustomerGroupDiscountRule{}
		} else {
			m["discount_rules"] = u.DiscountRules
		}
	}
	return m
}
