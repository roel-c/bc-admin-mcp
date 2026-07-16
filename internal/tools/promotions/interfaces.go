package promotions

import (
	"context"
	"encoding/json"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
)

// Compile-time check that *bigcommerce.Client satisfies BigCommercePromotionsAPI.
var _ BigCommercePromotionsAPI = (*bigcommerce.Client)(nil)

// BigCommercePromotionsAPI defines the BigCommerce client methods used by the
// promotions tool handlers. Defined consumer-side (per Go convention) so unit
// tests can substitute a hand-written mock without depending on the full
// client implementation.
//
// Slice 1 covers the promotion-level methods. Slice 2 adds the coupon-code
// sub-resource methods (list / create / delete / generate).
type BigCommercePromotionsAPI interface {
	SearchPromotions(ctx context.Context, params bigcommerce.PromotionListParams) ([]bigcommerce.Promotion, error)
	GetPromotion(ctx context.Context, id int) (*bigcommerce.Promotion, error)
	CreatePromotion(ctx context.Context, payload json.RawMessage) (*bigcommerce.Promotion, error)
	UpdatePromotion(ctx context.Context, id int, payload json.RawMessage) (*bigcommerce.Promotion, error)
	DeletePromotionsByIDs(ctx context.Context, ids []int) error

	// Coupon code sub-resource (slice 2).
	ListCouponCodes(ctx context.Context, promotionID int, params bigcommerce.CouponCodeListParams) (*bigcommerce.CouponCodeListResponse, error)
	CreateCouponCode(ctx context.Context, promotionID int, payload bigcommerce.CouponCodeCreate) (*bigcommerce.CouponCode, error)
	DeleteCouponCodes(ctx context.Context, promotionID int, ids []int) error
	GenerateCouponCodes(ctx context.Context, promotionID int, req bigcommerce.CodeGenRequest) (*bigcommerce.CodeGenResult, error)

	// Store-wide promotion settings (slice 3).
	GetPromotionSettings(ctx context.Context) (*bigcommerce.PromotionSettings, error)
	UpdatePromotionSettings(ctx context.Context, payload bigcommerce.PromotionSettings) (*bigcommerce.PromotionSettings, error)
}
