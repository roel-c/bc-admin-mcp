package inventory_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/tools/inventory"
	"github.com/stretchr/testify/suite"
)

type fakeInventoryBC struct {
	listLocationsFn          func(context.Context, bigcommerce.InventoryLocationListParams) ([]json.RawMessage, error)
	createLocationFn         func(context.Context, json.RawMessage) (json.RawMessage, error)
	updateLocationFn         func(context.Context, int, json.RawMessage) (json.RawMessage, error)
	deleteLocationFn         func(context.Context, int) error
	listLocationMetafieldsFn func(context.Context, int, bigcommerce.InventoryLocationMetafieldListParams) ([]bigcommerce.Metafield, error)
	createLocationMFFn       func(context.Context, int, bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	updateLocationMFFn       func(context.Context, int, int, bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	deleteLocationMFFn       func(context.Context, int, int) error
	listItemsFn              func(context.Context, bigcommerce.InventoryItemListParams) ([]json.RawMessage, error)
	getItemFn                func(context.Context, int) (json.RawMessage, error)
	absoluteFn               func(context.Context, json.RawMessage) (json.RawMessage, error)
	relativeFn               func(context.Context, json.RawMessage) (json.RawMessage, error)
	updateItemsFn            func(context.Context, json.RawMessage) (json.RawMessage, error)
}

func (f *fakeInventoryBC) ListInventoryLocations(ctx context.Context, params bigcommerce.InventoryLocationListParams) ([]json.RawMessage, error) {
	if f.listLocationsFn != nil {
		return f.listLocationsFn(ctx, params)
	}
	return nil, nil
}

func (f *fakeInventoryBC) CreateInventoryLocation(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
	if f.createLocationFn != nil {
		return f.createLocationFn(ctx, payload)
	}
	return nil, nil
}

func (f *fakeInventoryBC) UpdateInventoryLocation(ctx context.Context, locationID int, payload json.RawMessage) (json.RawMessage, error) {
	if f.updateLocationFn != nil {
		return f.updateLocationFn(ctx, locationID, payload)
	}
	return nil, nil
}

func (f *fakeInventoryBC) DeleteInventoryLocation(ctx context.Context, locationID int) error {
	if f.deleteLocationFn != nil {
		return f.deleteLocationFn(ctx, locationID)
	}
	return nil
}

func (f *fakeInventoryBC) ListInventoryLocationMetafields(ctx context.Context, locationID int, params bigcommerce.InventoryLocationMetafieldListParams) ([]bigcommerce.Metafield, error) {
	if f.listLocationMetafieldsFn != nil {
		return f.listLocationMetafieldsFn(ctx, locationID, params)
	}
	return nil, nil
}

func (f *fakeInventoryBC) CreateInventoryLocationMetafield(ctx context.Context, locationID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error) {
	if f.createLocationMFFn != nil {
		return f.createLocationMFFn(ctx, locationID, mf)
	}
	return nil, nil
}

func (f *fakeInventoryBC) UpdateInventoryLocationMetafield(ctx context.Context, locationID, metafieldID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error) {
	if f.updateLocationMFFn != nil {
		return f.updateLocationMFFn(ctx, locationID, metafieldID, mf)
	}
	return nil, nil
}

func (f *fakeInventoryBC) DeleteInventoryLocationMetafield(ctx context.Context, locationID, metafieldID int) error {
	if f.deleteLocationMFFn != nil {
		return f.deleteLocationMFFn(ctx, locationID, metafieldID)
	}
	return nil
}

func (f *fakeInventoryBC) ListInventoryItems(ctx context.Context, params bigcommerce.InventoryItemListParams) ([]json.RawMessage, error) {
	if f.listItemsFn != nil {
		return f.listItemsFn(ctx, params)
	}
	return nil, nil
}

func (f *fakeInventoryBC) GetInventoryItem(ctx context.Context, variantID int) (json.RawMessage, error) {
	if f.getItemFn != nil {
		return f.getItemFn(ctx, variantID)
	}
	return nil, nil
}

func (f *fakeInventoryBC) CreateInventoryAbsoluteAdjustment(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
	if f.absoluteFn != nil {
		return f.absoluteFn(ctx, payload)
	}
	return nil, nil
}

func (f *fakeInventoryBC) CreateInventoryRelativeAdjustment(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
	if f.relativeFn != nil {
		return f.relativeFn(ctx, payload)
	}
	return nil, nil
}

func (f *fakeInventoryBC) UpdateInventoryItems(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
	if f.updateItemsFn != nil {
		return f.updateItemsFn(ctx, payload)
	}
	return nil, nil
}

type InventoryToolsSuite struct {
	suite.Suite
	mock *fakeInventoryBC
	reg  *discovery.Registry
}

func TestInventoryToolsSuite(t *testing.T) {
	suite.Run(t, new(InventoryToolsSuite))
}

func (s *InventoryToolsSuite) SetupTest() {
	s.mock = &fakeInventoryBC{}
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("inventory", "Inventory")
	s.reg.RegisterCategory("inventory/locations", "Locations")
	s.reg.RegisterCategory("inventory/locations/metafields", "Location Metafields")
	s.reg.RegisterCategory("inventory/items", "Items")
	s.reg.RegisterCategory("inventory/adjustments", "Adjustments")
	inventory.New(s.mock).RegisterTools(s.reg)
}

func (s *InventoryToolsSuite) callTool(path string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(path)
	s.Require().NotNil(def)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: path, Arguments: args},
	}
	return def.Handler(context.Background(), req)
}

func (s *InventoryToolsSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *InventoryToolsSuite) TestListLocationsPassesPaging() {
	s.mock.listLocationsFn = func(_ context.Context, params bigcommerce.InventoryLocationListParams) ([]json.RawMessage, error) {
		s.Equal(2, params.Page)
		s.Equal(25, params.Limit)
		return []json.RawMessage{json.RawMessage(`{"id":1}`)}, nil
	}

	res, err := s.callTool("inventory/locations/list", map[string]any{
		"page":  float64(2),
		"limit": float64(25),
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *InventoryToolsSuite) TestListItemsRequiresFilterOrListAll() {
	res, err := s.callTool("inventory/items/list", map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(res.Content[0].(mcp.TextContent).Text, "provide at least one filter or set list_all=true")
}

func (s *InventoryToolsSuite) TestCreateLocationPreviewAndConfirmed() {
	createCalls := 0
	s.mock.createLocationFn = func(_ context.Context, payload json.RawMessage) (json.RawMessage, error) {
		createCalls++
		s.Contains(string(payload), `"name":"Warehouse East"`)
		return json.RawMessage(`{"id":9,"name":"Warehouse East"}`), nil
	}

	preview, err := s.callTool("inventory/locations/create", map[string]any{
		"location": map[string]any{
			"name": "Warehouse East",
			"code": "WH-E",
		},
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, createCalls)

	confirmed, err := s.callTool("inventory/locations/create", map[string]any{
		"location": map[string]any{
			"name": "Warehouse East",
			"code": "WH-E",
		},
		"confirmed": true,
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, createCalls)
}

func (s *InventoryToolsSuite) TestUpdateLocationPreviewAndConfirmed() {
	updateCalls := 0
	s.mock.updateLocationFn = func(_ context.Context, locationID int, payload json.RawMessage) (json.RawMessage, error) {
		updateCalls++
		s.Equal(9, locationID)
		s.Contains(string(payload), `"name":"Warehouse East 2"`)
		return json.RawMessage(`{"id":9,"name":"Warehouse East 2"}`), nil
	}

	preview, err := s.callTool("inventory/locations/update", map[string]any{
		"location_id": float64(9),
		"patch": map[string]any{
			"name": "Warehouse East 2",
		},
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, updateCalls)

	confirmed, err := s.callTool("inventory/locations/update", map[string]any{
		"location_id": float64(9),
		"patch": map[string]any{
			"name": "Warehouse East 2",
		},
		"confirmed": true,
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, updateCalls)
}

func (s *InventoryToolsSuite) TestDeleteLocationPreviewAndConfirmed() {
	deleteCalls := 0
	s.mock.deleteLocationFn = func(_ context.Context, locationID int) error {
		deleteCalls++
		s.Equal(9, locationID)
		return nil
	}

	preview, err := s.callTool("inventory/locations/delete", map[string]any{
		"location_id": float64(9),
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, deleteCalls)

	confirmed, err := s.callTool("inventory/locations/delete", map[string]any{
		"location_id": float64(9),
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, deleteCalls)
}

func (s *InventoryToolsSuite) TestListLocationMetafields() {
	s.mock.listLocationMetafieldsFn = func(_ context.Context, locationID int, params bigcommerce.InventoryLocationMetafieldListParams) ([]bigcommerce.Metafield, error) {
		s.Equal(9, locationID)
		s.Equal(1, params.Page)
		s.Equal(25, params.Limit)
		return []bigcommerce.Metafield{
			{ID: 50, Namespace: "ops", Key: "zone", Value: "east"},
		}, nil
	}

	res, err := s.callTool("inventory/locations/metafields/list", map[string]any{
		"location_id": float64(9),
		"page":        float64(1),
		"limit":       float64(25),
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *InventoryToolsSuite) TestSetLocationMetafieldPreviewAndConfirmed() {
	createCalls := 0
	s.mock.listLocationMetafieldsFn = func(_ context.Context, locationID int, _ bigcommerce.InventoryLocationMetafieldListParams) ([]bigcommerce.Metafield, error) {
		s.Equal(9, locationID)
		return []bigcommerce.Metafield{}, nil
	}
	s.mock.createLocationMFFn = func(_ context.Context, locationID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error) {
		createCalls++
		s.Equal(9, locationID)
		s.Equal("ops", mf.Namespace)
		s.Equal("zone", mf.Key)
		s.Equal("east", mf.Value)
		return &bigcommerce.Metafield{ID: 51, Namespace: mf.Namespace, Key: mf.Key, Value: mf.Value, PermissionSet: mf.PermissionSet}, nil
	}

	preview, err := s.callTool("inventory/locations/metafields/set", map[string]any{
		"location_id": float64(9),
		"namespace":   "ops",
		"key":         "zone",
		"value":       "east",
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, createCalls)

	confirmed, err := s.callTool("inventory/locations/metafields/set", map[string]any{
		"location_id": float64(9),
		"namespace":   "ops",
		"key":         "zone",
		"value":       "east",
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, createCalls)
}

func (s *InventoryToolsSuite) TestDeleteLocationMetafieldByNamespaceKeyPreviewAndConfirmed() {
	deleteCalls := 0
	s.mock.listLocationMetafieldsFn = func(_ context.Context, locationID int, _ bigcommerce.InventoryLocationMetafieldListParams) ([]bigcommerce.Metafield, error) {
		s.Equal(9, locationID)
		return []bigcommerce.Metafield{
			{ID: 52, Namespace: "ops", Key: "zone", Value: "east"},
		}, nil
	}
	s.mock.deleteLocationMFFn = func(_ context.Context, locationID, metafieldID int) error {
		deleteCalls++
		s.Equal(9, locationID)
		s.Equal(52, metafieldID)
		return nil
	}

	preview, err := s.callTool("inventory/locations/metafields/delete", map[string]any{
		"location_id": float64(9),
		"namespace":   "ops",
		"key":         "zone",
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, deleteCalls)

	confirmed, err := s.callTool("inventory/locations/metafields/delete", map[string]any{
		"location_id": float64(9),
		"namespace":   "ops",
		"key":         "zone",
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, deleteCalls)
}

func (s *InventoryToolsSuite) TestListItemsPassesFilters() {
	s.mock.listItemsFn = func(_ context.Context, params bigcommerce.InventoryItemListParams) ([]json.RawMessage, error) {
		s.Equal([]int{7}, params.LocationIDs)
		s.Equal([]int{11}, params.ProductIDs)
		s.Equal([]int{44, 45}, params.VariantIDs)
		s.Equal([]string{"SKU-1"}, params.SKUs)
		s.Equal(1, params.Page)
		s.Equal(50, params.Limit)
		return []json.RawMessage{json.RawMessage(`{"variant_id":44}`)}, nil
	}

	res, err := s.callTool("inventory/items/list", map[string]any{
		"location_ids": []any{float64(7)},
		"product_ids":  []any{float64(11)},
		"variant_ids":  []any{float64(44), float64(45)},
		"skus":         []any{"SKU-1"},
		"page":         float64(1),
		"limit":        float64(50),
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *InventoryToolsSuite) TestGetItem() {
	s.mock.getItemFn = func(_ context.Context, variantID int) (json.RawMessage, error) {
		s.Equal(44, variantID)
		return json.RawMessage(`{"variant_id":44,"available_to_sell":7}`), nil
	}

	res, err := s.callTool("inventory/items/get", map[string]any{
		"variant_id": float64(44),
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(44), data["variant_id"])
}

func (s *InventoryToolsSuite) TestAbsoluteAdjustmentPreviewAndConfirmed() {
	absoluteCalls := 0
	s.mock.absoluteFn = func(_ context.Context, payload json.RawMessage) (json.RawMessage, error) {
		absoluteCalls++
		s.Contains(string(payload), `"reason":"cycle_count"`)
		return json.RawMessage(`{"transaction_id":"txn_abs_1"}`), nil
	}

	preview, err := s.callTool("inventory/adjustments/absolute", map[string]any{
		"reason": "cycle_count",
		"items": []any{
			map[string]any{"location_id": float64(1), "variant_id": float64(44), "quantity": float64(10)},
		},
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, absoluteCalls)

	confirmed, err := s.callTool("inventory/adjustments/absolute", map[string]any{
		"reason": "cycle_count",
		"items": []any{
			map[string]any{"location_id": float64(1), "variant_id": float64(44), "quantity": float64(10)},
		},
		"confirmed": true,
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, absoluteCalls)
}

func (s *InventoryToolsSuite) TestRelativeAdjustmentValidatesNonZeroQuantity() {
	res, err := s.callTool("inventory/adjustments/relative", map[string]any{
		"reason": "order_adjustment",
		"items": []any{
			map[string]any{"location_id": float64(1), "variant_id": float64(44), "quantity": float64(0)},
		},
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(res.Content[0].(mcp.TextContent).Text, "must be non-zero")
}

func (s *InventoryToolsSuite) TestRelativeAdjustmentConfirmed() {
	relativeCalls := 0
	s.mock.relativeFn = func(_ context.Context, payload json.RawMessage) (json.RawMessage, error) {
		relativeCalls++
		s.Contains(string(payload), `"reason":"order_adjustment"`)
		return json.RawMessage(`{"transaction_id":"txn_rel_1"}`), nil
	}

	preview, err := s.callTool("inventory/adjustments/relative", map[string]any{
		"reason": "order_adjustment",
		"items": []any{
			map[string]any{"location_id": float64(1), "variant_id": float64(44), "quantity": float64(-1)},
		},
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, relativeCalls)

	confirmed, err := s.callTool("inventory/adjustments/relative", map[string]any{
		"reason": "order_adjustment",
		"items": []any{
			map[string]any{"location_id": float64(1), "variant_id": float64(44), "quantity": float64(-1)},
		},
		"confirmed": true,
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, relativeCalls)
}

func (s *InventoryToolsSuite) TestUpdateItemsBatchPreviewAndConfirmed() {
	updateCalls := 0
	s.mock.updateItemsFn = func(_ context.Context, payload json.RawMessage) (json.RawMessage, error) {
		updateCalls++
		s.Contains(string(payload), `"safety_stock":3`)
		return json.RawMessage(`{"transaction_id":"txn_items_1"}`), nil
	}

	preview, err := s.callTool("inventory/items/update_batch", map[string]any{
		"update": map[string]any{
			"items": []any{
				map[string]any{
					"location_id":        float64(1),
					"variant_id":         float64(44),
					"safety_stock":       float64(3),
					"is_in_stock":        true,
					"bin_picking_number": "A-11",
				},
			},
		},
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, updateCalls)

	confirmed, err := s.callTool("inventory/items/update_batch", map[string]any{
		"update": map[string]any{
			"items": []any{
				map[string]any{
					"location_id":        float64(1),
					"variant_id":         float64(44),
					"safety_stock":       float64(3),
					"is_in_stock":        true,
					"bin_picking_number": "A-11",
				},
			},
		},
		"confirmed": true,
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, updateCalls)
}

func (s *InventoryToolsSuite) TestUpdateItemsBatchValidatesRowCap() {
	items := make([]any, 0, 11)
	for i := 0; i < 11; i++ {
		items = append(items, map[string]any{
			"location_id": float64(1),
			"variant_id":  float64(i + 1),
		})
	}

	res, err := s.callTool("inventory/items/update_batch", map[string]any{
		"update": map[string]any{
			"items": items,
		},
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(res.Content[0].(mcp.TextContent).Text, "maximum of 10 rows")
}
