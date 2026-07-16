package promotions_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/promotions"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type PromotionSettingsSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	mockBC   *MockBigCommercePromotionsAPI
	registry *discovery.Registry
}

func TestPromotionSettingsSuite(t *testing.T) {
	suite.Run(t, new(PromotionSettingsSuite))
}

func (s *PromotionSettingsSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommercePromotionsAPI(s.ctrl)
	s.registry = discovery.NewRegistry()
	s.registry.RegisterCategory("marketing", "marketing")
	s.registry.RegisterCategory("marketing/promotions", "promotions")
	s.registry.RegisterCategory("marketing/promotions/settings", "settings")

	promotions.NewPromotionSettingsTools(s.mockBC, session.NewStore(0)).RegisterTools(s.registry)
}

func (s *PromotionSettingsSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *PromotionSettingsSuite) call(path string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.registry.GetTool(path)
	s.Require().NotNil(def, "tool %s not registered", path)
	return def.Handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Name: path, Arguments: args}})
}

func (s *PromotionSettingsSuite) parseJSON(res *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(res)
	s.Require().False(res.IsError, "unexpected tool error: %s", textOf(res))
	var m map[string]any
	s.Require().NoError(json.Unmarshal([]byte(textOf(res)), &m))
	return m
}

func currentSettings() *bigcommerce.PromotionSettings {
	return &bigcommerce.PromotionSettings{
		PromotionsTriggeredByZeroPriceProducts:  false,
		PromotionsApplyOnCustomPricedProducts:   false,
		NumberOfCouponsAllowedAtCheckout:        1,
		PromotionsAppliedOnOriginalProductPrice: true,
	}
}

// ---------------------------------------------------------------------------
// get
// ---------------------------------------------------------------------------

func (s *PromotionSettingsSuite) TestGetReturnsSettings() {
	s.mockBC.EXPECT().
		GetPromotionSettings(gomock.Any()).
		Return(currentSettings(), nil)

	res, err := s.call("marketing/promotions/settings/get", map[string]any{})
	s.NoError(err)
	data := s.parseJSON(res)
	settings, _ := data["settings"].(map[string]any)
	s.Equal(false, settings["promotions_triggered_by_products_with_zero_product_price"])
	s.Equal(false, settings["promotions_apply_on_products_with_custom_product_price"])
	s.Equal(float64(1), settings["number_of_coupons_allowed_at_checkout"])
	s.Equal(true, settings["promotions_applied_on_original_product_price"])
}

// ---------------------------------------------------------------------------
// update
// ---------------------------------------------------------------------------

func (s *PromotionSettingsSuite) TestUpdateRejectsWhenNoFieldsSupplied() {
	res, err := s.call("marketing/promotions/settings/update", map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "no settings supplied")
}

func (s *PromotionSettingsSuite) TestUpdateRejectsNonBooleanField() {
	res, err := s.call("marketing/promotions/settings/update", map[string]any{
		"promotions_apply_on_products_with_custom_product_price": "yes",
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "must be a boolean")
}

func (s *PromotionSettingsSuite) TestUpdateRejectsNonIntegerCouponCount() {
	res, err := s.call("marketing/promotions/settings/update", map[string]any{
		"number_of_coupons_allowed_at_checkout": float64(2.5),
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "must be an integer")
}

func (s *PromotionSettingsSuite) TestUpdateRejectsCouponCountBelowRange() {
	res, err := s.call("marketing/promotions/settings/update", map[string]any{
		"number_of_coupons_allowed_at_checkout": float64(0),
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "between 1 and 5")
}

func (s *PromotionSettingsSuite) TestUpdateRejectsCouponCountAboveRange() {
	res, err := s.call("marketing/promotions/settings/update", map[string]any{
		"number_of_coupons_allowed_at_checkout": float64(6),
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "between 1 and 5")
}

func (s *PromotionSettingsSuite) TestUpdatePreviewShowsMergedPayload() {
	s.mockBC.EXPECT().
		GetPromotionSettings(gomock.Any()).
		Return(currentSettings(), nil)

	res, err := s.call("marketing/promotions/settings/update", map[string]any{
		"promotions_apply_on_products_with_custom_product_price": true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	would, _ := data["would_apply"].(map[string]any)
	s.Equal(true, would["promotions_apply_on_products_with_custom_product_price"])
	s.Equal(float64(1), would["number_of_coupons_allowed_at_checkout"])
}

func (s *PromotionSettingsSuite) TestUpdatePreviewWarnsOnEnterpriseOnlyCouponCount() {
	s.mockBC.EXPECT().
		GetPromotionSettings(gomock.Any()).
		Return(currentSettings(), nil)

	res, err := s.call("marketing/promotions/settings/update", map[string]any{
		"number_of_coupons_allowed_at_checkout": float64(3),
	})
	s.NoError(err)
	data := s.parseJSON(res)
	warnings, _ := data["warnings"].([]any)
	s.Require().NotEmpty(warnings)
	s.Contains(warnings[0].(string), "Enterprise-plan feature")
}

func (s *PromotionSettingsSuite) TestUpdateNoopShortCircuitSkipsPut() {
	cur := currentSettings()
	s.mockBC.EXPECT().
		GetPromotionSettings(gomock.Any()).
		Return(cur, nil)
	// no expectation for UpdatePromotionSettings: must not be called

	res, err := s.call("marketing/promotions/settings/update", map[string]any{
		"number_of_coupons_allowed_at_checkout": float64(1),
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("noop", data["status"])
}

func (s *PromotionSettingsSuite) TestUpdateConfirmExecutesPut() {
	cur := currentSettings()
	updated := *cur
	updated.PromotionsTriggeredByZeroPriceProducts = true

	gomock.InOrder(
		s.mockBC.EXPECT().
			GetPromotionSettings(gomock.Any()).
			Return(cur, nil),
		s.mockBC.EXPECT().
			UpdatePromotionSettings(gomock.Any(), updated).
			Return(&updated, nil),
	)

	res, err := s.call("marketing/promotions/settings/update", map[string]any{
		"promotions_triggered_by_products_with_zero_product_price": true,
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("updated", data["status"])
	settings, _ := data["settings"].(map[string]any)
	s.Equal(true, settings["promotions_triggered_by_products_with_zero_product_price"])
}

func (s *PromotionSettingsSuite) TestUpdateConfirmEnterpriseFailureIncludesHint() {
	cur := currentSettings()
	merged := *cur
	merged.NumberOfCouponsAllowedAtCheckout = 2

	gomock.InOrder(
		s.mockBC.EXPECT().
			GetPromotionSettings(gomock.Any()).
			Return(cur, nil),
		s.mockBC.EXPECT().
			UpdatePromotionSettings(gomock.Any(), merged).
			Return(nil, errors.New("403: forbidden")),
	)

	res, err := s.call("marketing/promotions/settings/update", map[string]any{
		"number_of_coupons_allowed_at_checkout": float64(2),
		"confirmed":                             true,
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "requires an Enterprise plan")
}
