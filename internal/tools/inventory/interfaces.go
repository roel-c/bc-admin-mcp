package inventory

import (
	"context"
	"encoding/json"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
)

// Compile-time check that *bigcommerce.Client satisfies BigCommerceInventoryAPI.
var _ BigCommerceInventoryAPI = (*bigcommerce.Client)(nil)

// BigCommerceInventoryAPI defines the BigCommerce client methods used by
// inventory-domain tool handlers.
type BigCommerceInventoryAPI interface {
	ListInventoryLocations(ctx context.Context, params bigcommerce.InventoryLocationListParams) ([]json.RawMessage, error)
	CreateInventoryLocation(ctx context.Context, payload json.RawMessage) (json.RawMessage, error)
	UpdateInventoryLocation(ctx context.Context, locationID int, payload json.RawMessage) (json.RawMessage, error)
	DeleteInventoryLocation(ctx context.Context, locationID int) error
	ListInventoryLocationMetafields(ctx context.Context, locationID int, params bigcommerce.InventoryLocationMetafieldListParams) ([]bigcommerce.Metafield, error)
	CreateInventoryLocationMetafield(ctx context.Context, locationID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	UpdateInventoryLocationMetafield(ctx context.Context, locationID, metafieldID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	DeleteInventoryLocationMetafield(ctx context.Context, locationID, metafieldID int) error
	ListInventoryItems(ctx context.Context, params bigcommerce.InventoryItemListParams) ([]json.RawMessage, error)
	GetInventoryItem(ctx context.Context, variantID int) (json.RawMessage, error)
	ListInventoryLocationItems(ctx context.Context, locationID int, params bigcommerce.InventoryLocationItemListParams) ([]json.RawMessage, error)
	UpdateInventoryLocationItems(ctx context.Context, locationID int, payload json.RawMessage) (json.RawMessage, error)
	CreateInventoryAbsoluteAdjustment(ctx context.Context, payload json.RawMessage) (json.RawMessage, error)
	CreateInventoryRelativeAdjustment(ctx context.Context, payload json.RawMessage) (json.RawMessage, error)
	UpdateInventoryItems(ctx context.Context, payload json.RawMessage) (json.RawMessage, error)
}
