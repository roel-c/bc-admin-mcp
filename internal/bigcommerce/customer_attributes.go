package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// maxCustomerMetafieldIDInQuery caps the number of metafield ids in a single
// id:in query for /v3/customers/{customerId}/metafields and the batch endpoint.
const maxCustomerMetafieldIDInQuery = 50

type customerAttributesDataEnvelope struct {
	Data []CustomerAttribute `json:"data"`
}

type customerAttributeValuesDataEnvelope struct {
	Data []CustomerAttributeValue `json:"data"`
}

type metafieldsDataEnvelope struct {
	Data []Metafield `json:"data"`
}

// SearchCustomerAttributes lists customer attributes via GET /v3/customers/attributes.
// Supports filters via query params (id, id:in, name:like, etc.) plus paging.
func (c *Client) SearchCustomerAttributes(ctx context.Context, params map[string]string) ([]CustomerAttribute, error) {
	path := "customers/attributes"
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
		return nil, fmt.Errorf("search customer attributes: %w", err)
	}
	out := make([]CustomerAttribute, 0, len(raw))
	for _, r := range raw {
		var attr CustomerAttribute
		if err := json.Unmarshal(r, &attr); err != nil {
			return nil, fmt.Errorf("unmarshal customer attribute: %w", err)
		}
		out = append(out, attr)
	}
	return out, nil
}

// GetCustomerAttributesByIDs fetches attributes by numeric ID.
//
// Unlike most V3 list endpoints, GET /v3/customers/attributes does NOT
// support id / id:in filtering — BigCommerce returns a 422 ("The filter(s):
// id:in are not valid filter parameter(s)") for any attempt to filter by id.
// Confirmed live (FOLLOW-UPS.md FU-8); this endpoint only supports
// name/name:like filters or a full unfiltered list. There is no
// server-side way to narrow by ID, so this fetches the full attribute set
// and filters client-side. Stores are expected to have a small number of
// attribute definitions (BC's own UI is designed around this), so this is
// not a pagination-cost concern in practice.
func (c *Client) GetCustomerAttributesByIDs(ctx context.Context, ids []int) ([]CustomerAttribute, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("no attribute ids provided")
	}
	all, err := c.SearchCustomerAttributes(ctx, nil)
	if err != nil {
		return nil, err
	}
	want := make(map[int]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	out := make([]CustomerAttribute, 0, len(ids))
	for _, attr := range all {
		if want[attr.ID] {
			out = append(out, attr)
		}
	}
	return out, nil
}

// CreateCustomerAttributes creates attributes via POST /v3/customers/attributes.
// BigCommerce caps the request body at 10 rows; callers should chunk above that.
func (c *Client) CreateCustomerAttributes(ctx context.Context, payload []CustomerAttributeCreate) ([]CustomerAttribute, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty create attribute payload")
	}
	body, err := c.Post(ctx, "customers/attributes", payload)
	if err != nil {
		return nil, fmt.Errorf("create customer attributes: %w", err)
	}
	var env customerAttributesDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse create attributes response: %w", err)
	}
	return env.Data, nil
}

// UpdateCustomerAttributes updates attribute names via PUT /v3/customers/attributes.
// Only `name` is mutable; `type` is fixed at create time.
func (c *Client) UpdateCustomerAttributes(ctx context.Context, payload []CustomerAttributeUpdate) ([]CustomerAttribute, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty update attribute payload")
	}
	body, err := c.Put(ctx, "customers/attributes", payload)
	if err != nil {
		return nil, fmt.Errorf("update customer attributes: %w", err)
	}
	var env customerAttributesDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse update attributes response: %w", err)
	}
	return env.Data, nil
}

// DeleteCustomerAttributes deletes attributes via DELETE /v3/customers/attributes?id:in=…
// Deleting an attribute also deletes every value of that attribute on every customer.
func (c *Client) DeleteCustomerAttributes(ctx context.Context, ids []int) error {
	if len(ids) == 0 {
		return fmt.Errorf("no attribute ids provided")
	}
	q := url.Values{}
	q.Set("id:in", joinInts(ids))
	if _, err := c.Delete(ctx, "customers/attributes?"+q.Encode()); err != nil {
		return fmt.Errorf("delete customer attributes: %w", err)
	}
	return nil
}

// SearchCustomerAttributeValues lists attribute values via GET /v3/customers/attribute-values.
// Supports filters via params (customer_id:in, attribute_id:in, attribute_value, etc.).
func (c *Client) SearchCustomerAttributeValues(ctx context.Context, params map[string]string) ([]CustomerAttributeValue, error) {
	path := "customers/attribute-values"
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
		return nil, fmt.Errorf("search customer attribute values: %w", err)
	}
	out := make([]CustomerAttributeValue, 0, len(raw))
	for _, r := range raw {
		var v CustomerAttributeValue
		if err := json.Unmarshal(r, &v); err != nil {
			return nil, fmt.Errorf("unmarshal customer attribute value: %w", err)
		}
		out = append(out, v)
	}
	return out, nil
}

// UpsertCustomerAttributeValues upserts attribute values via PUT /v3/customers/attribute-values.
// BigCommerce uses the (customer_id, attribute_id) pair as the natural key.
// Up to 10 rows per call per BigCommerce limits; callers should chunk.
func (c *Client) UpsertCustomerAttributeValues(ctx context.Context, payload []CustomerAttributeValueUpsert) ([]CustomerAttributeValue, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty attribute value payload")
	}
	body, err := c.Put(ctx, "customers/attribute-values", payload)
	if err != nil {
		return nil, fmt.Errorf("upsert customer attribute values: %w", err)
	}
	var env customerAttributeValuesDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse upsert attribute values response: %w", err)
	}
	return env.Data, nil
}

// DeleteCustomerAttributeValues removes attribute values via
// DELETE /v3/customers/attribute-values?id:in=…
func (c *Client) DeleteCustomerAttributeValues(ctx context.Context, ids []int) error {
	if len(ids) == 0 {
		return fmt.Errorf("no attribute value ids provided")
	}
	q := url.Values{}
	q.Set("id:in", joinInts(ids))
	if _, err := c.Delete(ctx, "customers/attribute-values?"+q.Encode()); err != nil {
		return fmt.Errorf("delete customer attribute values: %w", err)
	}
	return nil
}

// ListCustomerMetafields fetches metafields for one customer via
// GET /v3/customers/{customerId}/metafields.
func (c *Client) ListCustomerMetafields(ctx context.Context, customerID int) ([]Metafield, error) {
	path := fmt.Sprintf("customers/%d/metafields", customerID)
	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list metafields for customer %d: %w", customerID, err)
	}
	mfs := make([]Metafield, 0, len(raw))
	for _, r := range raw {
		var mf Metafield
		if err := json.Unmarshal(r, &mf); err != nil {
			return nil, fmt.Errorf("unmarshal metafield: %w", err)
		}
		mfs = append(mfs, mf)
	}
	return mfs, nil
}

// CreateCustomerMetafield creates a metafield on a customer via
// POST /v3/customers/{customerId}/metafields.
func (c *Client) CreateCustomerMetafield(ctx context.Context, customerID int, mf Metafield) (*Metafield, error) {
	path := fmt.Sprintf("customers/%d/metafields", customerID)
	respBody, err := c.Post(ctx, path, mf)
	if err != nil {
		return nil, fmt.Errorf("create metafield on customer %d: %w", customerID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse metafield response: %w", err)
	}
	var created Metafield
	if err := json.Unmarshal(resp.Data, &created); err != nil {
		return nil, fmt.Errorf("unmarshal created metafield: %w", err)
	}
	return &created, nil
}

// UpdateCustomerMetafield updates a metafield on a customer via
// PUT /v3/customers/{customerId}/metafields/{metafieldId}.
func (c *Client) UpdateCustomerMetafield(ctx context.Context, customerID, metafieldID int, mf Metafield) (*Metafield, error) {
	path := fmt.Sprintf("customers/%d/metafields/%d", customerID, metafieldID)
	respBody, err := c.Put(ctx, path, mf)
	if err != nil {
		return nil, fmt.Errorf("update metafield %d on customer %d: %w", metafieldID, customerID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse metafield response: %w", err)
	}
	var updated Metafield
	if err := json.Unmarshal(resp.Data, &updated); err != nil {
		return nil, fmt.Errorf("unmarshal updated metafield: %w", err)
	}
	return &updated, nil
}

// DeleteCustomerMetafield removes one metafield from a customer via
// DELETE /v3/customers/{customerId}/metafields/{metafieldId}.
func (c *Client) DeleteCustomerMetafield(ctx context.Context, customerID, metafieldID int) error {
	path := fmt.Sprintf("customers/%d/metafields/%d", customerID, metafieldID)
	if _, err := c.Delete(ctx, path); err != nil {
		return fmt.Errorf("delete metafield %d on customer %d: %w", metafieldID, customerID, err)
	}
	return nil
}

// SearchAllCustomerMetafields lists metafields across customers via
// GET /v3/customers/metafields. Supports filters such as customer_id:in and key:in.
func (c *Client) SearchAllCustomerMetafields(ctx context.Context, params map[string]string) ([]Metafield, error) {
	path := "customers/metafields"
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
		return nil, fmt.Errorf("search customer metafields: %w", err)
	}
	out := make([]Metafield, 0, len(raw))
	for _, r := range raw {
		var mf Metafield
		if err := json.Unmarshal(r, &mf); err != nil {
			return nil, fmt.Errorf("unmarshal metafield: %w", err)
		}
		out = append(out, mf)
	}
	return out, nil
}

// CreateCustomerMetafieldsBatch creates many customer metafields at once via
// POST /v3/customers/metafields. Each row must include resource_id (customer id).
// BigCommerce caps the batch at 10 rows.
func (c *Client) CreateCustomerMetafieldsBatch(ctx context.Context, payload []Metafield) ([]Metafield, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty metafield create payload")
	}
	body, err := c.Post(ctx, "customers/metafields", payload)
	if err != nil {
		return nil, fmt.Errorf("batch create customer metafields: %w", err)
	}
	var env metafieldsDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse batch create metafields response: %w", err)
	}
	return env.Data, nil
}

// UpdateCustomerMetafieldsBatch updates many customer metafields via
// PUT /v3/customers/metafields. Each row must include id.
func (c *Client) UpdateCustomerMetafieldsBatch(ctx context.Context, payload []Metafield) ([]Metafield, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty metafield update payload")
	}
	body, err := c.Put(ctx, "customers/metafields", payload)
	if err != nil {
		return nil, fmt.Errorf("batch update customer metafields: %w", err)
	}
	var env metafieldsDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse batch update metafields response: %w", err)
	}
	return env.Data, nil
}

// DeleteCustomerMetafieldsBatch deletes customer metafields via
// DELETE /v3/customers/metafields?id:in=… IDs are chunked to keep the URL safe.
func (c *Client) DeleteCustomerMetafieldsBatch(ctx context.Context, ids []int) error {
	if len(ids) == 0 {
		return fmt.Errorf("no metafield ids provided")
	}
	for i := 0; i < len(ids); i += maxCustomerMetafieldIDInQuery {
		end := i + maxCustomerMetafieldIDInQuery
		if end > len(ids) {
			end = len(ids)
		}
		sub := ids[i:end]
		q := url.Values{}
		q.Set("id:in", joinInts(sub))
		if _, err := c.Delete(ctx, "customers/metafields?"+q.Encode()); err != nil {
			return fmt.Errorf("batch delete customer metafields: %w", err)
		}
	}
	return nil
}
