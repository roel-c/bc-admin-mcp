package catalog_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type ProductResolveSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
}

func TestProductResolveSuite(t *testing.T) {
	suite.Run(t, new(ProductResolveSuite))
}

func (s *ProductResolveSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
}

func (s *ProductResolveSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *ProductResolveSuite) TestFetchByIDs() {
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{1, 2}).Return([]bigcommerce.Product{
		{ID: 1, Name: "A"},
		{ID: 2, Name: "B"},
	}, nil)

	products, err := catalog.FetchProductsForWrite(context.Background(), s.mockBC, []int{1, 2}, "", "")
	s.NoError(err)
	s.Len(products, 2)
}

func (s *ProductResolveSuite) TestFetchByIDsNotFound() {
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{1, 999}).Return([]bigcommerce.Product{
		{ID: 1, Name: "A"},
	}, nil)

	_, err := catalog.FetchProductsForWrite(context.Background(), s.mockBC, []int{1, 999}, "", "")
	s.Error(err)
	s.Contains(err.Error(), "999")
}

func (s *ProductResolveSuite) TestFetchByIDsInvalidID() {
	_, err := catalog.FetchProductsForWrite(context.Background(), s.mockBC, []int{-1}, "", "")
	s.Error(err)
	s.Contains(err.Error(), "must be positive")
}

func (s *ProductResolveSuite) TestFetchBySKU() {
	s.mockBC.EXPECT().SearchProducts(gomock.Any(), map[string]string{"sku": "ABC-123"}).
		Return([]bigcommerce.Product{{ID: 42, Name: "Widget", SKU: "ABC-123"}}, nil)

	products, err := catalog.FetchProductsForWrite(context.Background(), s.mockBC, nil, "ABC-123", "")
	s.NoError(err)
	s.Len(products, 1)
	s.Equal(42, products[0].ID)
}

func (s *ProductResolveSuite) TestFetchBySKUNotFound() {
	s.mockBC.EXPECT().SearchProducts(gomock.Any(), map[string]string{"sku": "NOPE"}).
		Return(nil, nil)

	_, err := catalog.FetchProductsForWrite(context.Background(), s.mockBC, nil, "NOPE", "")
	s.Error(err)
	s.Contains(err.Error(), "no product found")
}

func (s *ProductResolveSuite) TestFetchBySKUMultipleMatches() {
	s.mockBC.EXPECT().SearchProducts(gomock.Any(), map[string]string{"sku": "DUP"}).
		Return([]bigcommerce.Product{{ID: 1}, {ID: 2}}, nil)

	_, err := catalog.FetchProductsForWrite(context.Background(), s.mockBC, nil, "DUP", "")
	s.Error(err)
	s.Contains(err.Error(), "multiple products")
}

func (s *ProductResolveSuite) TestFetchByName() {
	s.mockBC.EXPECT().SearchProducts(gomock.Any(), map[string]string{"name": "Widget"}).
		Return([]bigcommerce.Product{{ID: 42, Name: "Widget"}}, nil)

	products, err := catalog.FetchProductsForWrite(context.Background(), s.mockBC, nil, "", "Widget")
	s.NoError(err)
	s.Len(products, 1)
}

func (s *ProductResolveSuite) TestFetchByNameNotFound() {
	s.mockBC.EXPECT().SearchProducts(gomock.Any(), map[string]string{"name": "Ghost"}).
		Return(nil, nil)

	_, err := catalog.FetchProductsForWrite(context.Background(), s.mockBC, nil, "", "Ghost")
	s.Error(err)
	s.Contains(err.Error(), "no product found")
}

func (s *ProductResolveSuite) TestFetchNoModesError() {
	_, err := catalog.FetchProductsForWrite(context.Background(), s.mockBC, nil, "", "")
	s.Error(err)
	s.Contains(err.Error(), "provide product_ids")
}

func (s *ProductResolveSuite) TestFetchMultipleModesError() {
	_, err := catalog.FetchProductsForWrite(context.Background(), s.mockBC, []int{1}, "SKU", "")
	s.Error(err)
	s.Contains(err.Error(), "only one of")
}

func (s *ProductResolveSuite) TestFetchByIDsAPIError() {
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{1}).
		Return(nil, fmt.Errorf("api timeout"))

	_, err := catalog.FetchProductsForWrite(context.Background(), s.mockBC, []int{1}, "", "")
	s.Error(err)
	s.Contains(err.Error(), "api timeout")
}
