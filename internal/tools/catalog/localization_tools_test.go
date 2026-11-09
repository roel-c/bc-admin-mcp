package catalog_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type LocalizationToolsSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
}

func TestLocalizationToolsSuite(t *testing.T) {
	suite.Run(t, new(LocalizationToolsSuite))
}

func (s *LocalizationToolsSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
}

func (s *LocalizationToolsSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *LocalizationToolsSuite) registerAll(reg *discovery.Registry) {
	catalog.NewLocalizationLocalesTools(s.mockBC).RegisterTools(reg)
	catalog.NewLocalizationOverridesTools(s.mockBC).RegisterTools(reg)
	catalog.NewLocalizationOptionOverridesTools(s.mockBC).RegisterTools(reg)
	catalog.NewLocalizationCustomFieldOverridesTools(s.mockBC).RegisterTools(reg)
	catalog.NewLocalizationTranslationsTools(s.mockBC).RegisterTools(reg)
}

func (s *LocalizationToolsSuite) call(path string, args map[string]any) map[string]any {
	reg := discovery.NewRegistry()
	s.registerAll(reg)

	def := reg.GetTool(path)
	s.Require().NotNil(def, path)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	res, err := def.Handler(context.Background(), req)
	s.Require().NoError(err)
	s.Require().NotNil(res)
	if res.IsError {
		text := ""
		if len(res.Content) > 0 {
			if tc, ok := res.Content[0].(mcp.TextContent); ok {
				text = tc.Text
			}
		}
		s.FailNow("tool returned error: " + text)
	}
	s.Require().NotEmpty(res.Content)
	tc, ok := res.Content[0].(mcp.TextContent)
	s.Require().True(ok)
	var out map[string]any
	s.Require().NoError(json.Unmarshal([]byte(tc.Text), &out))
	return out
}

func (s *LocalizationToolsSuite) callExpectError(path string, args map[string]any) string {
	reg := discovery.NewRegistry()
	s.registerAll(reg)

	def := reg.GetTool(path)
	s.Require().NotNil(def, path)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	res, err := def.Handler(context.Background(), req)
	s.Require().NoError(err)
	s.Require().True(res.IsError)
	tc, ok := res.Content[0].(mcp.TextContent)
	s.Require().True(ok)
	return tc.Text
}

func (s *LocalizationToolsSuite) TestLocalesList() {
	s.mockBC.EXPECT().ListChannelLocales(gomock.Any(), 1).Return([]bigcommerce.ChannelLocale{
		{Code: "en", Status: "ACTIVE", IsDefault: true},
		{Code: "es", Status: "ACTIVE", IsDefault: false},
	}, nil)

	out := s.call("catalog/localization/locales/list", map[string]any{
		"channel_id": float64(1),
	})
	s.Equal(float64(2), out["total"])
	s.Equal(float64(1), out["channel_id"])
}

func (s *LocalizationToolsSuite) TestLocalesAddPreviewAndConfirm() {
	s.mockBC.EXPECT().ListChannelLocales(gomock.Any(), 1).Return([]bigcommerce.ChannelLocale{
		{Code: "en", Status: "ACTIVE", IsDefault: true},
	}, nil)

	preview := s.call("catalog/localization/locales/add", map[string]any{
		"channel_id": float64(1),
		"code":       "fr",
		"status":     "ACTIVE",
	})
	s.Equal("pending_confirmation", preview["status"])

	s.mockBC.EXPECT().ListChannelLocales(gomock.Any(), 1).Return([]bigcommerce.ChannelLocale{
		{Code: "en", Status: "ACTIVE", IsDefault: true},
	}, nil)
	s.mockBC.EXPECT().AddLocale(gomock.Any(), bigcommerce.AddLocaleInput{
		ChannelID: 1,
		Code:      "fr",
		Status:    "ACTIVE",
	}).Return(&bigcommerce.ChannelLocale{Code: "fr", Status: "ACTIVE"}, nil)

	out := s.call("catalog/localization/locales/add", map[string]any{
		"channel_id": float64(1),
		"code":       "fr",
		"status":     "ACTIVE",
		"confirmed":  true,
	})
	s.Equal("added", out["status"])
}

func (s *LocalizationToolsSuite) TestLocalesDeleteRejectsDefault() {
	s.mockBC.EXPECT().ListChannelLocales(gomock.Any(), 1).Return([]bigcommerce.ChannelLocale{
		{Code: "en", Status: "ACTIVE", IsDefault: true},
	}, nil)
	msg := s.callExpectError("catalog/localization/locales/delete", map[string]any{
		"channel_id": float64(1),
		"code":       "en",
	})
	s.Contains(msg, "cannot delete default locale")
}

func (s *LocalizationToolsSuite) TestOverridesGet() {
	s.mockBC.EXPECT().GetProductLocaleOverrides(gomock.Any(), 130, 1, "es").Return(&bigcommerce.ProductLocaleOverrides{
		ProductID: "bc/store/product/130",
		BasicInformation: &bigcommerce.ProductBasicInformation{
			Name: "Invoice Payment",
		},
		OverridesForLocale: &bigcommerce.LocaleProductOverrides{},
	}, nil)

	out := s.call("catalog/localization/overrides/get", map[string]any{
		"product_id": float64(130),
		"channel_id": float64(1),
		"locale":     "es",
	})
	s.Equal(float64(130), out["product_id"])
	s.Equal("es", out["locale"])
}

func (s *LocalizationToolsSuite) TestOverridesSetPreview() {
	s.mockBC.EXPECT().GetProductLocaleOverrides(gomock.Any(), 130, 1, "es").Return(&bigcommerce.ProductLocaleOverrides{
		ProductID:          "bc/store/product/130",
		OverridesForLocale: &bigcommerce.LocaleProductOverrides{},
	}, nil)

	out := s.call("catalog/localization/overrides/set", map[string]any{
		"product_id": float64(130),
		"channel_id": float64(1),
		"locale":     "es",
		"name":       "Pago de factura",
	})
	s.Equal("pending_confirmation", out["status"])
	would, ok := out["would_apply"].(map[string]any)
	s.Require().True(ok)
	s.Equal("Pago de factura", would["name"])
}

func (s *LocalizationToolsSuite) TestTranslationsUpdateRejectsDefaultLocale() {
	s.mockBC.EXPECT().DefaultLocaleCode(gomock.Any(), 1).Return("en", nil)
	msg := s.callExpectError("catalog/localization/translations/update", map[string]any{
		"channel_id":    float64(1),
		"locale":        "en",
		"resource_type": "PRODUCTS",
		"entities": []any{
			map[string]any{
				"resource_numeric_id": float64(130),
				"fields":              map[string]any{"name": "X"},
			},
		},
	})
	s.Contains(msg, "cannot write translations for default locale")
}

func (s *LocalizationToolsSuite) TestTranslationsUpdatePreview() {
	s.mockBC.EXPECT().DefaultLocaleCode(gomock.Any(), 1).Return("en", nil)
	s.mockBC.EXPECT().ListTranslations(gomock.Any(), gomock.Any()).Return(&bigcommerce.TranslationListResult{
		Resources: []bigcommerce.TranslationResource{
			{
				ResourceID: "bc/store/product/130",
				Fields: []bigcommerce.TranslationField{
					{FieldName: "name", Original: "Invoice Payment", Translation: nil},
				},
			},
		},
	}, nil)

	out := s.call("catalog/localization/translations/update", map[string]any{
		"channel_id":    float64(1),
		"locale":        "es",
		"resource_type": "PRODUCTS",
		"entities": []any{
			map[string]any{
				"resource_numeric_id": float64(130),
				"fields":              map[string]any{"name": "Pago de factura"},
			},
		},
	})
	s.Equal("pending_confirmation", out["status"])
}

func (s *LocalizationToolsSuite) TestOptionOverridesSetPreview() {
	s.mockBC.EXPECT().GetProductOptionLocaleOverrides(gomock.Any(), 130, 1, "en").Return(&bigcommerce.ProductOptionLocaleSnapshot{
		ProductID: "bc/store/product/130",
		Options: []bigcommerce.ProductOptionWithOverlay{
			{ID: "bc/store/productOption/10", DisplayName: "Length"},
		},
	}, nil)

	out := s.call("catalog/localization/overrides/options/set", map[string]any{
		"product_id": float64(130),
		"channel_id": float64(1),
		"locale":     "en",
		"options": []any{
			map[string]any{
				"option_numeric_id": float64(10),
				"display_name":      "Hose Length (ft)",
				"values": []any{
					map[string]any{"value_numeric_id": float64(20), "label": "3 ft Trade"},
				},
			},
		},
	})
	s.Equal("pending_confirmation", out["status"])
}

func (s *LocalizationToolsSuite) TestCustomFieldOverridesSetPreview() {
	s.mockBC.EXPECT().GetProductCustomFieldLocaleOverrides(gomock.Any(), 130, 1741970, "en").Return(
		&bigcommerce.ProductCustomFieldLocaleSnapshot{
			ProductID: "bc/store/product/130",
			CustomFields: []bigcommerce.ProductCustomFieldWithOverlay{
				{ID: "bc/store/productCustomField/1", Name: "Pack Qty", Value: "1"},
			},
		}, nil)

	out := s.call("catalog/localization/overrides/custom_fields/set", map[string]any{
		"product_id": float64(130),
		"channel_id": float64(1741970),
		"locale":     "en",
		"fields": []any{
			map[string]any{
				"custom_field_numeric_id": float64(1),
				"name":                    "Case Qty",
				"value":                   "12",
			},
		},
	})
	s.Equal("pending_confirmation", out["status"])
}
