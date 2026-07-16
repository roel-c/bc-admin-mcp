package promotions_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/tools/promotions"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type CouponCodesSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	mockBC   *MockBigCommercePromotionsAPI
	registry *discovery.Registry
}

func TestCouponCodesSuite(t *testing.T) {
	suite.Run(t, new(CouponCodesSuite))
}

func (s *CouponCodesSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommercePromotionsAPI(s.ctrl)
	s.registry = discovery.NewRegistry()
	s.registry.RegisterCategory("marketing", "marketing")
	s.registry.RegisterCategory("marketing/promotions", "promotions")
	s.registry.RegisterCategory("marketing/promotions/coupon", "coupon")
	s.registry.RegisterCategory("marketing/promotions/coupon/codes", "codes")

	promotions.NewCouponCodes(s.mockBC).RegisterTools(s.registry)
}

func (s *CouponCodesSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *CouponCodesSuite) call(path string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.registry.GetTool(path)
	s.Require().NotNil(def, "tool %s not registered", path)
	return def.Handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Name: path, Arguments: args}})
}

func (s *CouponCodesSuite) parseJSON(res *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(res)
	s.Require().False(res.IsError, "unexpected tool error: %s", textOf(res))
	var m map[string]any
	s.Require().NoError(json.Unmarshal([]byte(textOf(res)), &m))
	return m
}

// ---------------------------------------------------------------------------
// list — cursor pagination
// ---------------------------------------------------------------------------

func (s *CouponCodesSuite) TestListPassesCursorAndLimit() {
	expected := bigcommerce.CouponCodeListParams{After: "abc", Limit: 100}
	s.mockBC.EXPECT().
		ListCouponCodes(gomock.Any(), 11, expected).
		Return(&bigcommerce.CouponCodeListResponse{
			Codes:  []bigcommerce.CouponCode{{ID: 1, Code: "X"}},
			Cursor: bigcommerce.CouponCodeCursors{After: "next"},
		}, nil)

	res, err := s.call("marketing/promotions/coupon/codes/list", map[string]any{
		"promotion_id": float64(11),
		"after":        "abc",
		"limit":        float64(100),
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal(true, data["has_more"])
	s.Equal(float64(1), data["total_in_page"])
}

func (s *CouponCodesSuite) TestListRejectsDecimalLimit() {
	res, err := s.call("marketing/promotions/coupon/codes/list", map[string]any{
		"promotion_id": float64(11),
		"limit":        float64(10.5),
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "limit must be a positive integer")
}

// ---------------------------------------------------------------------------
// create_single — charset, immutability messaging, parent guards
// ---------------------------------------------------------------------------

func (s *CouponCodesSuite) TestCreateSingleRejectsInvalidCharset() {
	res, err := s.call("marketing/promotions/coupon/codes/create_single", map[string]any{
		"promotion_id": float64(11),
		"code":         "BAD!CODE",
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "invalid character")
}

func (s *CouponCodesSuite) TestCreateSingleRejectsDecimalMaxUses() {
	res, err := s.call("marketing/promotions/coupon/codes/create_single", map[string]any{
		"promotion_id": float64(11),
		"code":         "WELCOME10",
		"max_uses":     float64(1.25),
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "max_uses must be a non-negative integer")
}

func (s *CouponCodesSuite) TestCreateSingleRejectsTooLong() {
	long := ""
	for i := 0; i < 51; i++ {
		long += "A"
	}
	res, err := s.call("marketing/promotions/coupon/codes/create_single", map[string]any{
		"promotion_id": float64(11),
		"code":         long,
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "exceeds 50 characters")
}

func (s *CouponCodesSuite) TestCreateSinglePreviewSurfacesImmutabilityNotice() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 11).
		Return(&bigcommerce.Promotion{ID: 11, RedemptionType: "COUPON", CouponType: "SINGLE"}, nil)

	res, err := s.call("marketing/promotions/coupon/codes/create_single", map[string]any{
		"promotion_id": float64(11),
		"code":         "WELCOME10",
		"max_uses":     float64(5),
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	msg, _ := data["message"].(string)
	s.Contains(msg, "IMMUTABLE")
	s.Contains(msg, "delete and recreate")
}

func (s *CouponCodesSuite) TestCreateSingleSurfacesParentMaxUsesOverrideWarning() {
	parentMax := 100
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 11).
		Return(&bigcommerce.Promotion{
			ID:             11,
			RedemptionType: "COUPON",
			CouponType:     "SINGLE",
			MaxUses:        &parentMax,
		}, nil)

	res, err := s.call("marketing/promotions/coupon/codes/create_single", map[string]any{
		"promotion_id": float64(11),
		"code":         "WELCOME10",
	})
	s.NoError(err)
	data := s.parseJSON(res)
	warns, _ := data["warnings"].([]any)
	s.Require().NotEmpty(warns)
	s.Contains(warns[0].(string), "OVERRIDES")
}

func (s *CouponCodesSuite) TestCreateSingleRejectsAutomaticParent() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 11).
		Return(&bigcommerce.Promotion{ID: 11, RedemptionType: "AUTOMATIC"}, nil)

	res, err := s.call("marketing/promotions/coupon/codes/create_single", map[string]any{
		"promotion_id": float64(11),
		"code":         "X",
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "AUTOMATIC")
}

func (s *CouponCodesSuite) TestCreateSingleExecutesOnConfirm() {
	gomock.InOrder(
		s.mockBC.EXPECT().
			GetPromotion(gomock.Any(), 11).
			Return(&bigcommerce.Promotion{ID: 11, RedemptionType: "COUPON", CouponType: "SINGLE"}, nil),
		s.mockBC.EXPECT().
			CreateCouponCode(gomock.Any(), 11, gomock.Any()).
			Return(&bigcommerce.CouponCode{ID: 200, Code: "WELCOME10"}, nil),
	)

	res, err := s.call("marketing/promotions/coupon/codes/create_single", map[string]any{
		"promotion_id": float64(11),
		"code":         "WELCOME10",
		"confirmed":    true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("created", data["status"])
}

// ---------------------------------------------------------------------------
// generate_bulk — BC caps, parent BULK guard
// ---------------------------------------------------------------------------

func (s *CouponCodesSuite) TestGenerateBulkRejectsBatchSizeAboveCap() {
	res, err := s.call("marketing/promotions/coupon/codes/generate_bulk", map[string]any{
		"promotion_id": float64(11),
		"batch_size":   float64(251),
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "cannot exceed BigCommerce's per-call limit of 250")
}

func (s *CouponCodesSuite) TestGenerateBulkRejectsBadLength() {
	res, err := s.call("marketing/promotions/coupon/codes/generate_bulk", map[string]any{
		"promotion_id": float64(11),
		"batch_size":   float64(10),
		"length":       float64(20),
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "length must be between 6 and 16")
}

func (s *CouponCodesSuite) TestGenerateBulkRejectsBadFormat() {
	res, err := s.call("marketing/promotions/coupon/codes/generate_bulk", map[string]any{
		"promotion_id": float64(11),
		"batch_size":   float64(10),
		"format":       "EMOJI",
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "format must be one of")
}

func (s *CouponCodesSuite) TestGenerateBulkRefusesSingleCouponPromotion() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 11).
		Return(&bigcommerce.Promotion{ID: 11, RedemptionType: "COUPON", CouponType: "SINGLE"}, nil)

	res, err := s.call("marketing/promotions/coupon/codes/generate_bulk", map[string]any{
		"promotion_id": float64(11),
		"batch_size":   float64(10),
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "coupon_type=BULK")
}

func (s *CouponCodesSuite) TestGenerateBulkRefusesAutomaticParent() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 11).
		Return(&bigcommerce.Promotion{ID: 11, RedemptionType: "AUTOMATIC"}, nil)

	res, err := s.call("marketing/promotions/coupon/codes/generate_bulk", map[string]any{
		"promotion_id": float64(11),
		"batch_size":   float64(10),
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "AUTOMATIC")
}

func (s *CouponCodesSuite) TestGenerateBulkPreviewBlocksWithoutConfirm() {
	s.mockBC.EXPECT().
		GetPromotion(gomock.Any(), 11).
		Return(&bigcommerce.Promotion{
			ID:             11,
			Name:           "Big Sale",
			RedemptionType: "COUPON",
			CouponType:     "BULK",
			Status:         "ENABLED",
		}, nil)

	res, err := s.call("marketing/promotions/coupon/codes/generate_bulk", map[string]any{
		"promotion_id": float64(11),
		"batch_size":   float64(50),
		"length":       float64(8),
		"format":       "ALPHANUMERIC",
		"prefix":       "SUM",
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	parent, _ := data["parent"].(map[string]any)
	s.Equal("BULK", parent["coupon_type"])
}

func (s *CouponCodesSuite) TestGenerateBulkExecutesOnConfirm() {
	gomock.InOrder(
		s.mockBC.EXPECT().
			GetPromotion(gomock.Any(), 11).
			Return(&bigcommerce.Promotion{ID: 11, RedemptionType: "COUPON", CouponType: "BULK"}, nil),
		s.mockBC.EXPECT().
			GenerateCouponCodes(gomock.Any(), 11, gomock.Any()).
			Return(&bigcommerce.CodeGenResult{ID: 1, BatchSize: 2}, nil),
	)

	res, err := s.call("marketing/promotions/coupon/codes/generate_bulk", map[string]any{
		"promotion_id": float64(11),
		"batch_size":   float64(2),
		"length":       float64(6),
		"format":       "ALPHANUMERIC",
		"prefix":       "SUM",
		"confirmed":    true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("generated", data["status"])
	s.Equal(float64(2), data["generated_count"])
	s.Contains(data["message"], "codes/list")
}

// ---------------------------------------------------------------------------
// delete — batch cap, preview, confirm
// ---------------------------------------------------------------------------

func (s *CouponCodesSuite) TestDeleteRejectsAboveCap() {
	ids := make([]any, 41)
	for i := range ids {
		ids[i] = float64(i + 1)
	}
	res, err := s.call("marketing/promotions/coupon/codes/delete", map[string]any{
		"promotion_id": float64(11),
		"code_ids":     ids,
		"confirmed":    true,
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(textOf(res), "exceeds max of 40")
}

func (s *CouponCodesSuite) TestDeletePreviewBlocksWithoutConfirm() {
	res, err := s.call("marketing/promotions/coupon/codes/delete", map[string]any{
		"promotion_id": float64(11),
		"code_ids":     []any{float64(100), float64(101)},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	s.Equal(float64(2), data["would_delete_ct"])
}

func (s *CouponCodesSuite) TestDeleteExecutesOnConfirm() {
	s.mockBC.EXPECT().
		DeleteCouponCodes(gomock.Any(), 11, []int{100, 101}).
		Return(nil)

	res, err := s.call("marketing/promotions/coupon/codes/delete", map[string]any{
		"promotion_id": float64(11),
		"code_ids":     []any{float64(100), float64(101)},
		"confirmed":    true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("deleted", data["status"])
}
