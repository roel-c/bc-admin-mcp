// Hand-written mock matching gomock's expectation/recorder API. Mirrors the
// shape used by mockgen so existing patterns (s.mockBC.EXPECT().Method(...).Return(...))
// keep working without an external generator step.

package promotions_test

import (
	context "context"
	"encoding/json"
	reflect "reflect"

	bigcommerce "github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	gomock "go.uber.org/mock/gomock"
)

// MockBigCommercePromotionsAPI is a mock of the BigCommercePromotionsAPI interface.
type MockBigCommercePromotionsAPI struct {
	ctrl     *gomock.Controller
	recorder *MockBigCommercePromotionsAPIMockRecorder
}

// MockBigCommercePromotionsAPIMockRecorder records expected calls.
type MockBigCommercePromotionsAPIMockRecorder struct {
	mock *MockBigCommercePromotionsAPI
}

// NewMockBigCommercePromotionsAPI creates a new mock instance.
func NewMockBigCommercePromotionsAPI(ctrl *gomock.Controller) *MockBigCommercePromotionsAPI {
	mock := &MockBigCommercePromotionsAPI{ctrl: ctrl}
	mock.recorder = &MockBigCommercePromotionsAPIMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBigCommercePromotionsAPI) EXPECT() *MockBigCommercePromotionsAPIMockRecorder {
	return m.recorder
}

// SearchPromotions mocks base method.
func (m *MockBigCommercePromotionsAPI) SearchPromotions(ctx context.Context, params bigcommerce.PromotionListParams) ([]bigcommerce.Promotion, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SearchPromotions", ctx, params)
	ret0, _ := ret[0].([]bigcommerce.Promotion)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SearchPromotions indicates an expected call of SearchPromotions.
func (mr *MockBigCommercePromotionsAPIMockRecorder) SearchPromotions(ctx, params any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SearchPromotions", reflect.TypeOf((*MockBigCommercePromotionsAPI)(nil).SearchPromotions), ctx, params)
}

// GetPromotion mocks base method.
func (m *MockBigCommercePromotionsAPI) GetPromotion(ctx context.Context, id int) (*bigcommerce.Promotion, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPromotion", ctx, id)
	ret0, _ := ret[0].(*bigcommerce.Promotion)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetPromotion indicates an expected call of GetPromotion.
func (mr *MockBigCommercePromotionsAPIMockRecorder) GetPromotion(ctx, id any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPromotion", reflect.TypeOf((*MockBigCommercePromotionsAPI)(nil).GetPromotion), ctx, id)
}

// CreatePromotion mocks base method.
func (m *MockBigCommercePromotionsAPI) CreatePromotion(ctx context.Context, payload json.RawMessage) (*bigcommerce.Promotion, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreatePromotion", ctx, payload)
	ret0, _ := ret[0].(*bigcommerce.Promotion)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreatePromotion indicates an expected call of CreatePromotion.
func (mr *MockBigCommercePromotionsAPIMockRecorder) CreatePromotion(ctx, payload any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreatePromotion", reflect.TypeOf((*MockBigCommercePromotionsAPI)(nil).CreatePromotion), ctx, payload)
}

// UpdatePromotion mocks base method.
func (m *MockBigCommercePromotionsAPI) UpdatePromotion(ctx context.Context, id int, payload json.RawMessage) (*bigcommerce.Promotion, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdatePromotion", ctx, id, payload)
	ret0, _ := ret[0].(*bigcommerce.Promotion)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UpdatePromotion indicates an expected call of UpdatePromotion.
func (mr *MockBigCommercePromotionsAPIMockRecorder) UpdatePromotion(ctx, id, payload any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdatePromotion", reflect.TypeOf((*MockBigCommercePromotionsAPI)(nil).UpdatePromotion), ctx, id, payload)
}

// DeletePromotionsByIDs mocks base method.
func (m *MockBigCommercePromotionsAPI) DeletePromotionsByIDs(ctx context.Context, ids []int) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeletePromotionsByIDs", ctx, ids)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeletePromotionsByIDs indicates an expected call of DeletePromotionsByIDs.
func (mr *MockBigCommercePromotionsAPIMockRecorder) DeletePromotionsByIDs(ctx, ids any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeletePromotionsByIDs", reflect.TypeOf((*MockBigCommercePromotionsAPI)(nil).DeletePromotionsByIDs), ctx, ids)
}

// ListCouponCodes mocks base method.
func (m *MockBigCommercePromotionsAPI) ListCouponCodes(ctx context.Context, promotionID int, params bigcommerce.CouponCodeListParams) (*bigcommerce.CouponCodeListResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListCouponCodes", ctx, promotionID, params)
	ret0, _ := ret[0].(*bigcommerce.CouponCodeListResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListCouponCodes indicates an expected call of ListCouponCodes.
func (mr *MockBigCommercePromotionsAPIMockRecorder) ListCouponCodes(ctx, promotionID, params any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListCouponCodes", reflect.TypeOf((*MockBigCommercePromotionsAPI)(nil).ListCouponCodes), ctx, promotionID, params)
}

// CreateCouponCode mocks base method.
func (m *MockBigCommercePromotionsAPI) CreateCouponCode(ctx context.Context, promotionID int, payload bigcommerce.CouponCodeCreate) (*bigcommerce.CouponCode, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateCouponCode", ctx, promotionID, payload)
	ret0, _ := ret[0].(*bigcommerce.CouponCode)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateCouponCode indicates an expected call of CreateCouponCode.
func (mr *MockBigCommercePromotionsAPIMockRecorder) CreateCouponCode(ctx, promotionID, payload any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateCouponCode", reflect.TypeOf((*MockBigCommercePromotionsAPI)(nil).CreateCouponCode), ctx, promotionID, payload)
}

// DeleteCouponCodes mocks base method.
func (m *MockBigCommercePromotionsAPI) DeleteCouponCodes(ctx context.Context, promotionID int, ids []int) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteCouponCodes", ctx, promotionID, ids)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteCouponCodes indicates an expected call of DeleteCouponCodes.
func (mr *MockBigCommercePromotionsAPIMockRecorder) DeleteCouponCodes(ctx, promotionID, ids any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteCouponCodes", reflect.TypeOf((*MockBigCommercePromotionsAPI)(nil).DeleteCouponCodes), ctx, promotionID, ids)
}

// GenerateCouponCodes mocks base method.
func (m *MockBigCommercePromotionsAPI) GenerateCouponCodes(ctx context.Context, promotionID int, req bigcommerce.CodeGenRequest) (*bigcommerce.CodeGenResult, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GenerateCouponCodes", ctx, promotionID, req)
	ret0, _ := ret[0].(*bigcommerce.CodeGenResult)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GenerateCouponCodes indicates an expected call of GenerateCouponCodes.
func (mr *MockBigCommercePromotionsAPIMockRecorder) GenerateCouponCodes(ctx, promotionID, req any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GenerateCouponCodes", reflect.TypeOf((*MockBigCommercePromotionsAPI)(nil).GenerateCouponCodes), ctx, promotionID, req)
}

// GetPromotionSettings mocks base method.
func (m *MockBigCommercePromotionsAPI) GetPromotionSettings(ctx context.Context) (*bigcommerce.PromotionSettings, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPromotionSettings", ctx)
	ret0, _ := ret[0].(*bigcommerce.PromotionSettings)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetPromotionSettings indicates an expected call of GetPromotionSettings.
func (mr *MockBigCommercePromotionsAPIMockRecorder) GetPromotionSettings(ctx any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPromotionSettings", reflect.TypeOf((*MockBigCommercePromotionsAPI)(nil).GetPromotionSettings), ctx)
}

// UpdatePromotionSettings mocks base method.
func (m *MockBigCommercePromotionsAPI) UpdatePromotionSettings(ctx context.Context, payload bigcommerce.PromotionSettings) (*bigcommerce.PromotionSettings, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdatePromotionSettings", ctx, payload)
	ret0, _ := ret[0].(*bigcommerce.PromotionSettings)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UpdatePromotionSettings indicates an expected call of UpdatePromotionSettings.
func (mr *MockBigCommercePromotionsAPIMockRecorder) UpdatePromotionSettings(ctx, payload any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdatePromotionSettings", reflect.TypeOf((*MockBigCommercePromotionsAPI)(nil).UpdatePromotionSettings), ctx, payload)
}
