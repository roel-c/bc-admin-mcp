package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// maxPromotionIDInDelete caps the number of promotion IDs in a single
// DELETE /v3/promotions?id:in=… request. BigCommerce documents 50/call;
// we use 40 to stay under URL-length and concurrent-request limits.
const maxPromotionIDInDelete = 40

// promotionDataEnvelope is the V3 single-response shape for /v3/promotions/{id}.
type promotionDataEnvelope struct {
	Data Promotion `json:"data"`
}

// SearchPromotions lists promotions via GET /v3/promotions, applying the
// provided filter / sort / paging knobs. RedemptionType, when set, is
// forwarded as the documented `redemption_type` query parameter (e.g.
// `automatic`).
//
// Returns the paginated set as parsed Promotion values; the inner rules /
// notifications / customer trees stay as json.RawMessage so callers can pass
// them through verbatim or pretty-print without locking the schema.
//
// Pagination behavior:
//   - when params.Page or params.Limit is set, returns exactly that BC page
//   - otherwise, auto-paginates through all pages via GetAll
func (c *Client) SearchPromotions(ctx context.Context, params PromotionListParams) ([]Promotion, error) {
	path := "promotions"
	if q := buildPromotionListQuery(params); q != "" {
		path += "?" + q
	}

	if params.Page > 0 || params.Limit > 0 {
		body, err := c.Get(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("search promotions (single page): %w", err)
		}
		var resp PaginatedResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parse promotions response: %w", err)
		}
		return decodePromotions(resp.Data)
	}

	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("search promotions: %w", err)
	}
	return decodePromotions(raw)
}

func decodePromotions(raw []json.RawMessage) ([]Promotion, error) {
	out := make([]Promotion, 0, len(raw))
	for i, r := range raw {
		var p Promotion
		if err := json.Unmarshal(r, &p); err != nil {
			return nil, fmt.Errorf("unmarshal promotion at index %d: %w", i, err)
		}
		out = append(out, p)
	}
	return out, nil
}

func buildPromotionListQuery(p PromotionListParams) string {
	vals := url.Values{}
	if p.ID != 0 {
		vals.Set("id", strconv.Itoa(p.ID))
	}
	if p.Name != "" {
		vals.Set("name", p.Name)
	}
	if p.Code != "" {
		vals.Set("code", p.Code)
	}
	if p.Query != "" {
		vals.Set("query", p.Query)
	}
	if p.CurrencyCode != "" {
		vals.Set("currency_code", p.CurrencyCode)
	}
	if p.RedemptionType != "" {
		vals.Set("redemption_type", p.RedemptionType)
	}
	if p.Status != "" {
		vals.Set("status", p.Status)
	}
	if len(p.Channels) > 0 {
		ids := make([]string, 0, len(p.Channels))
		for _, id := range p.Channels {
			ids = append(ids, strconv.Itoa(id))
		}
		vals.Set("channels", strings.Join(ids, ","))
	}
	if p.Sort != "" {
		vals.Set("sort", p.Sort)
	}
	if p.Direction != "" {
		vals.Set("direction", p.Direction)
	}
	if p.Page > 0 {
		vals.Set("page", strconv.Itoa(p.Page))
	}
	if p.Limit > 0 {
		vals.Set("limit", strconv.Itoa(p.Limit))
	}
	return vals.Encode()
}

// GetPromotion fetches a single promotion by id via GET /v3/promotions/{id}.
func (c *Client) GetPromotion(ctx context.Context, id int) (*Promotion, error) {
	if id <= 0 {
		return nil, fmt.Errorf("promotion id must be positive")
	}
	body, err := c.Get(ctx, fmt.Sprintf("promotions/%d", id))
	if err != nil {
		return nil, fmt.Errorf("get promotion %d: %w", id, err)
	}
	var env promotionDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse promotion response: %w", err)
	}
	return &env.Data, nil
}

// CreatePromotion creates a single promotion via POST /v3/promotions. The
// payload is a raw JSON object so callers can pass the full BC schema
// without going through a typed Go AST. Validation is enforced at the tools
// layer before this point.
func (c *Client) CreatePromotion(ctx context.Context, payload json.RawMessage) (*Promotion, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty create promotion payload")
	}
	body, err := c.Post(ctx, "promotions", json.RawMessage(payload))
	if err != nil {
		return nil, fmt.Errorf("create promotion: %w", err)
	}
	var env promotionDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse create promotion response: %w", err)
	}
	return &env.Data, nil
}

// UpdatePromotion updates an existing promotion via PUT /v3/promotions/{id}.
// BC's PUT replaces the document, so the tools layer is responsible for
// merging top-level scalars and (optionally) splicing rules positionally
// before calling this method.
func (c *Client) UpdatePromotion(ctx context.Context, id int, payload json.RawMessage) (*Promotion, error) {
	if id <= 0 {
		return nil, fmt.Errorf("promotion id must be positive")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty update promotion payload")
	}
	body, err := c.Put(ctx, fmt.Sprintf("promotions/%d", id), json.RawMessage(payload))
	if err != nil {
		return nil, fmt.Errorf("update promotion %d: %w", id, err)
	}
	var env promotionDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse update promotion response: %w", err)
	}
	return &env.Data, nil
}

// promotionSettingsEnvelope is the V3 single-resource response shape for
// /v3/promotions/settings.
type promotionSettingsEnvelope struct {
	Data PromotionSettings `json:"data"`
}

// GetPromotionSettings reads the store-wide promotion settings via
// GET /v3/promotions/settings.
func (c *Client) GetPromotionSettings(ctx context.Context) (*PromotionSettings, error) {
	body, err := c.Get(ctx, "promotions/settings")
	if err != nil {
		return nil, fmt.Errorf("get promotion settings: %w", err)
	}
	var env promotionSettingsEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse promotion settings response: %w", err)
	}
	return &env.Data, nil
}

// UpdatePromotionSettings writes the store-wide promotion settings via
// PUT /v3/promotions/settings. BigCommerce expects the full settings object;
// the tools layer is responsible for fetch-then-merge before calling here.
//
// BigCommerce returns 403 when number_of_coupons_allowed_at_checkout > 1 on
// non-Enterprise plans; the error bubbles up unchanged so the tools layer
// can surface a clear hint.
func (c *Client) UpdatePromotionSettings(ctx context.Context, payload PromotionSettings) (*PromotionSettings, error) {
	body, err := c.Put(ctx, "promotions/settings", payload)
	if err != nil {
		return nil, fmt.Errorf("update promotion settings: %w", err)
	}
	var env promotionSettingsEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse update promotion settings response: %w", err)
	}
	return &env.Data, nil
}

// DeletePromotionsByIDs removes promotions in chunks via
// DELETE /v3/promotions?id:in=…. BigCommerce returns 422 when a promotion
// still has coupon codes attached; the error bubbles up unchanged so the
// tools layer can surface a hint.
func (c *Client) DeletePromotionsByIDs(ctx context.Context, ids []int) error {
	if len(ids) == 0 {
		return fmt.Errorf("no promotion ids provided")
	}
	for i := 0; i < len(ids); i += maxPromotionIDInDelete {
		end := i + maxPromotionIDInDelete
		if end > len(ids) {
			end = len(ids)
		}
		sub := ids[i:end]
		strs := make([]string, 0, len(sub))
		for _, id := range sub {
			if id <= 0 {
				return fmt.Errorf("promotion id must be positive: %d", id)
			}
			strs = append(strs, strconv.Itoa(id))
		}
		path := "promotions?" + url.Values{"id:in": []string{strings.Join(strs, ",")}}.Encode()
		if _, err := c.Delete(ctx, path); err != nil {
			return fmt.Errorf("delete promotions chunk: %w", err)
		}
	}
	return nil
}
