package catalog_test

import (
	"context"
	"encoding/json"
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

type CategoryHandlerSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	cats   *catalog.Categories
	reg    *discovery.Registry
}

func TestCategoryHandlerSuite(t *testing.T) {
	suite.Run(t, new(CategoryHandlerSuite))
}

func (s *CategoryHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.cats = catalog.NewCategories(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/categories", "Categories")
	s.reg.RegisterCategory("catalog/categories/metafields", "Metafields")
	s.cats.RegisterTools(s.reg)
}

func (s *CategoryHandlerSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *CategoryHandlerSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
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

func (s *CategoryHandlerSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

// --- Category List Tests ---

func (s *CategoryHandlerSuite) TestListRequiresFilterOrListAll() {
	result, err := s.callTool("catalog/categories/list", map[string]any{})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *CategoryHandlerSuite) TestListAllReturnsCategories() {
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), gomock.Any()).Return([]bigcommerce.Category{
		{ID: 1, Name: "Electronics", IsVisible: true},
		{ID: 2, Name: "Clothing", IsVisible: true},
	}, nil)

	result, err := s.callTool("catalog/categories/list", map[string]any{
		"list_all": true,
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal(float64(2), data["total"])
}

func (s *CategoryHandlerSuite) TestListByNameFilter() {
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), gomock.Any()).Return([]bigcommerce.Category{
		{ID: 1, Name: "Electronics", IsVisible: true},
	}, nil)

	result, err := s.callTool("catalog/categories/list", map[string]any{
		"name": "Electronics",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal(float64(1), data["total"])
}

// --- Category Get Tests ---

func (s *CategoryHandlerSuite) TestGetReturnsCategory() {
	s.mockBC.EXPECT().GetCategory(gomock.Any(), 42).Return(&bigcommerce.Category{
		ID: 42, Name: "Shoes", IsVisible: true, ParentID: 1,
	}, nil)

	result, err := s.callTool("catalog/categories/get", map[string]any{
		"category_id": float64(42),
	})
	s.NoError(err)
	s.False(result.IsError)
}

func (s *CategoryHandlerSuite) TestGetRequiresCategoryID() {
	result, err := s.callTool("catalog/categories/get", map[string]any{})
	s.NoError(err)
	s.True(result.IsError)
}

// --- Category Create Tests ---

func (s *CategoryHandlerSuite) TestCreatePreview() {
	s.mockBC.EXPECT().GetDefaultTreeID(gomock.Any()).Return(1, nil)

	result, err := s.callTool("catalog/categories/create", map[string]any{
		"name": "Summer Sale",
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("preview", data["status"])
	cat := data["category"].(map[string]any)
	s.Equal("Summer Sale", cat["name"])
}

func (s *CategoryHandlerSuite) TestCreateWithParentNamePreview() {
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), map[string]string{"name": "Electronics"}).
		Return([]bigcommerce.Category{{ID: 10, Name: "Electronics"}}, nil)

	result, err := s.callTool("catalog/categories/create", map[string]any{
		"name":        "Laptops",
		"parent_name": "Electronics",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("preview", data["status"])
	cat := data["category"].(map[string]any)
	s.Equal(float64(10), cat["parent_id"])
}

func (s *CategoryHandlerSuite) TestCreateExecute() {
	s.mockBC.EXPECT().GetDefaultTreeID(gomock.Any()).Return(1, nil)
	s.mockBC.EXPECT().CreateCategory(gomock.Any(), gomock.Any()).Return([]bigcommerce.Category{
		{ID: 99, Name: "Summer Sale", TreeID: 1},
	}, nil)

	result, err := s.callTool("catalog/categories/create", map[string]any{
		"name":      "Summer Sale",
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("created", data["status"])
	cat := data["category"].(map[string]any)
	s.Equal(float64(99), cat["id"])
}

func (s *CategoryHandlerSuite) TestCreateRejectsParentIDAndName() {
	result, err := s.callTool("catalog/categories/create", map[string]any{
		"name":        "Conflict",
		"parent_id":   float64(10),
		"parent_name": "Electronics",
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *CategoryHandlerSuite) TestCreateWithChannelIDResolvesTree() {
	s.mockBC.EXPECT().GetTreeIDForChannel(gomock.Any(), 7).Return(42, nil)
	s.mockBC.EXPECT().CreateCategory(gomock.Any(), gomock.AssignableToTypeOf(bigcommerce.CategoryCreate{})).
		DoAndReturn(func(_ context.Context, payload bigcommerce.CategoryCreate) ([]bigcommerce.Category, error) {
			s.Equal(42, payload.TreeID)
			return []bigcommerce.Category{{ID: 11, Name: payload.Name, TreeID: payload.TreeID}}, nil
		})

	result, err := s.callTool("catalog/categories/create", map[string]any{
		"name":       "EU Promos",
		"channel_id": float64(7),
		"confirmed":  true,
	})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal("created", data["status"])
}

func (s *CategoryHandlerSuite) TestCreateRejectsTreeAndChannelTogether() {
	result, err := s.callTool("catalog/categories/create", map[string]any{
		"name":       "Conflict",
		"tree_id":    float64(1),
		"channel_id": float64(7),
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *CategoryHandlerSuite) TestListChannelIDResolvesTreeFilter() {
	s.mockBC.EXPECT().GetTreeIDForChannel(gomock.Any(), 7).Return(42, nil)
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), gomock.AssignableToTypeOf(map[string]string{})).
		DoAndReturn(func(_ context.Context, params map[string]string) ([]bigcommerce.Category, error) {
			s.Equal("42", params["tree_id:in"])
			return []bigcommerce.Category{{ID: 1, Name: "EU Cat"}}, nil
		})

	result, err := s.callTool("catalog/categories/list", map[string]any{
		"channel_id": float64(7),
	})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal(float64(1), data["total"])
}

// --- Category Bulk Update Tests ---

func (s *CategoryHandlerSuite) TestBulkUpdatePreview() {
	s.mockBC.EXPECT().GetCategoriesByIDs(gomock.Any(), []int{1, 2}).Return([]bigcommerce.Category{
		{ID: 1, Name: "Electronics", IsVisible: true, PageTitle: "Old Title"},
		{ID: 2, Name: "Clothing", IsVisible: true, PageTitle: ""},
	}, nil)

	result, err := s.callTool("catalog/categories/bulk_update", map[string]any{
		"category_ids":   []any{float64(1), float64(2)},
		"set_page_title": "New Title",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("preview", data["status"])
	s.Equal(float64(2), data["categories_count"])
}

func (s *CategoryHandlerSuite) TestBulkUpdateExecute() {
	s.mockBC.EXPECT().GetCategoriesByIDs(gomock.Any(), []int{1}).Return([]bigcommerce.Category{
		{ID: 1, Name: "Electronics"},
	}, nil)
	s.mockBC.EXPECT().BatchUpdateCategories(gomock.Any(), gomock.Any()).Return(&bigcommerce.BatchResult{
		Succeeded: 1,
	}, nil)

	result, err := s.callTool("catalog/categories/bulk_update", map[string]any{
		"category_ids":   []any{float64(1)},
		"set_page_title": "Updated",
		"confirmed":      true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("executed", data["status"])
	s.Equal(float64(1), data["succeeded"])
}

func (s *CategoryHandlerSuite) TestBulkUpdateRequiresSetField() {
	result, err := s.callTool("catalog/categories/bulk_update", map[string]any{
		"category_ids": []any{float64(1)},
	})
	s.NoError(err)
	s.True(result.IsError)
}

// --- Category Delete Tests ---

func (s *CategoryHandlerSuite) TestDeletePreviewNoChildren() {
	s.mockBC.EXPECT().GetCategoriesByIDs(gomock.Any(), []int{42}).Return([]bigcommerce.Category{
		{ID: 42, Name: "Old Category"},
	}, nil)
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), gomock.Any()).Return(nil, nil)

	result, err := s.callTool("catalog/categories/delete", map[string]any{
		"category_id": float64(42),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("preview", data["status"])
}

func (s *CategoryHandlerSuite) TestDeleteBlockedByChildren() {
	s.mockBC.EXPECT().GetCategoriesByIDs(gomock.Any(), []int{42}).Return([]bigcommerce.Category{
		{ID: 42, Name: "Parent"},
	}, nil)
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), gomock.Any()).Return([]bigcommerce.Category{
		{ID: 43, Name: "Child"},
	}, nil)

	result, err := s.callTool("catalog/categories/delete", map[string]any{
		"category_id": float64(42),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("blocked", data["status"])
}

func (s *CategoryHandlerSuite) TestDeleteWithChildrenAcknowledged() {
	s.mockBC.EXPECT().GetCategoriesByIDs(gomock.Any(), []int{42}).Return([]bigcommerce.Category{
		{ID: 42, Name: "Parent"},
	}, nil)
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), gomock.Any()).Return([]bigcommerce.Category{
		{ID: 43, Name: "Child"},
	}, nil)

	result, err := s.callTool("catalog/categories/delete", map[string]any{
		"category_id":      float64(42),
		"include_children": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("preview", data["status"])
}

func (s *CategoryHandlerSuite) TestDeleteExecute() {
	s.mockBC.EXPECT().GetCategoriesByIDs(gomock.Any(), []int{42}).Return([]bigcommerce.Category{
		{ID: 42, Name: "Delete Me"},
	}, nil)
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), gomock.Any()).Return(nil, nil)
	s.mockBC.EXPECT().DeleteCategories(gomock.Any(), []int{42}).Return(nil)

	result, err := s.callTool("catalog/categories/delete", map[string]any{
		"category_id": float64(42),
		"confirmed":   true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("deleted", data["status"])
}

func (s *CategoryHandlerSuite) TestDeleteByName() {
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), map[string]string{"name": "Obsolete"}).
		Return([]bigcommerce.Category{{ID: 99, Name: "Obsolete"}}, nil)
	s.mockBC.EXPECT().GetCategoriesByIDs(gomock.Any(), []int{99}).Return([]bigcommerce.Category{
		{ID: 99, Name: "Obsolete"},
	}, nil)
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), gomock.Any()).Return(nil, nil)

	result, err := s.callTool("catalog/categories/delete", map[string]any{
		"category_name": "Obsolete",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("preview", data["status"])
}

// --- Bulk Delete Tests ---

func (s *CategoryHandlerSuite) TestBulkDeletePreview() {
	s.mockBC.EXPECT().GetCategoriesByIDs(gomock.Any(), []int{1, 2}).Return([]bigcommerce.Category{
		{ID: 1, Name: "A"},
		{ID: 2, Name: "B"},
	}, nil)
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)

	result, err := s.callTool("catalog/categories/bulk_delete", map[string]any{
		"category_ids": []any{float64(1), float64(2)},
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("preview", data["status"])
	s.Equal(float64(2), data["categories_count"])
}

func (s *CategoryHandlerSuite) TestBulkDeleteExecute() {
	s.mockBC.EXPECT().GetCategoriesByIDs(gomock.Any(), []int{1}).Return([]bigcommerce.Category{
		{ID: 1, Name: "A"},
	}, nil)
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), gomock.Any()).Return(nil, nil)
	s.mockBC.EXPECT().DeleteCategories(gomock.Any(), []int{1}).Return(nil)

	result, err := s.callTool("catalog/categories/bulk_delete", map[string]any{
		"category_ids": []any{float64(1)},
		"confirmed":    true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("deleted", data["status"])
}

// --- SEO Audit Tests ---

func (s *CategoryHandlerSuite) TestSEOAuditFindsIssues() {
	s.mockBC.EXPECT().SearchCategories(gomock.Any(), gomock.Any()).Return([]bigcommerce.Category{
		{ID: 1, Name: "No SEO", PageTitle: "", MetaDescription: "", SearchKeywords: ""},
		{ID: 2, Name: "Partial SEO", PageTitle: "Good Title", MetaDescription: "", SearchKeywords: ""},
		{ID: 3, Name: "Full SEO", PageTitle: "Title", MetaDescription: "Desc", SearchKeywords: "kw"},
	}, nil)

	result, err := s.callTool("catalog/categories/seo_audit", map[string]any{})
	s.NoError(err)
	s.False(result.IsError)
}

// --- Metafield Tests ---

func (s *CategoryHandlerSuite) TestMetafieldsListByID() {
	s.mockBC.EXPECT().ListCategoryMetafields(gomock.Any(), 42).Return([]bigcommerce.Metafield{
		{ID: 1, Namespace: "ns", Key: "k", Value: "v"},
	}, nil)

	result, err := s.callTool("catalog/categories/metafields/list", map[string]any{
		"category_id": float64(42),
	})
	s.NoError(err)
	s.False(result.IsError)
}

func (s *CategoryHandlerSuite) TestMetafieldsSetPreview() {
	s.mockBC.EXPECT().ListCategoryMetafields(gomock.Any(), 42).Return(nil, nil)

	result, err := s.callTool("catalog/categories/metafields/set", map[string]any{
		"category_id": float64(42),
		"namespace":   "my_app",
		"key":         "color",
		"value":       "blue",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
}

func (s *CategoryHandlerSuite) TestMetafieldsSetExecuteCreate() {
	s.mockBC.EXPECT().ListCategoryMetafields(gomock.Any(), 42).Return(nil, nil)
	s.mockBC.EXPECT().CreateCategoryMetafield(gomock.Any(), 42, gomock.Any()).Return(&bigcommerce.Metafield{
		ID: 1, Namespace: "my_app", Key: "color", Value: "blue",
	}, nil)

	result, err := s.callTool("catalog/categories/metafields/set", map[string]any{
		"category_id": float64(42),
		"namespace":   "my_app",
		"key":         "color",
		"value":       "blue",
		"confirmed":   true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("created", data["status"])
}
