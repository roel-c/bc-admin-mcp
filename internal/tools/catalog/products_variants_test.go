package catalog_test

import (
	"context"
	"encoding/json"
	"fmt"
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

type VariantToolSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestVariantToolSuite(t *testing.T) {
	suite.Run(t, new(VariantToolSuite))
}

func (s *VariantToolSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.reg.RegisterCategory("catalog/products/variants", "Variants")
	s.prods.RegisterTools(s.reg)
	s.prods.RegisterVariantTools(s.reg)
}

func (s *VariantToolSuite) TearDownTest() { s.ctrl.Finish() }

func (s *VariantToolSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found", toolPath)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: toolPath, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *VariantToolSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *VariantToolSuite) TestVariantList() {
	s.mockBC.EXPECT().ListVariantsForProduct(gomock.Any(), 1).Return([]bigcommerce.Variant{
		{ID: 100, ProductID: 1, SKU: "V1", Price: 19.99},
		{ID: 101, ProductID: 1, SKU: "V2", Price: 24.99},
	}, nil)

	result, err := s.callTool("catalog/products/variants/list", map[string]any{
		"product_id": float64(1),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal(float64(2), data["total_variants"])
}

func (s *VariantToolSuite) TestVariantCreatePreview() {
	s.mockBC.EXPECT().ListProductOptions(gomock.Any(), 1).Return([]bigcommerce.ProductOption{
		{
			ID: 10, DisplayName: "Size",
			OptionValues: []bigcommerce.ProductOptionValue{{ID: 100, Label: "Large"}},
		},
	}, nil)

	result, err := s.callTool("catalog/products/variants/create", map[string]any{
		"product_id": float64(1),
		"sku":        "NEW-V",
		"price":      float64(29.99),
		"option_values": []any{
			map[string]any{"option_display_name": "Size", "label": "Large"},
		},
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	payload := data["payload"].(map[string]any)
	ovs := payload["option_values"].([]any)
	s.Require().Len(ovs, 1)
	ov := ovs[0].(map[string]any)
	s.Equal(float64(100), ov["id"])
	s.Equal(float64(10), ov["option_id"])
}

func (s *VariantToolSuite) TestVariantCreateExecute() {
	price := float64(29.99)
	s.mockBC.EXPECT().ListProductOptions(gomock.Any(), 1).Return([]bigcommerce.ProductOption{
		{
			ID: 10, DisplayName: "Size",
			OptionValues: []bigcommerce.ProductOptionValue{{ID: 100, Label: "Large"}},
		},
	}, nil)
	s.mockBC.EXPECT().CreateVariant(gomock.Any(), 1, gomock.Any()).DoAndReturn(
		func(_ any, _ int, payload bigcommerce.ProductVariantCreate) (*bigcommerce.ProductVariantFull, error) {
			s.Require().Len(payload.OptionValues, 1)
			s.Equal(100, payload.OptionValues[0].ID)
			s.Equal(10, payload.OptionValues[0].OptionID)
			return &bigcommerce.ProductVariantFull{ID: 200, ProductID: 1, SKU: "NEW-V", Price: &price}, nil
		},
	)

	result, err := s.callTool("catalog/products/variants/create", map[string]any{
		"product_id": float64(1),
		"sku":        "NEW-V",
		"price":      float64(29.99),
		"option_values": []any{
			map[string]any{"option_display_name": "Size", "label": "Large"},
		},
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}

func (s *VariantToolSuite) TestVariantUpdatePreview() {
	price := float64(19.99)
	s.mockBC.EXPECT().GetVariant(gomock.Any(), 1, 100).Return(&bigcommerce.ProductVariantFull{
		ID: 100, ProductID: 1, SKU: "V1", Price: &price,
	}, nil)

	result, err := s.callTool("catalog/products/variants/update", map[string]any{
		"product_id": float64(1),
		"variant_id": float64(100),
		"price":      float64(24.99),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
}

func (s *VariantToolSuite) TestVariantUpdateExecute() {
	newPrice := float64(24.99)
	s.mockBC.EXPECT().UpdateVariant(gomock.Any(), 1, 100, gomock.Any()).Return(&bigcommerce.ProductVariantFull{
		ID: 100, ProductID: 1, Price: &newPrice,
	}, nil)

	result, err := s.callTool("catalog/products/variants/update", map[string]any{
		"product_id": float64(1),
		"variant_id": float64(100),
		"price":      float64(24.99),
		"confirmed":  true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}

func (s *VariantToolSuite) TestVariantDeletePreview() {
	price := float64(19.99)
	s.mockBC.EXPECT().GetVariant(gomock.Any(), 1, 100).Return(&bigcommerce.ProductVariantFull{
		ID: 100, ProductID: 1, SKU: "V1", Price: &price,
	}, nil)

	result, err := s.callTool("catalog/products/variants/delete", map[string]any{
		"product_id": float64(1),
		"variant_id": float64(100),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
}

func (s *VariantToolSuite) TestVariantDeleteExecute() {
	s.mockBC.EXPECT().DeleteVariant(gomock.Any(), 1, 100).Return(nil)

	result, err := s.callTool("catalog/products/variants/delete", map[string]any{
		"product_id": float64(1),
		"variant_id": float64(100),
		"confirmed":  true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}

func (s *VariantToolSuite) TestVariantCreateOptionValueIDAlias() {
	// option_value_id should be accepted as an alias for id in option_values.
	price := float64(4.75)
	s.mockBC.EXPECT().CreateVariant(gomock.Any(), 472, gomock.Any()).DoAndReturn(
		func(_ any, _ int, payload bigcommerce.ProductVariantCreate) (*bigcommerce.ProductVariantFull, error) {
			s.Require().Len(payload.OptionValues, 1)
			s.Equal(818, payload.OptionValues[0].ID)
			s.Equal(335, payload.OptionValues[0].OptionID)
			s.Equal("Download", payload.OptionValues[0].Label)
			return &bigcommerce.ProductVariantFull{ID: 1617, ProductID: 472, SKU: "30108448", Price: &price}, nil
		},
	)

	result, err := s.callTool("catalog/products/variants/create", map[string]any{
		"product_id": float64(472),
		"sku":        "30108448",
		"price":      float64(4.75),
		"option_values": []any{
			map[string]any{"option_id": float64(335), "option_value_id": float64(818), "label": "Download"},
		},
		"confirmed": true,
	})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}

func (s *VariantToolSuite) TestVariantUpdateNoFieldsError() {
	result, err := s.callTool("catalog/products/variants/update", map[string]any{
		"product_id": float64(1),
		"variant_id": float64(100),
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *VariantToolSuite) TestVariantCreateSkipsListWhenIDsPresent() {
	price := float64(4.75)
	// IDs already supplied — ListProductOptions must not be called.
	s.mockBC.EXPECT().CreateVariant(gomock.Any(), 472, gomock.Any()).Return(
		&bigcommerce.ProductVariantFull{ID: 1617, ProductID: 472, SKU: "30108448", Price: &price}, nil,
	)

	result, err := s.callTool("catalog/products/variants/create", map[string]any{
		"product_id": float64(472),
		"sku":        "30108448",
		"option_values": []any{
			map[string]any{"option_id": float64(335), "id": float64(818), "label": "Download"},
		},
		"confirmed": true,
	})
	s.NoError(err)
	s.False(result.IsError)
}

func (s *VariantToolSuite) TestVariantCreateUnknownOptionName() {
	s.mockBC.EXPECT().ListProductOptions(gomock.Any(), 1).Return([]bigcommerce.ProductOption{
		{ID: 10, DisplayName: "Size", OptionValues: []bigcommerce.ProductOptionValue{{ID: 100, Label: "Large"}}},
	}, nil)

	result, err := s.callTool("catalog/products/variants/create", map[string]any{
		"product_id": float64(1),
		"sku":        "X",
		"option_values": []any{
			map[string]any{"option_display_name": "Color", "label": "Red"},
		},
		"confirmed": true,
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *VariantToolSuite) TestVariantCreateBatchPreview() {
	s.mockBC.EXPECT().ListProductOptions(gomock.Any(), 1).Return([]bigcommerce.ProductOption{
		{
			ID: 10, DisplayName: "Size",
			OptionValues: []bigcommerce.ProductOptionValue{
				{ID: 100, Label: "Small"},
				{ID: 101, Label: "Large"},
			},
		},
	}, nil)

	result, err := s.callTool("catalog/products/variants/create_batch", map[string]any{
		"product_id": float64(1),
		"variants": []any{
			map[string]any{
				"sku": "V-S",
				"option_values": []any{
					map[string]any{"option_display_name": "Size", "label": "Small"},
				},
			},
			map[string]any{
				"sku": "V-L",
				"option_values": []any{
					map[string]any{"option_display_name": "size", "label": "large"},
				},
			},
		},
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(2), data["variant_count"])
}

func (s *VariantToolSuite) TestVariantCreateBatchExecute() {
	s.mockBC.EXPECT().ListProductOptions(gomock.Any(), 1).Return([]bigcommerce.ProductOption{
		{
			ID: 10, DisplayName: "Size",
			OptionValues: []bigcommerce.ProductOptionValue{
				{ID: 100, Label: "Small"},
				{ID: 101, Label: "Large"},
			},
		},
	}, nil)
	s.mockBC.EXPECT().CreateVariant(gomock.Any(), 1, gomock.Any()).Return(
		&bigcommerce.ProductVariantFull{ID: 201, ProductID: 1, SKU: "V-S"}, nil,
	)
	s.mockBC.EXPECT().CreateVariant(gomock.Any(), 1, gomock.Any()).Return(
		&bigcommerce.ProductVariantFull{ID: 202, ProductID: 1, SKU: "V-L"}, nil,
	)

	result, err := s.callTool("catalog/products/variants/create_batch", map[string]any{
		"product_id": float64(1),
		"variants": []any{
			map[string]any{
				"sku": "V-S",
				"option_values": []any{
					map[string]any{"option_display_name": "Size", "label": "Small"},
				},
			},
			map[string]any{
				"sku": "V-L",
				"option_values": []any{
					map[string]any{"option_display_name": "Size", "label": "Large"},
				},
			},
		},
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
	s.Equal(float64(2), data["created_count"])
	s.Equal(float64(0), data["failed_count"])
}

func (s *VariantToolSuite) TestVariantCreateBatchPartialSuccess() {
	s.mockBC.EXPECT().ListProductOptions(gomock.Any(), 1).Return([]bigcommerce.ProductOption{
		{
			ID: 10, DisplayName: "Size",
			OptionValues: []bigcommerce.ProductOptionValue{
				{ID: 100, Label: "Small"},
				{ID: 101, Label: "Large"},
			},
		},
	}, nil)
	s.mockBC.EXPECT().CreateVariant(gomock.Any(), 1, gomock.Any()).Return(
		&bigcommerce.ProductVariantFull{ID: 201, ProductID: 1, SKU: "V-S"}, nil,
	)
	s.mockBC.EXPECT().CreateVariant(gomock.Any(), 1, gomock.Any()).Return(
		nil, fmt.Errorf("duplicate sku"),
	)

	result, err := s.callTool("catalog/products/variants/create_batch", map[string]any{
		"product_id": float64(1),
		"variants": []any{
			map[string]any{
				"sku": "V-S",
				"option_values": []any{
					map[string]any{"option_display_name": "Size", "label": "Small"},
				},
			},
			map[string]any{
				"sku": "V-L",
				"option_values": []any{
					map[string]any{"option_display_name": "Size", "label": "Large"},
				},
			},
		},
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("partial_success", data["status"])
	s.Equal(float64(1), data["created_count"])
	s.Equal(float64(1), data["failed_count"])
}

func (s *VariantToolSuite) TestVariantCreateBatchExceedsCap() {
	variants := make([]any, 51)
	for i := range variants {
		variants[i] = map[string]any{
			"sku": fmt.Sprintf("V-%d", i),
			"option_values": []any{
				map[string]any{"option_id": float64(1), "id": float64(2), "label": "X"},
			},
		}
	}
	result, err := s.callTool("catalog/products/variants/create_batch", map[string]any{
		"product_id": float64(1),
		"variants":   variants,
		"confirmed":  true,
	})
	s.NoError(err)
	s.True(result.IsError)
}
