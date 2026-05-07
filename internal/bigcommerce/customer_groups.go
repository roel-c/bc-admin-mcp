package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// customerGroupsListMaxPages bounds the offset-paginated list endpoint so a
// runaway response cannot loop forever. Real stores rarely have more than a
// few dozen groups; this is a conservative ceiling.
const customerGroupsListMaxPages = 50

// ListCustomerGroups fetches all customer groups via GET /v2/customer_groups.
// V2 list responses are bare JSON arrays (no pagination envelope), so we walk
// pages manually using ?page=&limit= until BigCommerce returns an empty body
// or fewer rows than the requested limit.
//
// Filterable query keys passed through as-is: name, name:like, is_default,
// is_group_for_guests, date_created[:min|:max], date_modified[:min|:max].
func (c *Client) ListCustomerGroups(ctx context.Context, params map[string]string) ([]CustomerGroup, error) {
	limit := c.cfg.DefaultPageLimit
	if limit <= 0 || limit > 250 {
		limit = 250
	}

	var all []CustomerGroup
	for page := 1; page <= customerGroupsListMaxPages; page++ {
		vals := url.Values{}
		for k, v := range params {
			if v != "" {
				vals.Set(k, v)
			}
		}
		vals.Set("page", strconv.Itoa(page))
		vals.Set("limit", strconv.Itoa(limit))

		path := "customer_groups?" + vals.Encode()
		body, err := c.GetV2(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("list customer groups (page %d): %w", page, err)
		}

		// BC returns a 204 No Content (empty body) when no rows match the filter
		// on the first page, or when paging beyond the last result.
		if len(body) == 0 {
			break
		}

		var rows []CustomerGroup
		if err := json.Unmarshal(body, &rows); err != nil {
			return nil, fmt.Errorf("parse customer groups page %d: %w", page, err)
		}
		if len(rows) == 0 {
			break
		}
		all = append(all, rows...)

		if c.cfg.MaxTotalRecords > 0 && len(all) >= c.cfg.MaxTotalRecords {
			c.logger.Warn("customer groups pagination ceiling reached — truncating",
				"fetched", len(all),
				"limit", c.cfg.MaxTotalRecords,
			)
			all = all[:c.cfg.MaxTotalRecords]
			break
		}
		if len(rows) < limit {
			break
		}
	}

	return all, nil
}

// GetCustomerGroup fetches a single group via GET /v2/customer_groups/{id}.
func (c *Client) GetCustomerGroup(ctx context.Context, id int) (*CustomerGroup, error) {
	if id <= 0 {
		return nil, fmt.Errorf("customer group id must be positive")
	}
	body, err := c.GetV2(ctx, fmt.Sprintf("customer_groups/%d", id))
	if err != nil {
		return nil, fmt.Errorf("get customer group %d: %w", id, err)
	}
	var group CustomerGroup
	if err := json.Unmarshal(body, &group); err != nil {
		return nil, fmt.Errorf("parse customer group %d: %w", id, err)
	}
	return &group, nil
}

// CountCustomerGroups returns the total number of groups in the store via
// GET /v2/customer_groups/count.
func (c *Client) CountCustomerGroups(ctx context.Context) (int, error) {
	body, err := c.GetV2(ctx, "customer_groups/count")
	if err != nil {
		return 0, fmt.Errorf("count customer groups: %w", err)
	}
	var resp struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("parse customer groups count: %w", err)
	}
	return resp.Count, nil
}

// CreateCustomerGroup creates a group via POST /v2/customer_groups.
// Required field: payload.Name. The BC API may return a 207 multi-status when
// the group is created but a sitewide discount update partially failed; in
// that case the response body may not match customerGroup_Full and we
// surface a descriptive error.
func (c *Client) CreateCustomerGroup(ctx context.Context, payload CustomerGroupCreate) (*CustomerGroup, error) {
	if payload.Name == "" {
		return nil, fmt.Errorf("customer group name is required")
	}
	body, err := c.PostV2(ctx, "customer_groups", payload)
	if err != nil {
		return nil, fmt.Errorf("create customer group: %w", err)
	}
	var group CustomerGroup
	if err := json.Unmarshal(body, &group); err != nil {
		return nil, fmt.Errorf("parse created customer group: %w (raw: %s)", err, truncateBody(body))
	}
	return &group, nil
}

// UpdateCustomerGroup updates a group via PUT /v2/customer_groups/{id}.
// Discount rules are treated in bulk by BigCommerce: sending the field
// overwrites the entire set. Leave payload.DiscountRules nil to leave
// existing rules untouched.
func (c *Client) UpdateCustomerGroup(ctx context.Context, id int, payload CustomerGroupUpdate) (*CustomerGroup, error) {
	if id <= 0 {
		return nil, fmt.Errorf("customer group id must be positive")
	}
	body, err := c.PutV2(ctx, fmt.Sprintf("customer_groups/%d", id), payload)
	if err != nil {
		return nil, fmt.Errorf("update customer group %d: %w", id, err)
	}
	var group CustomerGroup
	if err := json.Unmarshal(body, &group); err != nil {
		return nil, fmt.Errorf("parse updated customer group %d: %w (raw: %s)", id, err, truncateBody(body))
	}
	return &group, nil
}

// DeleteCustomerGroup removes a group via DELETE /v2/customer_groups/{id}.
// Per BigCommerce: existing customers in the group are unassigned automatically.
func (c *Client) DeleteCustomerGroup(ctx context.Context, id int) error {
	if id <= 0 {
		return fmt.Errorf("customer group id must be positive")
	}
	if _, err := c.DeleteV2(ctx, fmt.Sprintf("customer_groups/%d", id)); err != nil {
		return fmt.Errorf("delete customer group %d: %w", id, err)
	}
	return nil
}

// truncateBody trims a raw response body for safe inclusion in error messages.
func truncateBody(b []byte) string {
	const max = 200
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "... (truncated)"
}
