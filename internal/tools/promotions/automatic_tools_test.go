package promotions_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/tools/promotions"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type AutomaticPromotionsSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	mockBC   *MockBigCommercePromotionsAPI
	registry *discovery.Registry
}

func TestAutomaticPromotionsSuite(t *testing.T) {
	suite.Run(t, new(AutomaticPromotionsSuite))
}

func (s *AutomaticPromotionsSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommercePromotionsAPI(s.ctrl)
	s.registry = discovery.NewRegistry()
	s.registry.RegisterCategory("marketing", "marketing")
	s.registry.RegisterCategory("marketing/promotions", "promotions")
	s.registry.RegisterCategory("marketing/promotions/automatic", "automatic")

	promotions.NewAutomaticPromotions(s.mockBC).RegisterTools(s.registry)
}

func (s *AutomaticPromotionsSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *AutomaticPromotionsSuite) call(path string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.registry.GetTool(path)
	s.Require().NotNil(def, "tool %s not registered", path)
	return def.Handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Name: path, Arguments: args}})
}

func (s *AutomaticPromotionsSuite) parseJSON(res *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(res)
	s.Require().False(res.IsError, "unexpected tool error: %s", textOf(res))
	var m map[string]any
	s.Require().NoError(json.Unmarshal([]byte(textOf(res)), &m))
	return m
}

func textOf(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	if t, ok := res.Content[0].(mcp.TextContent); ok {
		return t.Text
	}
	return ""
}

// ---------------------------------------------------------------------------
// list
// ---------------------------------------------------------------------------

func (s *AutomaticPromotionsSuite) TestListHardPinsAutomaticAndForwardsFilters() {
	expected := bigcommerce.PromotionListParams{
		RedemptionType: "automatic",
		Status:         "ENABLED",
		CurrencyCode:   "USD",
		Sort:           "priority",
		Direction:      "asc",
		Page:           2,
		Limit:          25,
		Channels:       []int{1, 2},
		Query:          "summer",
	}
	s.mockBC.EXPECT().
		SearchPromotions(gomock.Any(), expected).
		Return([]bigcommerce.Promotion{
			{ID: 7, Name: "Summer Sale", RedemptionType: "AUTOMATIC", Status: "ENABLED"},
		}, nil)

	res, err := s.call("marketing/promotions/automatic/list", map[string]any{
		"status":        "ENABLED",
		"currency_code": "usd",
		"sort":          "priority",
		"direction":     "asc",
		"page":          float64(2),
		"limit":         float64(25),
		"channels":      []any{float64(1), float64(2)},
		"query":         "summer",
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
	filters, _ := data["filters"].(map[string]any)
	s.Equal("automatic", filters["redemption_type"])
}

func (s *AutomaticPromotionsSuite) TestListFiltersOutCouponEntriesDefensively() {
	s.mockBC.EXPECT().
		SearchPromotions(gomock.Any(), gomock.Any()).
		Return([]bigcommerce.Promotion{
			{ID: 1, Name: "auto", RedemptionType: "AUTOMATIC"},
			{ID: 2, Name: "cpn", RedemptionType: "COUPON"},
			{ID: 3, Name: "no-type"}, // legacy entry without redemption_type set
		}, nil)

	res, err := s.call("marketing/promotions/automatic/list", map[string]any{})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal(float64(2), data["total"], "coupon entry must be filtered out, blank-type entries are kept")
}

func (s *AutomaticPromotionsSuite) TestListRejectsBadSort() {
	res, err := s.call("marketing/promotions/automatic/list", map[string]any{"sort": "bad"})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "sort must be one of")
}

func (s *AutomaticPromotionsSuite) TestListRejectsDecimalPage() {
	res, err := s.call("marketing/promotions/automatic/list", map[string]any{"page": float64(1.5)})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "page must be a positive integer")
}

// ---------------------------------------------------------------------------
// get
// ---------------------------------------------------------------------------

func (s *AutomaticPromotionsSuite) TestGetReturnsPromotion() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 42).
		Return(&bigcommerce.Promotion{ID: 42, Name: "Sale", RedemptionType: "AUTOMATIC"}, nil)

	res, err := s.call("marketing/promotions/automatic/get", map[string]any{"promotion_id": float64(42)})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal(float64(42), data["id"])
}

func (s *AutomaticPromotionsSuite) TestGetRejectsCouponPromotion() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 42).
		Return(&bigcommerce.Promotion{ID: 42, Name: "Coupon", RedemptionType: "COUPON"}, nil)

	res, err := s.call("marketing/promotions/automatic/get", map[string]any{"promotion_id": float64(42)})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "redemption_type=COUPON")
}

func (s *AutomaticPromotionsSuite) TestGetRejectsZeroID() {
	res, err := s.call("marketing/promotions/automatic/get", map[string]any{"promotion_id": float64(0)})
	s.NoError(err)
	s.True(res.IsError)
}

// ---------------------------------------------------------------------------
// create — preview, validation, soft-warn
// ---------------------------------------------------------------------------

func validCartItemsPromotion() map[string]any {
	return map[string]any{
		"name": "Buy One Get One",
		"rules": []any{
			map[string]any{
				"action": map[string]any{
					"cart_items": map[string]any{
						"discount": map[string]any{"percentage_amount": "100"},
						"strategy": "LEAST_EXPENSIVE",
						"items":    map[string]any{"products": []any{float64(174)}},
					},
				},
				"apply_once": false,
				"stop":       false,
			},
		},
		"notifications": []any{
			map[string]any{
				"type":      "APPLIED",
				"content":   "Free!",
				"locations": []any{"CART_PAGE"},
			},
		},
		"start_date": "2025-01-01T00:00:00+00:00",
		"status":     "ENABLED",
	}
}

func (s *AutomaticPromotionsSuite) TestCreatePreviewBlocksWithoutConfirmAndForcesAutomatic() {
	// On preview we still query the active count (single page).
	s.mockBC.EXPECT().
		SearchPromotions(gomock.Any(), gomock.Any()).
		Return([]bigcommerce.Promotion{}, nil)

	res, err := s.call("marketing/promotions/automatic/create", map[string]any{
		"promotion": validCartItemsPromotion(),
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	payload, _ := data["payload"].(map[string]any)
	s.Equal("AUTOMATIC", payload["redemption_type"], "create must hard-pin redemption_type=AUTOMATIC")
}

func (s *AutomaticPromotionsSuite) TestCreateValidatesActionShape() {
	bad := validCartItemsPromotion()
	rules := bad["rules"].([]any)
	rules[0].(map[string]any)["action"] = map[string]any{
		"cart_items": map[string]any{}, // discount missing
	}
	res, err := s.call("marketing/promotions/automatic/create", map[string]any{"promotion": bad})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "cart_items.discount is required")
}

func (s *AutomaticPromotionsSuite) TestCreateRejectsMutuallyExclusiveCustomerGroups() {
	bad := validCartItemsPromotion()
	bad["customer"] = map[string]any{
		"group_ids":          []any{float64(1)},
		"excluded_group_ids": []any{float64(2)},
	}
	res, err := s.call("marketing/promotions/automatic/create", map[string]any{"promotion": bad})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "cannot both be non-empty")
}

func (s *AutomaticPromotionsSuite) TestCreateRejectsBadDiscountCombo() {
	bad := validCartItemsPromotion()
	rules := bad["rules"].([]any)
	rules[0].(map[string]any)["action"] = map[string]any{
		"cart_items": map[string]any{
			"discount": map[string]any{
				"percentage_amount": "10",
				"fixed_amount":      "5",
			},
		},
	}
	res, err := s.call("marketing/promotions/automatic/create", map[string]any{"promotion": bad})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "exactly one of percentage_amount or fixed_amount")
}

func (s *AutomaticPromotionsSuite) TestCreateRejectsInvalidNotificationLocation() {
	bad := validCartItemsPromotion()
	bad["notifications"] = []any{
		map[string]any{
			"type":      "APPLIED",
			"content":   "x",
			"locations": []any{"FOOTER"},
		},
	}
	res, err := s.call("marketing/promotions/automatic/create", map[string]any{"promotion": bad})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "notifications[0].locations[0]")
}

func (s *AutomaticPromotionsSuite) TestCreateRejectsInvalidStatus() {
	bad := validCartItemsPromotion()
	bad["status"] = "INVALID"
	res, err := s.call("marketing/promotions/automatic/create", map[string]any{"promotion": bad})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "status must be ENABLED or DISABLED")
}

func (s *AutomaticPromotionsSuite) TestCreateRejectsBadCurrencyCode() {
	bad := validCartItemsPromotion()
	bad["currency_code"] = "usd"
	res, err := s.call("marketing/promotions/automatic/create", map[string]any{"promotion": bad})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "currency_code")
}

func (s *AutomaticPromotionsSuite) TestCreateValidatesItemMatcherKeys() {
	bad := validCartItemsPromotion()
	rules := bad["rules"].([]any)
	rules[0].(map[string]any)["action"].(map[string]any)["cart_items"].(map[string]any)["items"] = map[string]any{
		"unknown_key": []any{float64(1)},
	}
	res, err := s.call("marketing/promotions/automatic/create", map[string]any{"promotion": bad})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "unknown item matcher key")
}

func (s *AutomaticPromotionsSuite) TestCreateExecutesOnConfirm() {
	body, _ := json.Marshal(struct {
		Status string `json:"status"`
	}{Status: "ENABLED"})

	s.mockBC.EXPECT().
		SearchPromotions(gomock.Any(), gomock.Any()).
		Return([]bigcommerce.Promotion{}, nil).
		AnyTimes()
	s.mockBC.EXPECT().
		CreatePromotion(gomock.Any(), gomock.Any()).
		Return(&bigcommerce.Promotion{ID: 1, Name: "x", RedemptionType: "AUTOMATIC"}, nil)

	res, err := s.call("marketing/promotions/automatic/create", map[string]any{
		"promotion": validCartItemsPromotion(),
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("created", data["status"])
	_ = body
}

func (s *AutomaticPromotionsSuite) TestCreateSurfacesActiveCountWarning() {
	// 100 ENABLED promotions on the store -> soft-warn surfaces in preview.
	hundred := make([]bigcommerce.Promotion, 100)
	for i := range hundred {
		hundred[i] = bigcommerce.Promotion{ID: i + 1, Status: "ENABLED", RedemptionType: "AUTOMATIC"}
	}
	s.mockBC.EXPECT().
		SearchPromotions(gomock.Any(), gomock.Any()).
		Return(hundred, nil)

	res, err := s.call("marketing/promotions/automatic/create", map[string]any{
		"promotion": validCartItemsPromotion(),
	})
	s.NoError(err)
	data := s.parseJSON(res)
	warnings, _ := data["warnings"].([]any)
	s.Require().NotEmpty(warnings, "expected soft-warn for >=100 ENABLED promotions")
	joined := ""
	for _, w := range warnings {
		joined += w.(string) + " | "
	}
	s.Contains(joined, "ENABLED promotions")
}

// ---------------------------------------------------------------------------
// update — preview, fetch+merge, rules_patch positional
// ---------------------------------------------------------------------------

func savedPromotion() *bigcommerce.Promotion {
	rulesRaw, _ := json.Marshal([]any{
		map[string]any{
			"action": map[string]any{
				"cart_value": map[string]any{
					"discount": map[string]any{"percentage_amount": "10"},
				},
			},
			"apply_once": true,
			"stop":       true,
		},
		map[string]any{
			"action": map[string]any{
				"shipping": map[string]any{"free_shipping": true},
			},
			"apply_once": false,
			"stop":       false,
		},
	})
	notRaw, _ := json.Marshal([]any{
		map[string]any{
			"type":      "APPLIED",
			"content":   "x",
			"locations": []any{"CART_PAGE"},
		},
	})
	return &bigcommerce.Promotion{
		ID:             100,
		Name:           "Old Name",
		RedemptionType: "AUTOMATIC",
		Status:         "ENABLED",
		Rules:          rulesRaw,
		Notifications:  notRaw,
		CurrentUses:    5,
		CreatedFrom:    "api",
	}
}

func (s *AutomaticPromotionsSuite) TestUpdatePreviewMergesTopLevelScalars() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 100).
		Return(savedPromotion(), nil)

	res, err := s.call("marketing/promotions/automatic/update", map[string]any{
		"promotion_id": float64(100),
		"patch":        map[string]any{"name": "New Name"},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	would, _ := data["would_apply"].(map[string]any)
	s.Equal("New Name", would["name"])
	// Read-only fields stripped from would_apply.
	s.NotContains(would, "id")
	s.NotContains(would, "current_uses")
	s.NotContains(would, "created_from")
}

func (s *AutomaticPromotionsSuite) TestUpdateRejectsReadOnlyFieldsInPatch() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 100).
		Return(savedPromotion(), nil)

	res, err := s.call("marketing/promotions/automatic/update", map[string]any{
		"promotion_id": float64(100),
		"patch":        map[string]any{"current_uses": float64(0)},
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "read-only field")
}

func (s *AutomaticPromotionsSuite) TestUpdateRulesPatchPositional() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 100).
		Return(savedPromotion(), nil)

	replacementRule := map[string]any{
		"action": map[string]any{
			"cart_value": map[string]any{
				"discount": map[string]any{"fixed_amount": "20"},
			},
		},
		"apply_once": false,
		"stop":       false,
	}

	res, err := s.call("marketing/promotions/automatic/update", map[string]any{
		"promotion_id": float64(100),
		"rules_patch": []any{
			map[string]any{"index": float64(0), "replace_with": replacementRule},
		},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	would, _ := data["would_apply"].(map[string]any)
	rules, _ := would["rules"].([]any)
	s.Require().Len(rules, 2, "rules_patch must edit-in-place, not replace the entire array")
	first, _ := rules[0].(map[string]any)
	action, _ := first["action"].(map[string]any)
	cartValue, _ := action["cart_value"].(map[string]any)
	disc, _ := cartValue["discount"].(map[string]any)
	s.Equal("20", disc["fixed_amount"])

	// The second rule (shipping) must remain unchanged.
	second, _ := rules[1].(map[string]any)
	secondAction, _ := second["action"].(map[string]any)
	_, ok := secondAction["shipping"]
	s.True(ok, "rule[1] (shipping) must be untouched")
}

func (s *AutomaticPromotionsSuite) TestUpdateRulesPatchOutOfRange() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 100).
		Return(savedPromotion(), nil)

	res, err := s.call("marketing/promotions/automatic/update", map[string]any{
		"promotion_id": float64(100),
		"rules_patch": []any{
			map[string]any{"index": float64(99), "replace_with": map[string]any{"action": map[string]any{"shipping": map[string]any{"free_shipping": true}}}},
		},
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "out of range")
}

func (s *AutomaticPromotionsSuite) TestUpdateRulesReplacementWarnsInPreview() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 100).
		Return(savedPromotion(), nil)

	// Pass a fresh single-rule rules array via patch — must warn that the
	// existing array is being replaced in full.
	res, err := s.call("marketing/promotions/automatic/update", map[string]any{
		"promotion_id": float64(100),
		"patch": map[string]any{
			"rules": []any{
				map[string]any{
					"action": map[string]any{
						"cart_value": map[string]any{
							"discount": map[string]any{"percentage_amount": "5"},
						},
					},
				},
			},
		},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	warnings, _ := data["warnings"].([]any)
	s.Require().NotEmpty(warnings)
	combined := ""
	for _, w := range warnings {
		combined += w.(string) + " | "
	}
	s.Contains(combined, "REPLACED")
}

func (s *AutomaticPromotionsSuite) TestUpdateRejectsCouponPromotion() {
	coupon := savedPromotion()
	coupon.RedemptionType = "COUPON"
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 100).
		Return(coupon, nil)

	res, err := s.call("marketing/promotions/automatic/update", map[string]any{
		"promotion_id": float64(100),
		"patch":        map[string]any{"name": "x"},
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "redemption_type=COUPON")
}

func (s *AutomaticPromotionsSuite) TestUpdateExecutesOnConfirm() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 100).
		Return(savedPromotion(), nil)
	s.mockBC.EXPECT().
		UpdatePromotion(gomock.Any(), 100, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ int, body json.RawMessage) (*bigcommerce.Promotion, error) {
			var m map[string]any
			s.Require().NoError(json.Unmarshal(body, &m))
			s.Equal("AUTOMATIC", m["redemption_type"], "update payload must include hard-pinned redemption_type")
			s.Equal("Renamed", m["name"])
			return &bigcommerce.Promotion{ID: 100, Name: "Renamed", RedemptionType: "AUTOMATIC"}, nil
		})

	res, err := s.call("marketing/promotions/automatic/update", map[string]any{
		"promotion_id": float64(100),
		"patch":        map[string]any{"name": "Renamed"},
		"confirmed":    true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("updated", data["status"])
}

// ---------------------------------------------------------------------------
// set_status
// ---------------------------------------------------------------------------

func (s *AutomaticPromotionsSuite) TestSetStatusNoOpWhenAlreadyDesired() {
	p := savedPromotion()
	s.mockBC.EXPECT().GetPromotion(gomock.Any(), 100).Return(p, nil)

	res, err := s.call("marketing/promotions/automatic/set_status", map[string]any{
		"promotion_id": float64(100),
		"status":       "ENABLED",
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("noop", data["status"])
}

func (s *AutomaticPromotionsSuite) TestSetStatusPreviewThenExecute() {
	s.mockBC.EXPECT().GetPromotion(gomock.Any(), 100).Return(savedPromotion(), nil).Times(2)
	s.mockBC.EXPECT().
		UpdatePromotion(gomock.Any(), 100, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ int, body json.RawMessage) (*bigcommerce.Promotion, error) {
			var m map[string]any
			s.Require().NoError(json.Unmarshal(body, &m))
			s.Equal("DISABLED", m["status"])
			return &bigcommerce.Promotion{ID: 100, Status: "DISABLED", RedemptionType: "AUTOMATIC"}, nil
		})

	preview, err := s.call("marketing/promotions/automatic/set_status", map[string]any{
		"promotion_id": float64(100),
		"status":       "DISABLED",
	})
	s.NoError(err)
	pdata := s.parseJSON(preview)
	s.Equal("preview", pdata["status"])

	res, err := s.call("marketing/promotions/automatic/set_status", map[string]any{
		"promotion_id": float64(100),
		"status":       "DISABLED",
		"confirmed":    true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("updated", data["status"])
}

func (s *AutomaticPromotionsSuite) TestSetStatusRejectsBadValue() {
	res, err := s.call("marketing/promotions/automatic/set_status", map[string]any{
		"promotion_id": float64(100),
		"status":       "INVALID",
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "status must be ENABLED or DISABLED")
}

// ---------------------------------------------------------------------------
// delete — preview, batch cap, coupon-attached hint
// ---------------------------------------------------------------------------

func (s *AutomaticPromotionsSuite) TestDeleteEnforcesBatchCap() {
	ids := make([]any, 41)
	for i := range ids {
		ids[i] = float64(i + 1)
	}
	res, err := s.call("marketing/promotions/automatic/delete", map[string]any{
		"promotion_ids": ids,
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "exceeds max of 40")
}

func (s *AutomaticPromotionsSuite) TestDeleteRejectsDecimalPromotionID() {
	res, err := s.call("marketing/promotions/automatic/delete", map[string]any{
		"promotion_ids": []any{float64(1.5)},
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "must be an integer")
}

func (s *AutomaticPromotionsSuite) TestDeletePreviewListsMatchedPromotions() {
	s.mockBC.EXPECT().GetPromotion(gomock.Any(), 1).Return(&bigcommerce.Promotion{ID: 1, Name: "A", Status: "ENABLED", CurrentUses: 3, RedemptionType: "AUTOMATIC"}, nil)
	s.mockBC.EXPECT().GetPromotion(gomock.Any(), 2).Return(&bigcommerce.Promotion{ID: 2, Name: "B", Status: "DISABLED", RedemptionType: "AUTOMATIC"}, nil)

	res, err := s.call("marketing/promotions/automatic/delete", map[string]any{
		"promotion_ids": []any{float64(1), float64(2)},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	s.Equal(float64(2), data["would_delete"])
}

func (s *AutomaticPromotionsSuite) TestDeleteExecutesOnConfirm() {
	s.mockBC.EXPECT().DeletePromotionsByIDs(gomock.Any(), []int{1, 2}).Return(nil)

	res, err := s.call("marketing/promotions/automatic/delete", map[string]any{
		"promotion_ids": []any{float64(1), float64(2)},
		"confirmed":     true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("deleted", data["status"])
}

func (s *AutomaticPromotionsSuite) TestDeleteSurfacesCouponAttachedHint() {
	s.mockBC.EXPECT().
		DeletePromotionsByIDs(gomock.Any(), []int{1}).
		Return(errors.New("422 Unprocessable Entity: cannot delete promotion with coupon codes attached"))

	res, err := s.call("marketing/promotions/automatic/delete", map[string]any{
		"promotion_ids": []any{float64(1)},
		"confirmed":     true,
	})
	s.NoError(err)
	s.True(res.IsError)
	s.True(strings.Contains(textOf(res), "coupon codes attached"))
}
