package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// CategoryTree is a row from GET /v3/catalog/trees (Management API).
// Channels lists BigCommerce channel IDs tied to the tree (multi-storefront).
type CategoryTree struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Channels []int  `json:"channels,omitempty"`
}

// ListCategoryTrees returns category trees via GET /v3/catalog/trees.
// Optional params include channel_id:in and id:in (comma-separated values per API).
func (c *Client) ListCategoryTrees(ctx context.Context, params map[string]string) ([]CategoryTree, error) {
	path := "catalog/trees"
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
		return nil, fmt.Errorf("list category trees: %w", err)
	}

	out := make([]CategoryTree, 0, len(raw))
	for _, r := range raw {
		var t CategoryTree
		if err := json.Unmarshal(r, &t); err != nil {
			return nil, fmt.Errorf("unmarshal category tree: %w", err)
		}
		out = append(out, t)
	}
	return out, nil
}

// GetTreeIDForChannel returns a category tree ID for the given channel.
// If several trees match the filter, the first returned by the API is used.
func (c *Client) GetTreeIDForChannel(ctx context.Context, channelID int) (int, error) {
	if channelID <= 0 {
		return 0, fmt.Errorf("channel id must be positive")
	}
	trees, err := c.ListCategoryTrees(ctx, map[string]string{
		"channel_id:in": strconv.Itoa(channelID),
	})
	if err != nil {
		return 0, err
	}
	if len(trees) == 0 {
		return 0, fmt.Errorf("no category tree for channel %d", channelID)
	}
	return trees[0].ID, nil
}
