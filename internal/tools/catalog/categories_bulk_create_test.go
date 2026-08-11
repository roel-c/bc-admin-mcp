package catalog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

type CategoryBulkCreateSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	cats   *catalog.Categories
	reg    *discovery.Registry
}

func TestCategoryBulkCreateSuite(t *testing.T) {
	suite.Run(t, new(CategoryBulkCreateSuite))
}

func (s *CategoryBulkCreateSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.cats = catalog.NewCategories(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/categories", "Categories")
	s.cats.RegisterTools(s.reg)
}

func (s *CategoryBulkCreateSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *CategoryBulkCreateSuite) callTool(args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool("catalog/categories/bulk_create")
	s.Require().NotNil(def, "bulk_create tool not registered")
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "catalog/categories/bulk_create",
			Arguments: args,
		},
	}
	return def.Handler(context.Background(), req)
}

func (s *CategoryBulkCreateSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *CategoryBulkCreateSuite) TestBulkCreateRejectsMissingJSON() {
	result, err := s.callTool(map[string]any{})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *CategoryBulkCreateSuite) TestBulkCreateRejectsInvalidJSON() {
	result, err := s.callTool(map[string]any{
		"categories_json": "{not-json",
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *CategoryBulkCreateSuite) TestBulkCreateRejectsParentConflict() {
	result, err := s.callTool(map[string]any{
		"categories_json": `[{"name":"Laptops","parent_id":10,"parent_name":"Electronics"}]`,
	})
	s.NoError(err)
	s.True(result.IsError)
	s.Contains(result.Content[0].(mcp.TextContent).Text, "mutually exclusive")
}

func (s *CategoryBulkCreateSuite) TestBulkCreateRejectsTooManyNodes() {
	nodes := make([]map[string]string, 0, 101)
	for i := 0; i < 101; i++ {
		nodes = append(nodes, map[string]string{"name": fmt.Sprintf("Cat %d", i)})
	}
	raw, err := json.Marshal(nodes)
	s.Require().NoError(err)

	result, err := s.callTool(map[string]any{
		"categories_json": string(raw),
	})
	s.NoError(err)
	s.True(result.IsError)
	s.Contains(result.Content[0].(mcp.TextContent).Text, "maximum 100")
}

func (s *CategoryBulkCreateSuite) TestBulkCreatePreviewNestedTree() {
	s.mockBC.EXPECT().GetDefaultTreeID(gomock.Any()).Return(1, nil)

	payload := `[{
		"name":"Men",
		"children":[{
			"name":"Shoes",
			"children":[{"name":"All Shoes"},{"name":"Basketball"}]
		}]
	}]`
	result, err := s.callTool(map[string]any{
		"categories_json": payload,
	})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal("preview", data["status"])
	s.Equal(float64(4), data["total"])
	s.Equal(float64(3), data["max_depth"])
	cats := data["categories"].([]any)
	s.Len(cats, 4)
	byName := map[string]map[string]any{}
	for _, raw := range cats {
		row := raw.(map[string]any)
		byName[row["name"].(string)] = row
	}
	s.Equal("/men/", byName["Men"]["url_path"])
	s.Equal("/men/shoes/", byName["Shoes"]["url_path"])
	s.Equal("/men/shoes/all-shoes/", byName["All Shoes"]["url_path"])
	s.Equal("/men/shoes/basketball/", byName["Basketball"]["url_path"])
}

func (s *CategoryBulkCreateSuite) TestBulkCreatePreviewDistinctURLsForSameName() {
	s.mockBC.EXPECT().GetDefaultTreeID(gomock.Any()).Return(1, nil)

	payload := `[{
		"name":"mcp-url-smoke-root",
		"children":[
			{"name":"Shoes","children":[{"name":"Basketball"}]},
			{"name":"Shop By Sport","children":[{"name":"Basketball"}]}
		]
	}]`
	result, err := s.callTool(map[string]any{
		"categories_json": payload,
	})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	cats := data["categories"].([]any)
	urls := []string{}
	for _, raw := range cats {
		row := raw.(map[string]any)
		if row["name"].(string) == "Basketball" {
			urls = append(urls, row["url_path"].(string))
		}
	}
	s.Len(urls, 2)
	s.Equal("/mcp-url-smoke-root/shoes/basketball/", urls[0])
	s.Equal("/mcp-url-smoke-root/shop-by-sport/basketball/", urls[1])
	s.NotEqual(urls[0], urls[1])
}

func (s *CategoryBulkCreateSuite) TestBulkCreatePreviewWithChannelID() {
	s.mockBC.EXPECT().GetTreeIDForChannel(gomock.Any(), 7).Return(42, nil)

	result, err := s.callTool(map[string]any{
		"categories_json": `[{"name":"Men"},{"name":"Women"}]`,
		"channel_id":      float64(7),
	})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal("preview", data["status"])
	s.Equal(float64(42), data["root_tree_id"])
	s.Equal(float64(7), data["channel_id"])
}

func (s *CategoryBulkCreateSuite) TestBulkCreateExecuteLevelByLevel() {
	s.mockBC.EXPECT().GetTreeIDForChannel(gomock.Any(), 1).Return(1, nil)

	s.mockBC.EXPECT().CreateCategories(gomock.Any(), gomock.AssignableToTypeOf([]bigcommerce.CategoryCreate{})).
		DoAndReturn(func(_ context.Context, payloads []bigcommerce.CategoryCreate) ([]bigcommerce.Category, error) {
			s.Require().Len(payloads, 1)
			s.Equal("Men", payloads[0].Name)
			s.Equal(1, payloads[0].TreeID)
			s.Equal(0, payloads[0].ParentID)
			s.Require().NotNil(payloads[0].URL)
			s.Equal("/men/", payloads[0].URL.Path)
			s.True(payloads[0].URL.IsCustomized)
			return []bigcommerce.Category{{ID: 100, Name: "Men", TreeID: 1, ParentID: 0}}, nil
		})
	s.mockBC.EXPECT().CreateCategories(gomock.Any(), gomock.AssignableToTypeOf([]bigcommerce.CategoryCreate{})).
		DoAndReturn(func(_ context.Context, payloads []bigcommerce.CategoryCreate) ([]bigcommerce.Category, error) {
			s.Require().Len(payloads, 1)
			s.Equal("Shoes", payloads[0].Name)
			s.Equal(100, payloads[0].ParentID)
			s.Equal(0, payloads[0].TreeID)
			s.Require().NotNil(payloads[0].URL)
			s.Equal("/men/shoes/", payloads[0].URL.Path)
			return []bigcommerce.Category{{ID: 101, Name: "Shoes", ParentID: 100, TreeID: 1}}, nil
		})
	s.mockBC.EXPECT().CreateCategories(gomock.Any(), gomock.AssignableToTypeOf([]bigcommerce.CategoryCreate{})).
		DoAndReturn(func(_ context.Context, payloads []bigcommerce.CategoryCreate) ([]bigcommerce.Category, error) {
			s.Require().Len(payloads, 2)
			s.Equal("All Shoes", payloads[0].Name)
			s.Equal("Basketball", payloads[1].Name)
			s.Equal(101, payloads[0].ParentID)
			s.Equal(101, payloads[1].ParentID)
			s.Require().NotNil(payloads[0].URL)
			s.Require().NotNil(payloads[1].URL)
			s.Equal("/men/shoes/all-shoes/", payloads[0].URL.Path)
			s.Equal("/men/shoes/basketball/", payloads[1].URL.Path)
			return []bigcommerce.Category{
				{ID: 102, Name: "All Shoes", ParentID: 101, TreeID: 1},
				{ID: 103, Name: "Basketball", ParentID: 101, TreeID: 1},
			}, nil
		})

	payload := `[{
		"name":"Men",
		"children":[{
			"name":"Shoes",
			"children":[{"name":"All Shoes"},{"name":"Basketball"}]
		}]
	}]`
	result, err := s.callTool(map[string]any{
		"categories_json": payload,
		"channel_id":      float64(1),
		"confirmed":       true,
	})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal("created", data["status"])
	s.Equal(float64(4), data["created"])
	s.Equal(float64(4), data["requested"])
}

func (s *CategoryBulkCreateSuite) TestBulkCreateExecuteDuplicateNamesDistinctURLs() {
	s.mockBC.EXPECT().GetDefaultTreeID(gomock.Any()).Return(1, nil)
	s.mockBC.EXPECT().CreateCategories(gomock.Any(), gomock.Any()).
		Return([]bigcommerce.Category{{ID: 1, Name: "Root", TreeID: 1}}, nil)
	s.mockBC.EXPECT().CreateCategories(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, payloads []bigcommerce.CategoryCreate) ([]bigcommerce.Category, error) {
			s.Require().Len(payloads, 2)
			return []bigcommerce.Category{
				{ID: 2, Name: "Shoes", ParentID: 1},
				{ID: 3, Name: "Sport", ParentID: 1},
			}, nil
		})
	s.mockBC.EXPECT().CreateCategories(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, payloads []bigcommerce.CategoryCreate) ([]bigcommerce.Category, error) {
			s.Require().Len(payloads, 2)
			s.Equal("Basketball", payloads[0].Name)
			s.Equal("Basketball", payloads[1].Name)
			s.Require().NotNil(payloads[0].URL)
			s.Require().NotNil(payloads[1].URL)
			s.Equal("/root/shoes/basketball/", payloads[0].URL.Path)
			s.Equal("/root/sport/basketball/", payloads[1].URL.Path)
			s.NotEqual(payloads[0].URL.Path, payloads[1].URL.Path)
			return []bigcommerce.Category{
				{ID: 4, Name: "Basketball", ParentID: 2},
				{ID: 5, Name: "Basketball", ParentID: 3},
			}, nil
		})

	payload := `[{
		"name":"Root",
		"children":[
			{"name":"Shoes","children":[{"name":"Basketball"}]},
			{"name":"Sport","children":[{"name":"Basketball"}]}
		]
	}]`
	result, err := s.callTool(map[string]any{
		"categories_json": payload,
		"confirmed":       true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("created", data["status"])
	s.Equal(float64(5), data["created"])
}

func (s *CategoryBulkCreateSuite) TestBulkCreateRejectsSiblingNameDuplicate() {
	result, err := s.callTool(map[string]any{
		"categories_json": `[{"name":"Men","children":[{"name":"Shoes"},{"name":"Shoes"}]}]`,
	})
	s.NoError(err)
	s.True(result.IsError)
	s.Contains(result.Content[0].(mcp.TextContent).Text, "duplicate sibling")
}

func (s *CategoryBulkCreateSuite) TestBulkCreateRejectsDuplicateURLPathOverride() {
	result, err := s.callTool(map[string]any{
		"categories_json": `[{"name":"A","url_path":"/same/"},{"name":"B","url_path":"/same/"}]`,
	})
	s.NoError(err)
	s.True(result.IsError)
	s.Contains(result.Content[0].(mcp.TextContent).Text, "duplicate url_path")
}

func (s *CategoryBulkCreateSuite) TestSlugifyCategorySegment() {
	s.Equal("sandals-and-slides", catalog.SlugifyCategorySegment("Sandals & Slides"))
	s.Equal("new-and-featured", catalog.SlugifyCategorySegment("New & Featured"))
	s.Equal("extra-25-off-select-styles", catalog.SlugifyCategorySegment("Extra 25% Off Select Styles"))
}

func (s *CategoryBulkCreateSuite) TestCategoryURLPathFromDisplayPath() {
	s.Equal("/men/shoes/basketball/", catalog.CategoryURLPathFromDisplayPath("Men > Shoes > Basketball"))
	s.Equal("/men/shop-by-sport/basketball/", catalog.CategoryURLPathFromDisplayPath("Men > Shop By Sport > Basketball"))
}

func (s *CategoryBulkCreateSuite) TestNormalizeCategoryURLPath() {
	p, err := catalog.NormalizeCategoryURLPath("men/shoes")
	s.NoError(err)
	s.Equal("/men/shoes/", p)
	_, err = catalog.NormalizeCategoryURLPath("/")
	s.Error(err)
}

func (s *CategoryBulkCreateSuite) TestBulkCreatePartialSuccess() {
	s.mockBC.EXPECT().GetDefaultTreeID(gomock.Any()).Return(1, nil)
	s.mockBC.EXPECT().CreateCategories(gomock.Any(), gomock.Any()).
		Return([]bigcommerce.Category{{ID: 10, Name: "Men", TreeID: 1}}, nil)
	s.mockBC.EXPECT().CreateCategories(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("boom"))

	payload := `[{"name":"Men","children":[{"name":"Shoes"}]}]`
	result, err := s.callTool(map[string]any{
		"categories_json": payload,
		"confirmed":       true,
	})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal("partial_success", data["status"])
	s.Equal(float64(1), data["created"])
	s.Equal(float64(2), data["requested"])
	s.Contains(data, "errors")
}

func (s *CategoryBulkCreateSuite) TestBulkCreateFlatParentRef() {
	s.mockBC.EXPECT().GetDefaultTreeID(gomock.Any()).Return(1, nil)
	s.mockBC.EXPECT().CreateCategories(gomock.Any(), gomock.Any()).
		Return([]bigcommerce.Category{{ID: 1, Name: "Root", TreeID: 1}}, nil)
	s.mockBC.EXPECT().CreateCategories(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, payloads []bigcommerce.CategoryCreate) ([]bigcommerce.Category, error) {
			s.Equal("Child", payloads[0].Name)
			s.Equal(1, payloads[0].ParentID)
			return []bigcommerce.Category{{ID: 2, Name: "Child", ParentID: 1}}, nil
		})

	result, err := s.callTool(map[string]any{
		"categories_json": `[{"ref":"root","name":"Root"},{"ref":"child","name":"Child","parent_ref":"root"}]`,
		"confirmed":       true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("created", data["status"])
	s.Equal(float64(2), data["created"])
}

func (s *CategoryBulkCreateSuite) TestParseBulkCategoryCreateParamsNested() {
	params, err := catalog.ParseBulkCategoryCreateParams(map[string]any{
		"categories_json": `[{"name":"A","children":[{"name":"B"}]}]`,
	})
	s.NoError(err)
	s.Len(params.Nodes, 2)
	s.Equal("A", params.Nodes[0].Name)
	s.Equal("B", params.Nodes[1].Name)
	s.Equal(params.Nodes[0].Ref, params.Nodes[1].ParentRef)
	s.True(strings.Contains(params.Nodes[1].Path, "A > B"))
	s.Equal("/a/", params.Nodes[0].URLPath)
	s.Equal("/a/b/", params.Nodes[1].URLPath)
}
