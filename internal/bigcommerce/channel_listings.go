package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// ChannelListing is a row from GET /v3/channels/{channel_id}/listings.
// Variants holds the raw JSON array from BigCommerce (shape varies by channel).
type ChannelListing struct {
	ChannelID   int             `json:"channel_id"`
	ListingID   int             `json:"listing_id"`
	ProductID   int             `json:"product_id"`
	State       string          `json:"state,omitempty"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	ExternalID  string          `json:"external_id,omitempty"`
	Variants    json.RawMessage `json:"variants,omitempty"`
}

const (
	maxChannelListingsPages  = 100
	maxChannelListingsTotal  = 2000
	defaultListingsPageLimit = 50
)

// ListChannelListings fetches all pages of GET /v3/channels/{channel_id}/listings
// using limit + after cursor pagination. Optional query keys: product_id:in (comma-separated).
func (c *Client) ListChannelListings(ctx context.Context, channelID int, extraQuery map[string]string) ([]ChannelListing, error) {
	if channelID <= 0 {
		return nil, fmt.Errorf("channel id must be positive")
	}
	base := fmt.Sprintf("channels/%d/listings", channelID)
	limit := c.cfg.DefaultPageLimit
	if limit <= 0 {
		limit = defaultListingsPageLimit
	}
	if limit > 250 {
		limit = 250
	}

	var all []ChannelListing
	after := ""

	for page := 0; page < maxChannelListingsPages; page++ {
		vals := url.Values{}
		vals.Set("limit", strconv.Itoa(limit))
		if after != "" {
			vals.Set("after", after)
		}
		for k, v := range extraQuery {
			if v != "" {
				vals.Set(k, v)
			}
		}

		path := base + "?" + vals.Encode()
		body, err := c.Get(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("list channel listings (page %d): %w", page+1, err)
		}

		var resp struct {
			Data []json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parse channel listings page %d: %w", page+1, err)
		}
		if len(resp.Data) == 0 {
			break
		}

		var lastListingID int
		for _, raw := range resp.Data {
			var row ChannelListing
			if err := json.Unmarshal(raw, &row); err != nil {
				return nil, fmt.Errorf("unmarshal listing: %w", err)
			}
			all = append(all, row)
			if row.ListingID > 0 {
				lastListingID = row.ListingID
			}
		}

		if len(all) >= maxChannelListingsTotal {
			c.logger.Warn("channel listings fetch truncated at max",
				"max", maxChannelListingsTotal,
				"channel_id", channelID,
			)
			all = all[:maxChannelListingsTotal]
			break
		}

		if len(resp.Data) < limit || lastListingID == 0 {
			break
		}
		after = strconv.Itoa(lastListingID)
	}

	return all, nil
}

// CreateChannelListings POSTs a JSON array body to /v3/channels/{channel_id}/listings.
func (c *Client) CreateChannelListings(ctx context.Context, channelID int, body any) ([]byte, error) {
	if channelID <= 0 {
		return nil, fmt.Errorf("channel id must be positive")
	}
	path := fmt.Sprintf("channels/%d/listings", channelID)
	return c.Post(ctx, path, body)
}

// UpdateChannelListings PUTs a JSON array body to /v3/channels/{channel_id}/listings.
func (c *Client) UpdateChannelListings(ctx context.Context, channelID int, body any) ([]byte, error) {
	if channelID <= 0 {
		return nil, fmt.Errorf("channel id must be positive")
	}
	path := fmt.Sprintf("channels/%d/listings", channelID)
	return c.Put(ctx, path, body)
}
