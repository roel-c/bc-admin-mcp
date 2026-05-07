package customers_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/tools/customers"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type CustomerMetafieldsHandlerSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	mockBC   *MockBigCommerceCustomersAPI
	mfs      *customers.CustomerMetafields
	registry *discovery.Registry
}

func TestCustomerMetafieldsHandlerSuite(t *testing.T) {
	suite.Run(t, new(CustomerMetafieldsHandlerSuite))
}

func (s *CustomerMetafieldsHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceCustomersAPI(s.ctrl)
	s.mfs = customers.NewCustomerMetafields(s.mockBC)
	s.registry = discovery.NewRegistry()
	s.registry.RegisterCategory("customers", "Customers")
	s.registry.RegisterCategory("customers/metafields", "Metafields")
	s.mfs.RegisterTools(s.registry)
}

func (s *CustomerMetafieldsHandlerSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *CustomerMetafieldsHandlerSuite) callTool(path string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.registry.GetTool(path)
	s.Require().NotNil(def)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: path, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *CustomerMetafieldsHandlerSuite) parseJSON(res *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(res)
	s.Require().False(res.IsError)
	s.Require().NotEmpty(res.Content)
	txt := res.Content[0].(mcp.TextContent).Text
	var m map[string]any
	s.Require().NoError(json.Unmarshal([]byte(txt), &m))
	return m
}

func (s *CustomerMetafieldsHandlerSuite) TestListPerCustomer() {
	s.mockBC.EXPECT().ListCustomerMetafields(gomock.Any(), 42).
		Return([]bigcommerce.Metafield{{ID: 1, Namespace: "n", Key: "k", Value: "v"}}, nil)
	res, err := s.callTool("customers/metafields/list", map[string]any{"customer_id": float64(42)})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal(float64(42), data["customer_id"])
	s.Equal(float64(1), data["total"])
}

func (s *CustomerMetafieldsHandlerSuite) TestListAcrossCustomersRequiresFilter() {
	res, err := s.callTool("customers/metafields/list", map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CustomerMetafieldsHandlerSuite) TestListAcrossCustomersWithNamespace() {
	s.mockBC.EXPECT().SearchAllCustomerMetafields(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, p map[string]string) ([]bigcommerce.Metafield, error) {
			s.Equal("loyalty", p["namespace"])
			return []bigcommerce.Metafield{{ID: 7, Namespace: "loyalty", Key: "tier", Value: "gold"}}, nil
		})
	res, err := s.callTool("customers/metafields/list", map[string]any{"namespace": "loyalty"})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *CustomerMetafieldsHandlerSuite) TestSetCreatePreview() {
	s.mockBC.EXPECT().ListCustomerMetafields(gomock.Any(), 1).Return([]bigcommerce.Metafield{}, nil)
	res, err := s.callTool("customers/metafields/set", map[string]any{
		"customer_id": float64(1), "namespace": "loyalty", "key": "tier", "value": "gold",
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("pending_confirmation", data["status"])
	s.Equal("create", data["action"])
	s.Equal("app_only", data["permission_set"])
}

func (s *CustomerMetafieldsHandlerSuite) TestSetUpdateConfirmed() {
	existing := []bigcommerce.Metafield{{ID: 100, Namespace: "loyalty", Key: "tier", Value: "silver", PermissionSet: "read"}}
	s.mockBC.EXPECT().ListCustomerMetafields(gomock.Any(), 1).Return(existing, nil)
	s.mockBC.EXPECT().UpdateCustomerMetafield(gomock.Any(), 1, 100, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ int, _ int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error) {
			s.Equal("read", mf.PermissionSet)
			s.Equal("gold", mf.Value)
			return &bigcommerce.Metafield{ID: 100, Namespace: "loyalty", Key: "tier", Value: "gold", PermissionSet: "read"}, nil
		})
	res, err := s.callTool("customers/metafields/set", map[string]any{
		"customer_id": float64(1), "namespace": "loyalty", "key": "tier", "value": "gold", "confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("updated", data["status"])
}

func (s *CustomerMetafieldsHandlerSuite) TestDeleteByNamespaceKey() {
	existing := []bigcommerce.Metafield{{ID: 50, Namespace: "loyalty", Key: "tier"}}
	s.mockBC.EXPECT().ListCustomerMetafields(gomock.Any(), 1).Return(existing, nil)
	res, err := s.callTool("customers/metafields/delete", map[string]any{
		"customer_id": float64(1), "namespace": "loyalty", "key": "tier",
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(50), data["metafield_id"])

	s.mockBC.EXPECT().ListCustomerMetafields(gomock.Any(), 1).Return(existing, nil)
	s.mockBC.EXPECT().DeleteCustomerMetafield(gomock.Any(), 1, 50).Return(nil)
	res, err = s.callTool("customers/metafields/delete", map[string]any{
		"customer_id": float64(1), "namespace": "loyalty", "key": "tier", "confirmed": true,
	})
	s.NoError(err)
	data = s.parseJSON(res)
	s.Equal("deleted", data["status"])
}

func (s *CustomerMetafieldsHandlerSuite) TestBulkSetPreview() {
	for _, cid := range []int{1, 2} {
		s.mockBC.EXPECT().ListCustomerMetafields(gomock.Any(), cid).Return([]bigcommerce.Metafield{}, nil)
	}
	res, err := s.callTool("customers/metafields/bulk_set", map[string]any{
		"customer_ids": []any{float64(1), float64(2)},
		"namespace":    "loyalty", "key": "tier", "value": "gold",
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(2), data["customer_count"])
}

func (s *CustomerMetafieldsHandlerSuite) TestBulkSetConfirmedMixedCreateUpdate() {
	s.mockBC.EXPECT().ListCustomerMetafields(gomock.Any(), 1).
		Return([]bigcommerce.Metafield{{ID: 10, Namespace: "loyalty", Key: "tier", Value: "silver", PermissionSet: "read"}}, nil)
	s.mockBC.EXPECT().ListCustomerMetafields(gomock.Any(), 2).Return([]bigcommerce.Metafield{}, nil)

	s.mockBC.EXPECT().UpdateCustomerMetafield(gomock.Any(), 1, 10, gomock.Any()).
		Return(&bigcommerce.Metafield{ID: 10, Namespace: "loyalty", Key: "tier", Value: "gold", PermissionSet: "read"}, nil)
	s.mockBC.EXPECT().CreateCustomerMetafield(gomock.Any(), 2, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error) {
			s.Equal("app_only", mf.PermissionSet)
			return &bigcommerce.Metafield{ID: 11, Namespace: "loyalty", Key: "tier", Value: "gold", PermissionSet: "app_only"}, nil
		})

	res, err := s.callTool("customers/metafields/bulk_set", map[string]any{
		"customer_ids": []any{float64(1), float64(2)},
		"namespace":    "loyalty", "key": "tier", "value": "gold",
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("completed", data["status"])
	s.Equal(float64(2), data["succeeded"])
}

func (s *CustomerMetafieldsHandlerSuite) TestBulkDeleteSkipsMissing() {
	s.mockBC.EXPECT().ListCustomerMetafields(gomock.Any(), 1).
		Return([]bigcommerce.Metafield{{ID: 10, Namespace: "loyalty", Key: "tier"}}, nil)
	s.mockBC.EXPECT().ListCustomerMetafields(gomock.Any(), 2).Return([]bigcommerce.Metafield{}, nil)
	s.mockBC.EXPECT().DeleteCustomerMetafield(gomock.Any(), 1, 10).Return(nil)

	res, err := s.callTool("customers/metafields/bulk_delete", map[string]any{
		"customer_ids": []any{float64(1), float64(2)},
		"namespace":    "loyalty", "key": "tier",
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("completed", data["status"])
	s.Equal(float64(1), data["succeeded"])
	s.Equal(float64(1), data["skipped"])
}
