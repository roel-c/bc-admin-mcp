package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// maxCouponCodeIDInDelete caps the number of code IDs in a single
// DELETE /v3/promotions/{id}/codes?id:in=… request. BigCommerce documents 50
// per call; we use 40 to stay under URL-length limits and align with the
// promotion-delete cap.
const maxCouponCodeIDInDelete = 40

// codeGenBatchSizeMax mirrors BigCommerce's hard cap on /codegen.batch_size.
const codeGenBatchSizeMax = 250

// couponCodeListWire matches the V3 list response envelope including cursor
// pagination meta.
type couponCodeListWire struct {
	Data []CouponCode `json:"data"`
	Meta struct {
		Cursor CouponCodeCursors `json:"cursor"`
	} `json:"meta"`
}

// couponCodeSingleWire matches the V3 single-resource response envelope used
// by POST /v3/promotions/{id}/codes.
type couponCodeSingleWire struct {
	Data CouponCode `json:"data"`
}

// CodeGenResult describes the codegen batch record BigCommerce returns from
// POST /promotions/{id}/codegen. NOTE: this response is a single object
// describing the generation batch — it does NOT contain the minted codes.
// Retrieve the actual codes via GET /promotions/{id}/codes (ListCouponCodes).
type CodeGenResult struct {
	ID        int `json:"id,omitempty"`
	BatchSize int `json:"batch_size,omitempty"`
}

// codeGenWire matches the V3 codegen response envelope. Data is an object
// (the batch record), not an array of codes.
type codeGenWire struct {
	Data json.RawMessage `json:"data"`
}

// ListCouponCodes fetches one cursor-paginated page of coupon codes for a
// promotion. BigCommerce uses cursor pagination here (before/after) rather
// than offset pagination, so callers walk the cursor to fetch all codes.
func (c *Client) ListCouponCodes(ctx context.Context, promotionID int, params CouponCodeListParams) (*CouponCodeListResponse, error) {
	if promotionID <= 0 {
		return nil, fmt.Errorf("promotion id must be positive")
	}
	path := fmt.Sprintf("promotions/%d/codes", promotionID)
	if q := buildCouponCodeListQuery(params); q != "" {
		path += "?" + q
	}
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list coupon codes for promotion %d: %w", promotionID, err)
	}
	var env couponCodeListWire
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse coupon codes response: %w", err)
	}
	return &CouponCodeListResponse{Codes: env.Data, Cursor: env.Meta.Cursor}, nil
}

func buildCouponCodeListQuery(p CouponCodeListParams) string {
	vals := url.Values{}
	if p.Before != "" {
		vals.Set("before", p.Before)
	}
	if p.After != "" {
		vals.Set("after", p.After)
	}
	if p.Limit > 0 {
		vals.Set("limit", strconv.Itoa(p.Limit))
	}
	return vals.Encode()
}

// CreateCouponCode creates a single coupon code on a promotion via
// POST /v3/promotions/{id}/codes.
//
// BigCommerce constraints:
//   - code: required, max 50 characters, allowed chars are letters, numbers,
//     spaces, underscores, and hyphens.
//   - max_uses=0 means unlimited; the parent promotion's max_uses overrides
//     this when both are set.
//   - There is no PUT — to change a code, delete and recreate.
func (c *Client) CreateCouponCode(ctx context.Context, promotionID int, payload CouponCodeCreate) (*CouponCode, error) {
	if promotionID <= 0 {
		return nil, fmt.Errorf("promotion id must be positive")
	}
	if payload.Code == "" {
		return nil, fmt.Errorf("coupon code is required")
	}
	body, err := c.Post(ctx, fmt.Sprintf("promotions/%d/codes", promotionID), payload)
	if err != nil {
		return nil, fmt.Errorf("create coupon code on promotion %d: %w", promotionID, err)
	}
	var env couponCodeSingleWire
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse create coupon code response: %w", err)
	}
	return &env.Data, nil
}

// DeleteCouponCodes removes coupon codes in chunks via
// DELETE /v3/promotions/{id}/codes?id:in=…. Chunking matches BC's documented
// limit of 50 per call (we use 40 for headroom).
func (c *Client) DeleteCouponCodes(ctx context.Context, promotionID int, ids []int) error {
	if promotionID <= 0 {
		return fmt.Errorf("promotion id must be positive")
	}
	if len(ids) == 0 {
		return fmt.Errorf("no coupon code ids provided")
	}
	for i := 0; i < len(ids); i += maxCouponCodeIDInDelete {
		end := i + maxCouponCodeIDInDelete
		if end > len(ids) {
			end = len(ids)
		}
		sub := ids[i:end]
		strs := make([]string, 0, len(sub))
		for _, id := range sub {
			if id <= 0 {
				return fmt.Errorf("coupon code id must be positive: %d", id)
			}
			strs = append(strs, strconv.Itoa(id))
		}
		path := fmt.Sprintf("promotions/%d/codes?", promotionID) +
			url.Values{"id:in": []string{strings.Join(strs, ",")}}.Encode()
		if _, err := c.Delete(ctx, path); err != nil {
			return fmt.Errorf("delete coupon codes chunk: %w", err)
		}
	}
	return nil
}

// GenerateCouponCodes mints a batch of coupon codes for a BULK coupon
// promotion via POST /v3/promotions/{id}/codegen.
//
// BigCommerce constraints:
//   - The parent promotion's coupon_type must be BULK; SINGLE promotions 422.
//   - batch_size is required and capped at 250 per request.
//   - length, when set, must be in [6, 16] (excluding prefix/suffix).
//
// We do not enforce coupon_type=BULK at the client layer — the tools layer
// pre-flights the parent and refuses on SINGLE before the request fires.
func (c *Client) GenerateCouponCodes(ctx context.Context, promotionID int, req CodeGenRequest) (*CodeGenResult, error) {
	if promotionID <= 0 {
		return nil, fmt.Errorf("promotion id must be positive")
	}
	if req.BatchSize <= 0 {
		return nil, fmt.Errorf("batch_size must be positive")
	}
	if req.BatchSize > codeGenBatchSizeMax {
		return nil, fmt.Errorf("batch_size exceeds BigCommerce maximum of %d per request", codeGenBatchSizeMax)
	}
	body, err := c.Post(ctx, fmt.Sprintf("promotions/%d/codegen", promotionID), req)
	if err != nil {
		return nil, fmt.Errorf("generate coupon codes for promotion %d: %w", promotionID, err)
	}
	var env codeGenWire
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse codegen response envelope: %w", err)
	}
	// data is an object (the batch record); decode best-effort. The codes
	// themselves are fetched separately via ListCouponCodes.
	res := &CodeGenResult{}
	_ = json.Unmarshal(env.Data, res)
	return res, nil
}
