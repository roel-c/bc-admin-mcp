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

type CouponPromotionsSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	mockBC   *MockBigCommercePromotionsAPI
	registry *discovery.Registry
}

func TestCouponPromotionsSuite(t *testing.T) {
	suite.Run(t, new(CouponPromotionsSuite))
}

func (s *CouponPromotionsSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommercePromotionsAPI(s.ctrl)
	s.registry = discovery.NewRegistry()
	s.registry.RegisterCategory("marketing", "marketing")
	s.registry.RegisterCategory("marketing/promotions", "promotions")
	s.registry.RegisterCategory("marketing/promotions/coupon", "coupon")

	promotions.NewCouponPromotions(s.mockBC, session.NewStore(0)).RegisterTools(s.registry)
}

func (s *CouponPromotionsSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *CouponPromotionsSuite) call(path string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.registry.GetTool(path)
	s.Require().NotNil(def, "tool %s not registered", path)
	return def.Handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Name: path, Arguments: args}})
}

func (s *CouponPromotionsSuite) parseJSON(res *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(res)
	s.Require().False(res.IsError, "unexpected tool error: %s", textOf(res))
	var m map[string]any
	s.Require().NoError(json.Unmarshal([]byte(textOf(res)), &m))
	return m
}

// validCouponPromotion returns a baseline COUPON promotion that the
// validators will accept. Tests mutate copies of this when probing edge
// cases.
func validCouponPromotion() map[string]any {
	return map[string]any{
		"name":        "Welcome 10",
		"coupon_type": "SINGLE",
		"rules": []any{
			map[string]any{
				"action": map[string]any{
					"cart_value": map[string]any{
						"discount": map[string]any{"percentage_amount": "10"},
					},
				},
				"apply_once": false,
				"stop":       false,
			},
		},
		"status": "ENABLED",
	}
}

// ---------------------------------------------------------------------------
// list
// ---------------------------------------------------------------------------

func (s *CouponPromotionsSuite) TestListHardPinsCoupon() {
	expected := bigcommerce.PromotionListParams{
		RedemptionType: "coupon",
		Code:           "WELCOME10",
		Limit:          25,
	}
	s.mockBC.EXPECT().
		SearchPromotions(gomock.Any(), expected).
		Return([]bigcommerce.Promotion{
			{ID: 9, Name: "Welcome", RedemptionType: "COUPON"},
		}, nil)

	res, err := s.call("marketing/promotions/coupon/list", map[string]any{
		"code":  "WELCOME10",
		"limit": float64(25),
	})
	s.NoError(err)
	data := s.parseJSON(res)
	filters, _ := data["filters"].(map[string]any)
	s.Equal("coupon", filters["redemption_type"])
	s.Equal("WELCOME10", filters["code"])
}

func (s *CouponPromotionsSuite) TestListFiltersOutAutomaticEntriesDefensively() {
	s.mockBC.EXPECT().
		SearchPromotions(gomock.Any(), gomock.Any()).
		Return([]bigcommerce.Promotion{
			{ID: 1, Name: "auto", RedemptionType: "AUTOMATIC"},
			{ID: 2, Name: "cpn", RedemptionType: "COUPON"},
			{ID: 3, Name: "blank"}, // legacy: no redemption_type returned
		}, nil)
	res, err := s.call("marketing/promotions/coupon/list", map[string]any{})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal(float64(2), data["total"], "AUTOMATIC must be filtered out, blank-type entries are kept")
}

func (s *CouponPromotionsSuite) TestListRejectsDecimalLimit() {
	res, err := s.call("marketing/promotions/coupon/list", map[string]any{"limit": float64(10.25)})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "limit must be a positive integer")
}

// ---------------------------------------------------------------------------
// get
// ---------------------------------------------------------------------------

func (s *CouponPromotionsSuite) TestGetRejectsAutomaticPromotion() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 42).
		Return(&bigcommerce.Promotion{ID: 42, RedemptionType: "AUTOMATIC"}, nil)

	res, err := s.call("marketing/promotions/coupon/get", map[string]any{"promotion_id": float64(42)})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "redemption_type=AUTOMATIC")
}

// ---------------------------------------------------------------------------
// create — coupon-specific validation
// ---------------------------------------------------------------------------

func (s *CouponPromotionsSuite) TestCreatePreviewHardPinsCoupon() {
	s.mockBC.EXPECT().
		SearchPromotions(gomock.Any(), gomock.Any()).
		Return([]bigcommerce.Promotion{}, nil)

	res, err := s.call("marketing/promotions/coupon/create", map[string]any{
		"promotion": validCouponPromotion(),
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	payload, _ := data["payload"].(map[string]any)
	s.Equal("COUPON", payload["redemption_type"])
}

func (s *CouponPromotionsSuite) TestCreateRejectsDeprecatedOverrideField() {
	bad := validCouponPromotion()
	bad["coupon_overrides_automatic_when_offering_higher_discounts"] = true

	res, err := s.call("marketing/promotions/coupon/create", map[string]any{"promotion": bad})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "deprecated")
	s.Contains(textOf(res), "coupon_overrides_other_promotions")
}

func (s *CouponPromotionsSuite) TestCreateRejectsBadCouponType() {
	bad := validCouponPromotion()
	bad["coupon_type"] = "GOLDEN_TICKET"
	res, err := s.call("marketing/promotions/coupon/create", map[string]any{"promotion": bad})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "coupon_type must be SINGLE or BULK")
}

func (s *CouponPromotionsSuite) TestCreateRejectsOverrideFlagWithoutCBUWOPFalse() {
	// coupon_overrides_other_promotions=true requires can_be_used_with_other_promotions=false.
	bad := validCouponPromotion()
	bad["coupon_overrides_other_promotions"] = true
	bad["can_be_used_with_other_promotions"] = true

	res, err := s.call("marketing/promotions/coupon/create", map[string]any{"promotion": bad})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "requires can_be_used_with_other_promotions=false")
}

func (s *CouponPromotionsSuite) TestCreateAcceptsOverrideFlagWhenCBUWOPFalse() {
	s.mockBC.EXPECT().
		SearchPromotions(gomock.Any(), gomock.Any()).
		Return([]bigcommerce.Promotion{}, nil)

	good := validCouponPromotion()
	good["coupon_overrides_other_promotions"] = true
	good["can_be_used_with_other_promotions"] = false

	res, err := s.call("marketing/promotions/coupon/create", map[string]any{"promotion": good})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *CouponPromotionsSuite) TestCreateRejectsMultipleCodesOnSingle() {
	bad := validCouponPromotion()
	bad["coupon_type"] = "SINGLE"
	bad["multiple_codes"] = map[string]any{"length": float64(8)}

	res, err := s.call("marketing/promotions/coupon/create", map[string]any{"promotion": bad})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "multiple_codes is only valid on coupon_type=BULK")
}

func (s *CouponPromotionsSuite) TestCreateAcceptsMultipleCodesOnBulk() {
	s.mockBC.EXPECT().
		SearchPromotions(gomock.Any(), gomock.Any()).
		Return([]bigcommerce.Promotion{}, nil)

	good := validCouponPromotion()
	good["coupon_type"] = "BULK"
	good["multiple_codes"] = map[string]any{"length": float64(8)}

	res, err := s.call("marketing/promotions/coupon/create", map[string]any{"promotion": good})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *CouponPromotionsSuite) TestCreateExecutesOnConfirm() {
	s.mockBC.EXPECT().
		SearchPromotions(gomock.Any(), gomock.Any()).
		Return([]bigcommerce.Promotion{}, nil).
		AnyTimes()
	s.mockBC.EXPECT().
		CreatePromotion(gomock.Any(), gomock.Any()).
		Return(&bigcommerce.Promotion{ID: 7, RedemptionType: "COUPON"}, nil)

	res, err := s.call("marketing/promotions/coupon/create", map[string]any{
		"promotion": validCouponPromotion(),
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("created", data["status"])
}

// ---------------------------------------------------------------------------
// update — fetch-merge-PUT
// ---------------------------------------------------------------------------

func (s *CouponPromotionsSuite) TestUpdateRejectsAutomaticPromotion() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 11).
		Return(&bigcommerce.Promotion{ID: 11, RedemptionType: "AUTOMATIC", Status: "ENABLED"}, nil)

	res, err := s.call("marketing/promotions/coupon/update", map[string]any{
		"promotion_id": float64(11),
		"patch":        map[string]any{"name": "x"},
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "redemption_type=AUTOMATIC")
}

func (s *CouponPromotionsSuite) TestUpdateMergesScalarPatchAndStampsCoupon() {
	current := &bigcommerce.Promotion{
		ID:             11,
		Name:           "Old",
		RedemptionType: "COUPON",
		CouponType:     "SINGLE",
		Status:         "ENABLED",
		Rules:          json.RawMessage(`[{"action":{"cart_value":{"discount":{"percentage_amount":"10"}}}}]`),
	}
	s.mockBC.EXPECT().GetPromotion(gomock.Any(), 11).Return(current, nil)

	res, err := s.call("marketing/promotions/coupon/update", map[string]any{
		"promotion_id": float64(11),
		"patch":        map[string]any{"name": "New"},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	would, _ := data["would_apply"].(map[string]any)
	s.Equal("New", would["name"])
	s.Equal("COUPON", would["redemption_type"], "merged payload must stamp COUPON before validation")
}

func (s *CouponPromotionsSuite) TestUpdateRulesPatchPositional() {
	current := &bigcommerce.Promotion{
		ID:             11,
		RedemptionType: "COUPON",
		CouponType:     "SINGLE",
		Rules: json.RawMessage(`[
			{"action":{"cart_value":{"discount":{"percentage_amount":"10"}}}},
			{"action":{"cart_value":{"discount":{"percentage_amount":"20"}}}}
		]`),
	}
	s.mockBC.EXPECT().GetPromotion(gomock.Any(), 11).Return(current, nil)

	res, err := s.call("marketing/promotions/coupon/update", map[string]any{
		"promotion_id": float64(11),
		"rules_patch": []any{
			map[string]any{
				"index": float64(1),
				"replace_with": map[string]any{
					"action": map[string]any{
						"cart_value": map[string]any{
							"discount": map[string]any{"percentage_amount": "30"},
						},
					},
				},
			},
		},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	would, _ := data["would_apply"].(map[string]any)
	rules, _ := would["rules"].([]any)
	s.Require().Len(rules, 2)
	r1, _ := rules[1].(map[string]any)
	disc := r1["action"].(map[string]any)["cart_value"].(map[string]any)["discount"].(map[string]any)
	s.Equal("30", disc["percentage_amount"])
}

func (s *CouponPromotionsSuite) TestUpdateRejectsReadOnlyField() {
	current := &bigcommerce.Promotion{ID: 11, RedemptionType: "COUPON", CouponType: "SINGLE"}
	s.mockBC.EXPECT().GetPromotion(gomock.Any(), 11).Return(current, nil)

	res, err := s.call("marketing/promotions/coupon/update", map[string]any{
		"promotion_id": float64(11),
		"patch":        map[string]any{"redemption_type": "AUTOMATIC"},
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), `read-only field "redemption_type"`)
}

// ---------------------------------------------------------------------------
// set_status
// ---------------------------------------------------------------------------

func (s *CouponPromotionsSuite) TestSetStatusRejectsAutomatic() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 11).
		Return(&bigcommerce.Promotion{ID: 11, RedemptionType: "AUTOMATIC", Status: "ENABLED"}, nil)

	res, err := s.call("marketing/promotions/coupon/set_status", map[string]any{
		"promotion_id": float64(11),
		"status":       "DISABLED",
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "redemption_type=AUTOMATIC")
}

func (s *CouponPromotionsSuite) TestSetStatusNoOp() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 11).
		Return(&bigcommerce.Promotion{ID: 11, RedemptionType: "COUPON", CouponType: "SINGLE", Status: "ENABLED"}, nil)

	res, err := s.call("marketing/promotions/coupon/set_status", map[string]any{
		"promotion_id": float64(11),
		"status":       "ENABLED",
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("noop", data["status"])
}

// ---------------------------------------------------------------------------
// delete — preview, cascade, hint
// ---------------------------------------------------------------------------

func (s *CouponPromotionsSuite) TestDeletePreviewSurfacesAttachedCodes() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 11).
		Return(&bigcommerce.Promotion{ID: 11, Name: "Test", RedemptionType: "COUPON", Status: "ENABLED"}, nil)
	s.mockBC.EXPECT().
		ListCouponCodes(gomock.Any(), 11, gomock.Any()).
		Return(&bigcommerce.CouponCodeListResponse{
			Codes: []bigcommerce.CouponCode{
				{ID: 100, Code: "WELCOME10"},
				{ID: 101, Code: "WELCOME20"},
			},
			Cursor: bigcommerce.CouponCodeCursors{After: "next-cursor"},
		}, nil)

	res, err := s.call("marketing/promotions/coupon/delete", map[string]any{
		"promotion_ids": []any{float64(11)},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	matched, _ := data["matched"].([]any)
	s.Require().Len(matched, 1)
	entry, _ := matched[0].(map[string]any)
	s.Equal(float64(2), entry["attached_codes_first_page"])
	s.Equal(true, entry["has_more_code_pages"])
	sample, _ := entry["attached_codes_sample"].([]any)
	s.Len(sample, 2)
}

func (s *CouponPromotionsSuite) TestDeleteCascadeWalksAndDeletesCodes() {
	gomock.InOrder(
		s.mockBC.EXPECT().
			ListCouponCodes(gomock.Any(), 11, bigcommerce.CouponCodeListParams{Limit: 250}).
			Return(&bigcommerce.CouponCodeListResponse{
				Codes:  []bigcommerce.CouponCode{{ID: 100}, {ID: 101}},
				Cursor: bigcommerce.CouponCodeCursors{After: "next"},
			}, nil),
		s.mockBC.EXPECT().
			ListCouponCodes(gomock.Any(), 11, bigcommerce.CouponCodeListParams{After: "next", Limit: 250}).
			Return(&bigcommerce.CouponCodeListResponse{
				Codes:  []bigcommerce.CouponCode{{ID: 102}},
				Cursor: bigcommerce.CouponCodeCursors{},
			}, nil),
		s.mockBC.EXPECT().
			DeleteCouponCodes(gomock.Any(), 11, []int{100, 101, 102}).
			Return(nil),
		s.mockBC.EXPECT().
			DeletePromotionsByIDs(gomock.Any(), []int{11}).
			Return(nil),
	)

	res, err := s.call("marketing/promotions/coupon/delete", map[string]any{
		"promotion_ids":      []any{float64(11)},
		"delete_codes_first": true,
		"confirmed":          true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("deleted", data["status"])
	report, _ := data["codes_deleted_per_promotion"].(map[string]any)
	s.Equal(float64(3), report["11"])
}

func (s *CouponPromotionsSuite) TestDeleteWithoutCascadeSurfaceHintOn422() {
	s.mockBC.EXPECT().
		DeletePromotionsByIDs(gomock.Any(), []int{11}).
		Return(errors.New("422: coupon codes still attached"))

	res, err := s.call("marketing/promotions/coupon/delete", map[string]any{
		"promotion_ids": []any{float64(11)},
		"confirmed":     true,
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "marketing/promotions/coupon/codes/delete")
	s.Contains(textOf(res), "delete_codes_first=true")
}

func (s *CouponPromotionsSuite) TestDeleteCascadeRefusesAboveCap() {
	// First page returns 1001 codes — above cascadeDeleteCodesCap (1000).
	bigPage := make([]bigcommerce.CouponCode, 1001)
	for i := range bigPage {
		bigPage[i] = bigcommerce.CouponCode{ID: i + 1}
	}
	s.mockBC.EXPECT().
		ListCouponCodes(gomock.Any(), 11, bigcommerce.CouponCodeListParams{Limit: 250}).
		Return(&bigcommerce.CouponCodeListResponse{Codes: bigPage}, nil)

	res, err := s.call("marketing/promotions/coupon/delete", map[string]any{
		"promotion_ids":      []any{float64(11)},
		"delete_codes_first": true,
		"confirmed":          true,
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "more than 1000 coupon codes")
}

func (s *CouponPromotionsSuite) TestDeleteRejectsOverCap() {
	ids := make([]any, 41)
	for i := range ids {
		ids[i] = float64(i + 1)
	}
	res, err := s.call("marketing/promotions/coupon/delete", map[string]any{
		"promotion_ids": ids,
		"confirmed":     true,
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "exceeds max of 40")
}
