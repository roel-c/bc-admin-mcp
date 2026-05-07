package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type priceListDataEnvelope struct {
	Data PriceList `json:"data"`
}

type priceListRecordDataEnvelope struct {
	Data []PriceListRecord `json:"data"`
}

type priceListAssignmentDataEnvelope struct {
	Data PriceListAssignment `json:"data"`
}

// ListPriceLists returns price lists from GET /v3/pricelists.
// When any explicit pagination cursor/page option is provided, this returns
// just that page. Otherwise it auto-paginates with GetAll.
func (c *Client) ListPriceLists(ctx context.Context, params PriceListListParams) ([]PriceList, error) {
	path := "pricelists"
	if q := buildPriceListListQuery(params); q != "" {
		path += "?" + q
	}

	if usesExplicitPagination(params.Page, params.Limit, params.Before, params.After) {
		body, err := c.Get(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("list price lists (single page): %w", err)
		}
		var resp PaginatedResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parse price lists response: %w", err)
		}
		return decodePriceLists(resp.Data)
	}

	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list price lists: %w", err)
	}
	return decodePriceLists(raw)
}

// GetPriceList fetches one price list by id from GET /v3/pricelists/{id}.
func (c *Client) GetPriceList(ctx context.Context, priceListID int) (*PriceList, error) {
	if priceListID <= 0 {
		return nil, fmt.Errorf("price list id must be positive")
	}
	body, err := c.Get(ctx, fmt.Sprintf("pricelists/%d", priceListID))
	if err != nil {
		return nil, fmt.Errorf("get price list %d: %w", priceListID, err)
	}
	var env priceListDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse price list %d response: %w", priceListID, err)
	}
	return &env.Data, nil
}

// CreatePriceList creates a price list via POST /v3/pricelists.
func (c *Client) CreatePriceList(ctx context.Context, payload PriceListCreate) (*PriceList, error) {
	if strings.TrimSpace(payload.Name) == "" {
		return nil, fmt.Errorf("price list name is required")
	}
	body, err := c.Post(ctx, "pricelists", payload)
	if err != nil {
		return nil, fmt.Errorf("create price list: %w", err)
	}
	var env priceListDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse create price list response: %w", err)
	}
	return &env.Data, nil
}

// UpdatePriceList updates a price list via PUT /v3/pricelists/{id}.
func (c *Client) UpdatePriceList(ctx context.Context, priceListID int, payload PriceListUpdate) (*PriceList, error) {
	if priceListID <= 0 {
		return nil, fmt.Errorf("price list id must be positive")
	}
	body, err := c.Put(ctx, fmt.Sprintf("pricelists/%d", priceListID), payload)
	if err != nil {
		return nil, fmt.Errorf("update price list %d: %w", priceListID, err)
	}
	var env priceListDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse update price list response: %w", err)
	}
	return &env.Data, nil
}

// DeletePriceList deletes one price list via DELETE /v3/pricelists/{id}.
func (c *Client) DeletePriceList(ctx context.Context, priceListID int) error {
	if priceListID <= 0 {
		return fmt.Errorf("price list id must be positive")
	}
	if _, err := c.Delete(ctx, fmt.Sprintf("pricelists/%d", priceListID)); err != nil {
		return fmt.Errorf("delete price list %d: %w", priceListID, err)
	}
	return nil
}

// ListPriceListRecords returns records for one price list from
// GET /v3/pricelists/{id}/records.
func (c *Client) ListPriceListRecords(ctx context.Context, priceListID int, params PriceListRecordListParams) ([]PriceListRecord, error) {
	if priceListID <= 0 {
		return nil, fmt.Errorf("price list id must be positive")
	}

	path := fmt.Sprintf("pricelists/%d/records", priceListID)
	if q := buildPriceListRecordListQuery(params); q != "" {
		path += "?" + q
	}

	if usesExplicitPagination(params.Page, params.Limit, params.Before, params.After) {
		body, err := c.Get(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("list price list records (single page): %w", err)
		}
		var env priceListRecordDataEnvelope
		if err := json.Unmarshal(body, &env); err != nil {
			return nil, fmt.Errorf("parse price list records response: %w", err)
		}
		return env.Data, nil
	}

	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list price list records: %w", err)
	}
	return decodePriceListRecords(raw)
}

// UpsertPriceListRecords creates/updates records for one list via
// PUT /v3/pricelists/{id}/records.
func (c *Client) UpsertPriceListRecords(ctx context.Context, priceListID int, records []PriceListRecordUpsert) error {
	if priceListID <= 0 {
		return fmt.Errorf("price list id must be positive")
	}
	if len(records) == 0 {
		return fmt.Errorf("no price list records provided")
	}
	if _, err := c.Put(ctx, fmt.Sprintf("pricelists/%d/records", priceListID), records); err != nil {
		return fmt.Errorf("upsert price list records for %d: %w", priceListID, err)
	}
	return nil
}

// DeletePriceListRecords removes records for one price list via
// DELETE /v3/pricelists/{id}/records.
func (c *Client) DeletePriceListRecords(ctx context.Context, priceListID int, params PriceListRecordDeleteParams) error {
	if priceListID <= 0 {
		return fmt.Errorf("price list id must be positive")
	}

	path := fmt.Sprintf("pricelists/%d/records", priceListID)
	if q := buildPriceListRecordDeleteQuery(params); q != "" {
		path += "?" + q
	}

	if _, err := c.Delete(ctx, path); err != nil {
		return fmt.Errorf("delete price list records for %d: %w", priceListID, err)
	}
	return nil
}

// ListPriceListAssignments returns assignment rows from
// GET /v3/pricelists/assignments.
func (c *Client) ListPriceListAssignments(ctx context.Context, params PriceListAssignmentListParams) ([]PriceListAssignment, error) {
	path := "pricelists/assignments"
	if q := buildPriceListAssignmentListQuery(params); q != "" {
		path += "?" + q
	}

	if usesExplicitPagination(params.Page, params.Limit, params.Before, params.After) {
		body, err := c.Get(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("list price list assignments (single page): %w", err)
		}
		var resp PaginatedResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parse price list assignments response: %w", err)
		}
		return decodePriceListAssignments(resp.Data)
	}

	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list price list assignments: %w", err)
	}
	return decodePriceListAssignments(raw)
}

// CreatePriceListAssignments creates assignment rows via
// POST /v3/pricelists/assignments.
func (c *Client) CreatePriceListAssignments(ctx context.Context, assignments []PriceListAssignmentCreate) error {
	if len(assignments) == 0 {
		return fmt.Errorf("no price list assignments provided")
	}
	if _, err := c.Post(ctx, "pricelists/assignments", assignments); err != nil {
		return fmt.Errorf("create price list assignments: %w", err)
	}
	return nil
}

// UpsertPriceListAssignment upserts one assignment for a price list via
// PUT /v3/pricelists/{price_list_id}/assignments.
func (c *Client) UpsertPriceListAssignment(ctx context.Context, priceListID int, payload PriceListAssignmentUpsert) (*PriceListAssignment, error) {
	if priceListID <= 0 {
		return nil, fmt.Errorf("price list id must be positive")
	}
	body, err := c.Put(ctx, fmt.Sprintf("pricelists/%d/assignments", priceListID), payload)
	if err != nil {
		return nil, fmt.Errorf("upsert price list assignment for %d: %w", priceListID, err)
	}
	var env priceListAssignmentDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse upsert price list assignment response: %w", err)
	}
	return &env.Data, nil
}

// DeletePriceListAssignments removes assignments via
// DELETE /v3/pricelists/assignments with one or more filter parameters.
func (c *Client) DeletePriceListAssignments(ctx context.Context, params PriceListAssignmentDeleteParams) error {
	q := buildPriceListAssignmentDeleteQuery(params)
	if q == "" {
		return fmt.Errorf("at least one assignment filter is required for delete")
	}
	if _, err := c.Delete(ctx, "pricelists/assignments?"+q); err != nil {
		return fmt.Errorf("delete price list assignments: %w", err)
	}
	return nil
}

func buildPriceListListQuery(p PriceListListParams) string {
	vals := url.Values{}
	if p.ID > 0 {
		vals.Set("id", strconv.Itoa(p.ID))
	}
	if len(p.IDs) > 0 {
		vals.Set("id:in", joinInts(p.IDs))
	}
	if p.Name != "" {
		vals.Set("name", p.Name)
	}
	if p.NameLike != "" {
		vals.Set("name:like", p.NameLike)
	}
	if p.DateCreated != "" {
		vals.Set("date_created", p.DateCreated)
	}
	if p.DateModified != "" {
		vals.Set("date_modified", p.DateModified)
	}
	if p.DateCreatedMin != "" {
		vals.Set("date_created:min", p.DateCreatedMin)
	}
	if p.DateCreatedMax != "" {
		vals.Set("date_created:max", p.DateCreatedMax)
	}
	if p.DateModifiedMin != "" {
		vals.Set("date_modified:min", p.DateModifiedMin)
	}
	if p.DateModifiedMax != "" {
		vals.Set("date_modified:max", p.DateModifiedMax)
	}
	if p.Page > 0 {
		vals.Set("page", strconv.Itoa(p.Page))
	}
	if p.Limit > 0 {
		vals.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Before != "" {
		vals.Set("before", p.Before)
	}
	if p.After != "" {
		vals.Set("after", p.After)
	}
	return vals.Encode()
}

func buildPriceListRecordListQuery(p PriceListRecordListParams) string {
	vals := url.Values{}
	if len(p.VariantIDs) > 0 {
		vals.Set("variant_id:in", joinInts(p.VariantIDs))
	}
	if len(p.ProductIDs) > 0 {
		vals.Set("product_id:in", joinInts(p.ProductIDs))
	}
	if p.SKU != "" {
		vals.Set("sku", p.SKU)
	}
	if len(p.SKUs) > 0 {
		vals.Set("sku:in", strings.Join(p.SKUs, ","))
	}
	if p.Currency != "" {
		vals.Set("currency", p.Currency)
	}
	if len(p.Currencies) > 0 {
		vals.Set("currency:in", strings.Join(p.Currencies, ","))
	}
	if len(p.Include) > 0 {
		vals.Set("include", strings.Join(p.Include, ","))
	}
	if p.Page > 0 {
		vals.Set("page", strconv.Itoa(p.Page))
	}
	if p.Limit > 0 {
		vals.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Before != "" {
		vals.Set("before", p.Before)
	}
	if p.After != "" {
		vals.Set("after", p.After)
	}
	return vals.Encode()
}

func buildPriceListRecordDeleteQuery(p PriceListRecordDeleteParams) string {
	vals := url.Values{}
	if p.Currency != "" {
		vals.Set("currency", p.Currency)
	}
	if len(p.VariantIDs) > 0 {
		vals.Set("variant_id:in", joinInts(p.VariantIDs))
	}
	if len(p.SKUs) > 0 {
		vals.Set("sku:in", strings.Join(p.SKUs, ","))
	}
	return vals.Encode()
}

func buildPriceListAssignmentListQuery(p PriceListAssignmentListParams) string {
	vals := url.Values{}
	if p.ID > 0 {
		vals.Set("id", strconv.Itoa(p.ID))
	}
	if p.PriceListID > 0 {
		vals.Set("price_list_id", strconv.Itoa(p.PriceListID))
	}
	if p.CustomerGroupID > 0 {
		vals.Set("customer_group_id", strconv.Itoa(p.CustomerGroupID))
	}
	if p.ChannelID > 0 {
		vals.Set("channel_id", strconv.Itoa(p.ChannelID))
	}
	if len(p.IDs) > 0 {
		vals.Set("id:in", joinInts(p.IDs))
	}
	if len(p.PriceListIDs) > 0 {
		vals.Set("price_list_id:in", joinInts(p.PriceListIDs))
	}
	if len(p.CustomerGroupIDs) > 0 {
		vals.Set("customer_group_id:in", joinInts(p.CustomerGroupIDs))
	}
	if len(p.ChannelIDs) > 0 {
		vals.Set("channel_id:in", joinInts(p.ChannelIDs))
	}
	if p.Page > 0 {
		vals.Set("page", strconv.Itoa(p.Page))
	}
	if p.Limit > 0 {
		vals.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Before != "" {
		vals.Set("before", p.Before)
	}
	if p.After != "" {
		vals.Set("after", p.After)
	}
	return vals.Encode()
}

func buildPriceListAssignmentDeleteQuery(p PriceListAssignmentDeleteParams) string {
	vals := url.Values{}
	if p.ID > 0 {
		vals.Set("id", strconv.Itoa(p.ID))
	}
	if p.PriceListID > 0 {
		vals.Set("price_list_id", strconv.Itoa(p.PriceListID))
	}
	if p.CustomerGroupID > 0 {
		vals.Set("customer_group_id", strconv.Itoa(p.CustomerGroupID))
	}
	if p.ChannelID > 0 {
		vals.Set("channel_id", strconv.Itoa(p.ChannelID))
	}
	if len(p.ChannelIDs) > 0 {
		vals.Set("channel_id:in", joinInts(p.ChannelIDs))
	}
	return vals.Encode()
}

func usesExplicitPagination(page, limit int, before, after string) bool {
	return page > 0 || limit > 0 || before != "" || after != ""
}

func decodePriceLists(raw []json.RawMessage) ([]PriceList, error) {
	out := make([]PriceList, 0, len(raw))
	for i, r := range raw {
		var row PriceList
		if err := json.Unmarshal(r, &row); err != nil {
			return nil, fmt.Errorf("unmarshal price list at index %d: %w", i, err)
		}
		out = append(out, row)
	}
	return out, nil
}

func decodePriceListRecords(raw []json.RawMessage) ([]PriceListRecord, error) {
	out := make([]PriceListRecord, 0, len(raw))
	for i, r := range raw {
		var row PriceListRecord
		if err := json.Unmarshal(r, &row); err != nil {
			return nil, fmt.Errorf("unmarshal price list record at index %d: %w", i, err)
		}
		out = append(out, row)
	}
	return out, nil
}

func decodePriceListAssignments(raw []json.RawMessage) ([]PriceListAssignment, error) {
	out := make([]PriceListAssignment, 0, len(raw))
	for i, r := range raw {
		var row PriceListAssignment
		if err := json.Unmarshal(r, &row); err != nil {
			return nil, fmt.Errorf("unmarshal price list assignment at index %d: %w", i, err)
		}
		out = append(out, row)
	}
	return out, nil
}
