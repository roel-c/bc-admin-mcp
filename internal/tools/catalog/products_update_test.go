package catalog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type UpdateToolSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestUpdateToolSuite(t *testing.T) {
	suite.Run(t, new(UpdateToolSuite))
}

func (s *UpdateToolSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.prods.RegisterTools(s.reg)
}

func (s *UpdateToolSuite) TearDownTest() { s.ctrl.Finish() }

func (s *UpdateToolSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found", toolPath)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: toolPath, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *UpdateToolSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *UpdateToolSuite) TestUpdatePreviewSingleProduct() {
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{42}).Return([]bigcommerce.Product{
		{ID: 42, Name: "Widget", Price: 19.99, IsVisible: true},
	}, nil)

	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(42)},
		"price":       float64(24.99),
		"is_visible":  false,
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(1), data["total_products"])
	fields := data["fields_updated"].([]any)
	s.Contains(fields, "price")
	s.Contains(fields, "is_visible")
}

func (s *UpdateToolSuite) TestUpdateExecuteSingleProduct() {
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{42}).Return([]bigcommerce.Product{
		{ID: 42, Name: "Widget", Price: 19.99},
	}, nil)

	// Preview first
	s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(42)},
		"price":       float64(24.99),
	})

	s.mockBC.EXPECT().BatchUpdateProducts(gomock.Any(), gomock.Any()).Return(&bigcommerce.BatchResult{
		Succeeded: 1,
	}, nil)

	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(42)},
		"price":       float64(24.99),
		"confirmed":   true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
	s.Equal(float64(1), data["products_updated"])
}

func (s *UpdateToolSuite) TestUpdateByCategoryWithLimit() {
	prods := []bigcommerce.Product{
		{ID: 1, Name: "A", Price: 10},
		{ID: 2, Name: "B", Price: 20},
		{ID: 3, Name: "C", Price: 30},
	}
	s.mockBC.EXPECT().ListProductsByCategory(gomock.Any(), 5, gomock.Any()).Return(prods, nil)

	result, err := s.callTool("catalog/products/update", map[string]any{
		"category_id": float64(5),
		"limit":       float64(2),
		"name":        "Renamed",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(2), data["total_products"])
}

func (s *UpdateToolSuite) TestUpdateNoFieldsError() {
	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(1)},
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *UpdateToolSuite) TestUpdateNoTargetError() {
	result, err := s.callTool("catalog/products/update", map[string]any{
		"price": float64(10),
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *UpdateToolSuite) TestUpdateMultipleTargetModesError() {
	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids":  []any{float64(1)},
		"sku":          "ABC",
		"price":        float64(10),
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *UpdateToolSuite) TestUpdateSEOFields() {
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{10}).Return([]bigcommerce.Product{
		{ID: 10, Name: "SEO Product", PageTitle: "Old"},
	}, nil)

	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids":      []any{float64(10)},
		"page_title":       "New Title",
		"meta_description": "New Description",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	fields := data["fields_updated"].([]any)
	s.Contains(fields, "page_title")
	s.Contains(fields, "meta_description")
}

func (s *UpdateToolSuite) TestUpdatePreviewIncludesChannelAssignmentsBlock() {
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{42}).Return([]bigcommerce.Product{
		{ID: 42, Name: "Widget", Price: 19.99},
	}, nil)

	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(42)},
		"price":       float64(24.99),
		"channel_ids": []any{float64(1), float64(3)},
	})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	preview, ok := data["channel_assignments_preview"].(map[string]any)
	s.Require().True(ok, "channel_assignments_preview missing")
	s.Equal(float64(2), preview["total_pairs"])
}

func (s *UpdateToolSuite) TestUpdateConfirmedAdditiveAssignment() {
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{42}).Return([]bigcommerce.Product{
		{ID: 42, Name: "Widget", Price: 19.99},
	}, nil).Times(1)
	s.mockBC.EXPECT().BatchUpdateProducts(gomock.Any(), gomock.Any()).Return(&bigcommerce.BatchResult{
		Succeeded: 1,
	}, nil)
	s.mockBC.EXPECT().UpsertProductChannelAssignments(gomock.Any(), []bigcommerce.ProductChannelAssignment{
		{ProductID: 42, ChannelID: 7},
	}).Return(nil)

	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(42)},
		"price":       float64(24.99),
		"channel_ids": []any{float64(7)},
		"confirmed":   true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
	ca := data["channel_assignments"].(map[string]any)
	s.Equal("completed", ca["status"])
}

func (s *UpdateToolSuite) TestUpdateChannelOnlyNoFieldsAllowed() {
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{42}).Return([]bigcommerce.Product{
		{ID: 42, Name: "Widget"},
	}, nil)
	s.mockBC.EXPECT().UpsertProductChannelAssignments(gomock.Any(), []bigcommerce.ProductChannelAssignment{
		{ProductID: 42, ChannelID: 1},
	}).Return(nil)

	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(42)},
		"channel_ids": []any{float64(1)},
		"confirmed":   true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
	s.Equal(float64(1), data["products_updated"])
}

func (s *UpdateToolSuite) TestUpdateChannelAssignmentSkippedOnBatchFailure() {
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{42}).Return([]bigcommerce.Product{
		{ID: 42, Name: "Widget"},
	}, nil)
	s.mockBC.EXPECT().BatchUpdateProducts(gomock.Any(), gomock.Any()).Return(&bigcommerce.BatchResult{
		Succeeded: 0,
		Failed:    1,
		Errors:    []bigcommerce.BatchError{{Offset: 0, Count: 1, Err: "boom"}},
	}, nil)
	// Crucially: no UpsertProductChannelAssignments call is expected.

	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(42)},
		"price":       float64(24.99),
		"channel_ids": []any{float64(7)},
		"confirmed":   true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("partial_success", data["status"])
	ca := data["channel_assignments"].(map[string]any)
	s.Equal("skipped", ca["status"])
}

func (s *UpdateToolSuite) TestUpdatePairsCapEnforced() {
	prods := make([]bigcommerce.Product, 26)
	ids := make([]int, 26)
	for i := 0; i < 26; i++ {
		prods[i] = bigcommerce.Product{ID: i + 1, Name: fmt.Sprintf("P%d", i+1)}
		ids[i] = i + 1
	}
	idArgs := make([]any, len(ids))
	for i, id := range ids {
		idArgs[i] = float64(id)
	}
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), ids).Return(prods, nil)

	chArgs := make([]any, 20)
	for i := 0; i < 20; i++ {
		chArgs[i] = float64(i + 1)
	}
	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": idArgs,
		"price":       float64(1),
		"channel_ids": chArgs,
	})
	s.NoError(err)
	s.True(result.IsError)
}

// TestUpdateInterleavedPreviewsConfirmAOnly guards against the
// previously-possible failure mode where two previews shared a single cache
// key and the second preview overwrote the first — so a confirm shaped like
// preview A would have applied A's field changes to B's products.
//
// Under fingerprinted cache keys (UpdateParams.cacheKey) preview A and
// preview B occupy distinct cache slots, so confirm A finds and uses A's
// snapshot. The critical invariant verified here is that BatchUpdateProducts
// for confirm A is called with product 11 (and price=99), never with
// preview B's product 22 or its is_visible field.
func (s *UpdateToolSuite) TestUpdateInterleavedPreviewsConfirmAOnly() {
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{11}).Return([]bigcommerce.Product{
		{ID: 11, Name: "A", Price: 10},
	}, nil)
	resA, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(11)},
		"price":       float64(99),
	})
	s.NoError(err)
	s.False(resA.IsError)

	// Preview B targets a completely different product. Under the legacy
	// single-key cache this overwrote A's snapshot; under fingerprinted keys
	// it does not.
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{22}).Return([]bigcommerce.Product{
		{ID: 22, Name: "B", Price: 20},
	}, nil)
	resB, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(22)},
		"is_visible":  false,
	})
	s.NoError(err)
	s.False(resB.IsError)

	s.mockBC.EXPECT().
		BatchUpdateProducts(gomock.Any(), gomock.AssignableToTypeOf([]bigcommerce.ProductUpdate{})).
		DoAndReturn(func(_ context.Context, updates []bigcommerce.ProductUpdate) (*bigcommerce.BatchResult, error) {
			s.Require().Len(updates, 1, "confirm A must touch exactly one product")
			s.Equal(11, updates[0].ID, "confirm A must update product 11, not preview B's product 22")
			s.Require().NotNil(updates[0].Price, "confirm A must apply the price field")
			s.InEpsilon(99.0, *updates[0].Price, 1e-9)
			s.Nil(updates[0].IsVisible, "confirm A must NOT carry preview B's is_visible field")
			return &bigcommerce.BatchResult{Succeeded: 1}, nil
		})

	resAConfirm, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(11)},
		"price":       float64(99),
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(resAConfirm.IsError)
	data := s.parseJSON(resAConfirm)
	s.Equal("completed", data["status"])
	s.Equal(float64(1), data["products_updated"])
}

// TestUpdateConfirmWithoutMatchingPreviewRefetches verifies the safety net:
// a confirm call whose targeting+fields fingerprint does not match any
// cached preview must NOT silently use some other preview's snapshot. It
// re-fetches and writes only to its own resolved targets.
func (s *UpdateToolSuite) TestUpdateConfirmWithoutMatchingPreviewRefetches() {
	// Seed the cache with a preview for product 11.
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{11}).Return([]bigcommerce.Product{
		{ID: 11, Name: "A", Price: 10},
	}, nil)
	_, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(11)},
		"price":       float64(99),
	})
	s.NoError(err)

	// Confirm a *different* update (product 22, different field). Must miss
	// the cache and fetch product 22 fresh.
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{22}).Return([]bigcommerce.Product{
		{ID: 22, Name: "B", Price: 20},
	}, nil)
	s.mockBC.EXPECT().
		BatchUpdateProducts(gomock.Any(), gomock.AssignableToTypeOf([]bigcommerce.ProductUpdate{})).
		DoAndReturn(func(_ context.Context, updates []bigcommerce.ProductUpdate) (*bigcommerce.BatchResult, error) {
			s.Require().Len(updates, 1)
			s.Equal(22, updates[0].ID, "must update product 22 (current call's targeting), not the cached product 11")
			s.Require().NotNil(updates[0].IsVisible)
			s.False(*updates[0].IsVisible)
			s.Nil(updates[0].Price, "must NOT carry the cached preview's price field")
			return &bigcommerce.BatchResult{Succeeded: 1}, nil
		})

	res, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(22)},
		"is_visible":  false,
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(res.IsError)
}

// TestUpdateConfirmReusesCacheOnMatchingTargeting verifies the happy path:
// when preview and confirm carry identical targeting + fields, confirm
// reuses the cached product snapshot and skips the GetProductsByIDs round
// trip — preserving the original token-saving design.
func (s *UpdateToolSuite) TestUpdateConfirmReusesCacheOnMatchingTargeting() {
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{42}).Return([]bigcommerce.Product{
		{ID: 42, Name: "Widget", Price: 19.99},
	}, nil).Times(1)

	_, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(42)},
		"price":       float64(24.99),
	})
	s.NoError(err)

	s.mockBC.EXPECT().BatchUpdateProducts(gomock.Any(), gomock.Any()).Return(&bigcommerce.BatchResult{
		Succeeded: 1,
	}, nil)

	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(42)},
		"price":       float64(24.99),
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}

// TestUpdateRejectsWrongTypedFields ensures wrong-typed scalar updates fail
// loudly instead of being silently dropped from the BC payload — previously
// extractString/extractFloat/etc. swallowed type mismatches, so an LLM that
// passed price="24.99" got a 200 result that hadn't actually changed price.
func (s *UpdateToolSuite) TestUpdateRejectsWrongTypedPrice() {
	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(42)},
		"price":       "24.99", // string instead of number
	})
	s.NoError(err)
	s.True(result.IsError, "wrong-typed price must surface as a tool error")
	text := result.Content[0].(mcp.TextContent).Text
	s.Contains(text, "price")
}

func (s *UpdateToolSuite) TestUpdateRejectsWrongTypedIsVisible() {
	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(42)},
		"is_visible":  "true",
	})
	s.NoError(err)
	s.True(result.IsError)
	text := result.Content[0].(mcp.TextContent).Text
	s.Contains(text, "is_visible")
}

func (s *UpdateToolSuite) TestUpdateRejectsWrongTypedName() {
	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(42)},
		"name":        float64(123),
	})
	s.NoError(err)
	s.True(result.IsError)
	text := result.Content[0].(mcp.TextContent).Text
	s.Contains(text, "name")
}
