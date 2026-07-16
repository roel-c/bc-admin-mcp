package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
)

// B2BChannel is a storefront channel as seen by B2B Edition. Note that id (B2B
// Edition's own ID) and channelId (the BigCommerce channel ID) differ.
type B2BChannel struct {
	ID        int    `json:"id"`
	ChannelID int    `json:"channelId"`
	Name      string `json:"name"`
	Type      string `json:"type,omitempty"`
	Platform  string `json:"platform,omitempty"`
	Status    string `json:"status,omitempty"`
	IconURL   string `json:"iconUrl,omitempty"`
}

// ListB2BChannels returns the storefront channels known to B2B Edition.
func (c *B2BClient) ListB2BChannels(ctx context.Context) ([]B2BChannel, error) {
	raw, err := c.B2BGetAll(ctx, "channels")
	if err != nil {
		return nil, fmt.Errorf("list B2B channels: %w", err)
	}
	out := make([]B2BChannel, 0, len(raw))
	for _, r := range raw {
		var ch B2BChannel
		if err := json.Unmarshal(r, &ch); err != nil {
			return nil, fmt.Errorf("unmarshal B2B channel: %w", err)
		}
		out = append(out, ch)
	}
	return out, nil
}

// GetB2BChannel returns a single channel by its BigCommerce channel ID.
func (c *B2BClient) GetB2BChannel(ctx context.Context, channelID int) (*B2BChannel, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("channels/%d", channelID))
	if err != nil {
		return nil, fmt.Errorf("get B2B channel %d: %w", channelID, err)
	}
	var ch B2BChannel
	if err := b2bUnmarshalSingle(body, &ch, "get B2B channel"); err != nil {
		return nil, err
	}
	return &ch, nil
}
