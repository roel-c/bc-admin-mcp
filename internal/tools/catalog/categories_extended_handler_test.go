package catalog_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type CategoryExtendedSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	cats   *catalog.Categories
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestCategoryExtendedSuite(t *testing.T) {
	suite.Run(t, new(CategoryExtendedSuite))
}

func (s *CategoryExtendedSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.cats = catalog.NewCategories(s.mockBC, s.cache)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/categories", "Categories")
	s.reg.RegisterCategory("catalog/categories/metafields", "Metafields")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.cats.RegisterTools(s.reg)
	s.prods.RegisterTools(s.reg)
}

func (s *CategoryExtendedSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *CategoryExtendedSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found in registry", toolPath)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolPath,
			Arguments: args,
		},
	}
	return def.Handler(context.Background(), req)
}

func (s *CategoryExtendedSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

// --- Move handler tests ---

func (s *CategoryExtendedSuite) TestMovePreview() {
	s.mockBC.EXPECT().GetCategory(gomock.Any(), 10).Return(&bigcommerce.Category{
		ID: 10, Name: "Laptops", ParentID: 5,
	}, nil)
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, params map[string]string) ([]bigcommerce.Category, error) {
			if pid, ok := params["parent_id:in"]; ok {
				switch pid {
				case "20":
					return nil, nil
				case "10":
					return []bigcommerce.Category{{ID: 11, Name: "Gaming Laptops"}}, nil
				case "11":
					return nil, nil
				}
			}
			return nil, nil
		}).AnyTimes()
	s.mockBC.EXPECT().GetCategory(gomock.Any(), 5).Return(&bigcommerce.Category{
		ID: 5, Name: "Electronics",
	}, nil)
	s.mockBC.EXPECT().GetCategory(gomock.Any(), 20).Return(&bigcommerce.Category{
		ID: 20, Name: "Computers",
	}, nil)

	result, err := s.callTool("catalog/categories/move", map[string]any{
		"category_id":   float64(10),
		"new_parent_id": float64(20),
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(1), data["descendants_that_move"])
}

func (s *CategoryExtendedSuite) TestMoveExecute() {
	s.mockBC.EXPECT().GetCategory(gomock.Any(), 10).Return(&bigcommerce.Category{
		ID: 10, Name: "Laptops", ParentID: 5,
	}, nil)
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, params map[string]string) ([]bigcommerce.Category, error) {
			return nil, nil
		}).AnyTimes()
	s.mockBC.EXPECT().BatchUpdateCategories(gomock.Any(), gomock.Any()).Return(&bigcommerce.BatchResult{
		Succeeded: 1,
	}, nil)

	result, err := s.callTool("catalog/categories/move", map[string]any{
		"category_id":   float64(10),
		"new_parent_id": float64(20),
		"confirmed":     true,
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}

func (s *CategoryExtendedSuite) TestMoveSameParentError() {
	s.mockBC.EXPECT().GetCategory(gomock.Any(), 10).Return(&bigcommerce.Category{
		ID: 10, Name: "Laptops", ParentID: 20,
	}, nil)

	result, err := s.callTool("catalog/categories/move", map[string]any{
		"category_id":   float64(10),
		"new_parent_id": float64(20),
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *CategoryExtendedSuite) TestMoveSelfCycleError() {
	s.mockBC.EXPECT().GetCategory(gomock.Any(), 10).Return(&bigcommerce.Category{
		ID: 10, Name: "Laptops", ParentID: 5,
	}, nil)

	result, err := s.callTool("catalog/categories/move", map[string]any{
		"category_id":   float64(10),
		"new_parent_id": float64(10),
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *CategoryExtendedSuite) TestMoveByName() {
	// Use gomock.Any() for SearchCategories since the call ordering depends on
	// name resolution, cycle detection, and descendant counting all using SearchCategories.
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, params map[string]string) ([]bigcommerce.Category, error) {
			if name, ok := params["name"]; ok {
				switch name {
				case "Laptops":
					return []bigcommerce.Category{{ID: 10, Name: "Laptops"}}, nil
				case "Computers":
					return []bigcommerce.Category{{ID: 20, Name: "Computers"}}, nil
				}
			}
			if pid, ok := params["parent_id:in"]; ok {
				switch pid {
				case "20":
					return nil, nil
				case "10":
					return nil, nil
				}
			}
			return nil, nil
		}).AnyTimes()
	s.mockBC.EXPECT().GetCategory(gomock.Any(), 10).Return(&bigcommerce.Category{
		ID: 10, Name: "Laptops", ParentID: 5,
	}, nil)
	s.mockBC.EXPECT().GetCategory(gomock.Any(), 5).Return(&bigcommerce.Category{
		ID: 5, Name: "Electronics",
	}, nil)
	s.mockBC.EXPECT().GetCategory(gomock.Any(), 20).Return(&bigcommerce.Category{
		ID: 20, Name: "Computers",
	}, nil)

	result, err := s.callTool("catalog/categories/move", map[string]any{
		"category_name":   "Laptops",
		"new_parent_name": "Computers",
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
}

// --- Reorder handler tests ---

func (s *CategoryExtendedSuite) TestReorderPreview() {
	s.mockBC.EXPECT().GetCategoriesByIDs(gomock.Any(), []int{3, 1, 2}).Return([]bigcommerce.Category{
		{ID: 1, Name: "Alpha", ParentID: 10, SortOrder: 0},
		{ID: 2, Name: "Beta", ParentID: 10, SortOrder: 10},
		{ID: 3, Name: "Gamma", ParentID: 10, SortOrder: 20},
	}, nil)
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), map[string]string{"parent_id:in": "10"}).
		Return([]bigcommerce.Category{
			{ID: 1, Name: "Alpha", ParentID: 10, SortOrder: 0},
			{ID: 2, Name: "Beta", ParentID: 10, SortOrder: 10},
			{ID: 3, Name: "Gamma", ParentID: 10, SortOrder: 20},
		}, nil)

	result, err := s.callTool("catalog/categories/reorder", map[string]any{
		"category_ids": []any{float64(3), float64(1), float64(2)},
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	changes := data["changes"].([]any)
	s.Len(changes, 3)
	// Verify the order is 3→0, 1→10, 2→20
	first := changes[0].(map[string]any)
	s.Equal(float64(3), first["id"])
	s.Equal(float64(0), first["new_sort_order"])
}

func (s *CategoryExtendedSuite) TestReorderExecute() {
	s.mockBC.EXPECT().GetCategoriesByIDs(gomock.Any(), []int{2, 1}).Return([]bigcommerce.Category{
		{ID: 1, Name: "Alpha", ParentID: 10, SortOrder: 0},
		{ID: 2, Name: "Beta", ParentID: 10, SortOrder: 10},
	}, nil)
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), map[string]string{"parent_id:in": "10"}).
		Return([]bigcommerce.Category{
			{ID: 1, Name: "Alpha", ParentID: 10},
			{ID: 2, Name: "Beta", ParentID: 10},
		}, nil)
	s.mockBC.EXPECT().BatchUpdateCategories(gomock.Any(), gomock.Any()).Return(&bigcommerce.BatchResult{
		Succeeded: 2,
	}, nil)

	result, err := s.callTool("catalog/categories/reorder", map[string]any{
		"category_ids": []any{float64(2), float64(1)},
		"confirmed":    true,
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
	s.Equal(float64(2), data["succeeded"])
}

func (s *CategoryExtendedSuite) TestReorderMismatchedParentsError() {
	s.mockBC.EXPECT().GetCategoriesByIDs(gomock.Any(), []int{1, 2}).Return([]bigcommerce.Category{
		{ID: 1, Name: "Alpha", ParentID: 10},
		{ID: 2, Name: "Beta", ParentID: 99},
	}, nil)

	result, err := s.callTool("catalog/categories/reorder", map[string]any{
		"category_ids": []any{float64(1), float64(2)},
	})
	s.NoError(err)
	s.True(result.IsError)
}

// --- Category Products handler tests ---

func (s *CategoryExtendedSuite) TestCategoryProductsByID() {
	s.mockBC.EXPECT().GetCategory(gomock.Any(), 5).Return(&bigcommerce.Category{
		ID: 5, Name: "Electronics",
	}, nil)
	s.mockBC.EXPECT().ListProductsByCategory(gomock.Any(), 5, gomock.Any()).Return([]bigcommerce.Product{
		{ID: 100, Name: "Laptop", Price: 999.99, SKU: "LAP-01"},
		{ID: 101, Name: "Tablet", Price: 499.99, SKU: "TAB-01"},
	}, nil)

	result, err := s.callTool("catalog/categories/products", map[string]any{
		"category_id": float64(5),
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal(float64(5), data["category_id"])
	s.Equal("Electronics", data["category_name"])
	s.Equal(float64(2), data["total_products"])
}

func (s *CategoryExtendedSuite) TestCategoryProductsByName() {
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), map[string]string{"name": "Electronics"}).
		Return([]bigcommerce.Category{{ID: 5, Name: "Electronics"}}, nil)
	s.mockBC.EXPECT().ListProductsByCategory(gomock.Any(), 5, gomock.Any()).Return([]bigcommerce.Product{
		{ID: 100, Name: "Laptop", Price: 999.99},
	}, nil)

	result, err := s.callTool("catalog/categories/products", map[string]any{
		"category_name": "Electronics",
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal(float64(5), data["category_id"])
	s.Equal(float64(1), data["total_products"])
}

func (s *CategoryExtendedSuite) TestCategoryProductsWithLimit() {
	s.mockBC.EXPECT().GetCategory(gomock.Any(), 5).Return(&bigcommerce.Category{
		ID: 5, Name: "Electronics",
	}, nil)
	s.mockBC.EXPECT().ListProductsByCategory(gomock.Any(), 5, gomock.Any()).Return([]bigcommerce.Product{
		{ID: 100, Name: "A"}, {ID: 101, Name: "B"}, {ID: 102, Name: "C"},
	}, nil)

	result, err := s.callTool("catalog/categories/products", map[string]any{
		"category_id": float64(5),
		"limit":       float64(2),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal(float64(2), data["total_products"])
}

// --- Assign Categories handler tests ---

func (s *CategoryExtendedSuite) TestAssignCategoriesPreview() {
	result, err := s.callTool("catalog/products/assign_categories", map[string]any{
		"product_ids":  []any{float64(1), float64(2)},
		"category_ids": []any{float64(10), float64(20)},
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(4), data["total_assignments"])
	s.Equal(float64(2), data["product_count"])
	s.Equal(float64(2), data["category_count"])
}

func (s *CategoryExtendedSuite) TestAssignCategoriesExecute() {
	s.mockBC.EXPECT().UpsertCategoryAssignments(gomock.Any(), gomock.Any()).Return(nil)

	result, err := s.callTool("catalog/products/assign_categories", map[string]any{
		"product_ids":  []any{float64(1)},
		"category_ids": []any{float64(10)},
		"confirmed":    true,
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}

func (s *CategoryExtendedSuite) TestAssignCategoriesMissingProductIDs() {
	result, err := s.callTool("catalog/products/assign_categories", map[string]any{
		"category_ids": []any{float64(10)},
	})
	s.NoError(err)
	s.True(result.IsError)
}

// --- Metafield Delete handler tests ---

func (s *CategoryExtendedSuite) TestMetafieldDeleteByIDPreview() {
	result, err := s.callTool("catalog/categories/metafields/delete", map[string]any{
		"category_id":  float64(42),
		"metafield_id": float64(7),
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(7), data["metafield_id"])
	s.Equal(float64(42), data["category_id"])
}

func (s *CategoryExtendedSuite) TestMetafieldDeleteByIDExecute() {
	s.mockBC.EXPECT().DeleteCategoryMetafield(gomock.Any(), 42, 7).Return(nil)

	result, err := s.callTool("catalog/categories/metafields/delete", map[string]any{
		"category_id":  float64(42),
		"metafield_id": float64(7),
		"confirmed":    true,
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("deleted", data["status"])
}

func (s *CategoryExtendedSuite) TestMetafieldDeleteByNamespaceKeyPreview() {
	s.mockBC.EXPECT().ListCategoryMetafields(gomock.Any(), 42).Return([]bigcommerce.Metafield{
		{ID: 7, Namespace: "my_app", Key: "color", Value: "blue"},
	}, nil)

	result, err := s.callTool("catalog/categories/metafields/delete", map[string]any{
		"category_id": float64(42),
		"namespace":   "my_app",
		"key":         "color",
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(7), data["metafield_id"])
}

func (s *CategoryExtendedSuite) TestMetafieldDeleteByNamespaceKeyExecute() {
	s.mockBC.EXPECT().ListCategoryMetafields(gomock.Any(), 42).Return([]bigcommerce.Metafield{
		{ID: 7, Namespace: "my_app", Key: "color", Value: "blue"},
	}, nil)
	s.mockBC.EXPECT().DeleteCategoryMetafield(gomock.Any(), 42, 7).Return(nil)

	result, err := s.callTool("catalog/categories/metafields/delete", map[string]any{
		"category_id": float64(42),
		"namespace":   "my_app",
		"key":         "color",
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("deleted", data["status"])
}

func (s *CategoryExtendedSuite) TestMetafieldDeleteNotFound() {
	s.mockBC.EXPECT().ListCategoryMetafields(gomock.Any(), 42).Return(nil, nil)

	result, err := s.callTool("catalog/categories/metafields/delete", map[string]any{
		"category_id": float64(42),
		"namespace":   "my_app",
		"key":         "missing",
	})
	s.NoError(err)
	s.True(result.IsError)
}
