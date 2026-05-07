package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// maxSegmentIDInQuery caps the number of segment UUIDs in a single id:in query
// for /v3/segments. BC does not document a hard limit on the query string, so
// we keep this conservative to stay well under URL-length limits.
const maxSegmentIDInQuery = 40

// maxShopperProfileIDInQuery caps the number of shopper-profile UUIDs in a
// single id:in query for /v3/shopper-profiles and the membership endpoints.
const maxShopperProfileIDInQuery = 40

type segmentsDataEnvelope struct {
	Data []Segment `json:"data"`
}

type shopperProfilesDataEnvelope struct {
	Data []ShopperProfile `json:"data"`
}

// SearchSegments lists customer segments via GET /v3/segments. Supported params
// include id:in (UUIDs), page, and limit per BigCommerce.
func (c *Client) SearchSegments(ctx context.Context, params map[string]string) ([]Segment, error) {
	path := "segments"
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
		return nil, fmt.Errorf("search segments: %w", err)
	}
	out := make([]Segment, 0, len(raw))
	for _, r := range raw {
		var s Segment
		if err := json.Unmarshal(r, &s); err != nil {
			return nil, fmt.Errorf("unmarshal segment: %w", err)
		}
		out = append(out, s)
	}
	return out, nil
}

// GetSegmentsByIDs fetches segments by UUIDs using GET /v3/segments?id:in=…
// IDs are chunked to keep query strings safe.
func (c *Client) GetSegmentsByIDs(ctx context.Context, ids []string) ([]Segment, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("no segment ids provided")
	}
	var all []Segment
	for i := 0; i < len(ids); i += maxSegmentIDInQuery {
		end := i + maxSegmentIDInQuery
		if end > len(ids) {
			end = len(ids)
		}
		sub := ids[i:end]
		params := map[string]string{"id:in": strings.Join(sub, ",")}
		part, err := c.SearchSegments(ctx, params)
		if err != nil {
			return nil, err
		}
		all = append(all, part...)
	}
	return all, nil
}

// CreateSegments creates segments via POST /v3/segments. Body is an array.
// Callers should chunk per BigCommerce concurrency limits (10 concurrent).
func (c *Client) CreateSegments(ctx context.Context, payload []SegmentCreate) ([]Segment, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty create segments payload")
	}
	body, err := c.Post(ctx, "segments", payload)
	if err != nil {
		return nil, fmt.Errorf("create segments: %w", err)
	}
	var env segmentsDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse create segments response: %w", err)
	}
	return env.Data, nil
}

// UpdateSegments updates segments via PUT /v3/segments. Each row must include
// the segment ID. BigCommerce accepts partial updates of name and description.
func (c *Client) UpdateSegments(ctx context.Context, payload []SegmentUpdate) ([]Segment, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty update segments payload")
	}
	body, err := c.Put(ctx, "segments", payload)
	if err != nil {
		return nil, fmt.Errorf("update segments: %w", err)
	}
	var env segmentsDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse update segments response: %w", err)
	}
	return env.Data, nil
}

// DeleteSegments deletes segments via DELETE /v3/segments?id:in=… BigCommerce
// removes the segment metadata only; associated shopper profiles are kept.
func (c *Client) DeleteSegments(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("no segment ids provided")
	}
	for i := 0; i < len(ids); i += maxSegmentIDInQuery {
		end := i + maxSegmentIDInQuery
		if end > len(ids) {
			end = len(ids)
		}
		sub := ids[i:end]
		q := url.Values{}
		q.Set("id:in", strings.Join(sub, ","))
		if _, err := c.Delete(ctx, "segments?"+q.Encode()); err != nil {
			return fmt.Errorf("delete segments: %w", err)
		}
	}
	return nil
}

// ListShopperProfilesInSegment performs GET /v3/segments/{segmentId}/shopper-profiles.
// Note: BigCommerce requires the modify Customers OAuth scope for this endpoint
// (the only GET in the segmentation API that does so).
func (c *Client) ListShopperProfilesInSegment(ctx context.Context, segmentID string) ([]ShopperProfile, error) {
	if strings.TrimSpace(segmentID) == "" {
		return nil, fmt.Errorf("segment_id is required")
	}
	path := fmt.Sprintf("segments/%s/shopper-profiles", url.PathEscape(segmentID))
	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list shopper profiles in segment %s: %w", segmentID, err)
	}
	out := make([]ShopperProfile, 0, len(raw))
	for _, r := range raw {
		var p ShopperProfile
		if err := json.Unmarshal(r, &p); err != nil {
			return nil, fmt.Errorf("unmarshal shopper profile: %w", err)
		}
		out = append(out, p)
	}
	return out, nil
}

// AddShopperProfilesToSegment performs POST /v3/segments/{segmentId}/shopper-profiles
// with an array of shopper-profile UUIDs. BigCommerce caps each request at 50
// profiles; callers should chunk.
func (c *Client) AddShopperProfilesToSegment(ctx context.Context, segmentID string, profileIDs []string) ([]ShopperProfile, error) {
	if strings.TrimSpace(segmentID) == "" {
		return nil, fmt.Errorf("segment_id is required")
	}
	if len(profileIDs) == 0 {
		return nil, fmt.Errorf("no shopper profile ids provided")
	}
	path := fmt.Sprintf("segments/%s/shopper-profiles", url.PathEscape(segmentID))
	body, err := c.Post(ctx, path, profileIDs)
	if err != nil {
		return nil, fmt.Errorf("add shopper profiles to segment %s: %w", segmentID, err)
	}
	var wrap struct {
		Data []ShopperProfile `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("parse add shopper profiles response: %w", err)
	}
	return wrap.Data, nil
}

// RemoveShopperProfilesFromSegment performs DELETE /v3/segments/{segmentId}/shopper-profiles?id:in=…
// to disassociate profiles from a segment without deleting the profiles themselves.
func (c *Client) RemoveShopperProfilesFromSegment(ctx context.Context, segmentID string, profileIDs []string) error {
	if strings.TrimSpace(segmentID) == "" {
		return fmt.Errorf("segment_id is required")
	}
	if len(profileIDs) == 0 {
		return fmt.Errorf("no shopper profile ids provided")
	}
	for i := 0; i < len(profileIDs); i += maxShopperProfileIDInQuery {
		end := i + maxShopperProfileIDInQuery
		if end > len(profileIDs) {
			end = len(profileIDs)
		}
		sub := profileIDs[i:end]
		q := url.Values{}
		q.Set("id:in", strings.Join(sub, ","))
		path := fmt.Sprintf("segments/%s/shopper-profiles?%s", url.PathEscape(segmentID), q.Encode())
		if _, err := c.Delete(ctx, path); err != nil {
			return fmt.Errorf("remove shopper profiles from segment %s: %w", segmentID, err)
		}
	}
	return nil
}

// ListShopperProfiles performs GET /v3/shopper-profiles. The endpoint supports
// only page and limit; there is no id:in filter or single-profile GET.
func (c *Client) ListShopperProfiles(ctx context.Context, params map[string]string) ([]ShopperProfile, error) {
	path := "shopper-profiles"
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
		return nil, fmt.Errorf("list shopper profiles: %w", err)
	}
	out := make([]ShopperProfile, 0, len(raw))
	for _, r := range raw {
		var p ShopperProfile
		if err := json.Unmarshal(r, &p); err != nil {
			return nil, fmt.Errorf("unmarshal shopper profile: %w", err)
		}
		out = append(out, p)
	}
	return out, nil
}

// CreateShopperProfiles performs POST /v3/shopper-profiles with an array of
// {customer_id} entries. Each profile is 1:1 with a registered customer.
func (c *Client) CreateShopperProfiles(ctx context.Context, payload []ShopperProfileCreate) ([]ShopperProfile, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty shopper profile create payload")
	}
	body, err := c.Post(ctx, "shopper-profiles", payload)
	if err != nil {
		return nil, fmt.Errorf("create shopper profiles: %w", err)
	}
	var env shopperProfilesDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse create shopper profiles response: %w", err)
	}
	return env.Data, nil
}

// DeleteShopperProfiles performs DELETE /v3/shopper-profiles?id:in=… to remove
// profiles. BC removes the profiles and their segment memberships; the customer
// records themselves are not touched.
func (c *Client) DeleteShopperProfiles(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("no shopper profile ids provided")
	}
	for i := 0; i < len(ids); i += maxShopperProfileIDInQuery {
		end := i + maxShopperProfileIDInQuery
		if end > len(ids) {
			end = len(ids)
		}
		sub := ids[i:end]
		q := url.Values{}
		q.Set("id:in", strings.Join(sub, ","))
		if _, err := c.Delete(ctx, "shopper-profiles?"+q.Encode()); err != nil {
			return fmt.Errorf("delete shopper profiles: %w", err)
		}
	}
	return nil
}

// ListSegmentsForShopperProfile performs GET /v3/shopper-profiles/{shopperProfileId}/segments
// to list all segments containing a profile.
func (c *Client) ListSegmentsForShopperProfile(ctx context.Context, profileID string) ([]Segment, error) {
	if strings.TrimSpace(profileID) == "" {
		return nil, fmt.Errorf("shopper_profile_id is required")
	}
	path := fmt.Sprintf("shopper-profiles/%s/segments", url.PathEscape(profileID))
	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list segments for shopper profile %s: %w", profileID, err)
	}
	out := make([]Segment, 0, len(raw))
	for _, r := range raw {
		var s Segment
		if err := json.Unmarshal(r, &s); err != nil {
			return nil, fmt.Errorf("unmarshal segment: %w", err)
		}
		out = append(out, s)
	}
	return out, nil
}
