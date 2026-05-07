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

type CustomerRecordsHandlerSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	mockBC   *MockBigCommerceCustomersAPI
	records  *customers.CustomerRecords
	registry *discovery.Registry
}

func TestCustomerRecordsHandlerSuite(t *testing.T) {
	suite.Run(t, new(CustomerRecordsHandlerSuite))
}

func (s *CustomerRecordsHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceCustomersAPI(s.ctrl)
	s.records = customers.NewCustomerRecords(s.mockBC)
	s.registry = discovery.NewRegistry()
	s.registry.RegisterCategory("customers", "Customers")
	s.registry.RegisterCategory("customers/groups", "Groups")
	s.records.RegisterTools(s.registry)
}

func (s *CustomerRecordsHandlerSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *CustomerRecordsHandlerSuite) callTool(path string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.registry.GetTool(path)
	s.Require().NotNil(def)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: path, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *CustomerRecordsHandlerSuite) parseJSON(res *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(res)
	s.Require().False(res.IsError)
	s.Require().NotEmpty(res.Content)
	txt := res.Content[0].(mcp.TextContent).Text
	var m map[string]any
	s.Require().NoError(json.Unmarshal([]byte(txt), &m))
	return m
}

func (s *CustomerRecordsHandlerSuite) TestListRequiresFilterOrListAll() {
	res, err := s.callTool("customers/list", map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CustomerRecordsHandlerSuite) TestListCallsSearchWithFilters() {
	s.mockBC.EXPECT().SearchCustomers(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, p map[string]string) ([]bigcommerce.Customer, error) {
			s.Equal("a@b.com", p["email:in"])
			return []bigcommerce.Customer{{ID: 1, Email: "a@b.com", FirstName: "A", LastName: "B"}}, nil
		})
	res, err := s.callTool("customers/list", map[string]any{"list_all": false, "email_in": "a@b.com"})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *CustomerRecordsHandlerSuite) TestGetSingleCustomer() {
	s.mockBC.EXPECT().SearchCustomers(gomock.Any(), map[string]string{"id:in": "9"}).
		Return([]bigcommerce.Customer{{ID: 9, Email: "x@y.com", FirstName: "X", LastName: "Y"}}, nil)
	res, err := s.callTool("customers/get", map[string]any{"customer_id": float64(9)})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal(float64(9), data["id"])
}

func (s *CustomerRecordsHandlerSuite) TestCreatePreviewWithoutPassword() {
	res, err := s.callTool("customers/create", map[string]any{
		"email": "n@e.com", "first_name": "N", "last_name": "E",
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *CustomerRecordsHandlerSuite) TestCreateRejectsPasswordWithoutSetPasswordFlag() {
	res, err := s.callTool("customers/create", map[string]any{
		"email": "n@e.com", "first_name": "N", "last_name": "E",
		"new_password": "secret",
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CustomerRecordsHandlerSuite) TestCreateExecutesWithPasswordGates() {
	s.mockBC.EXPECT().CreateCustomers(gomock.Any(), gomock.Any()).
		Return([]bigcommerce.Customer{{ID: 2, Email: "n@e.com", FirstName: "N", LastName: "E"}}, nil)
	res, err := s.callTool("customers/create", map[string]any{
		"email": "n@e.com", "first_name": "N", "last_name": "E",
		"new_password": "secret",
		"set_password": true,
		"confirmed":    true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("created", data["status"])
}

func (s *CustomerRecordsHandlerSuite) TestAssignGroupPreview() {
	s.mockBC.EXPECT().GetCustomersByIDs(gomock.Any(), []int{1, 2}).
		Return([]bigcommerce.Customer{
			{ID: 1, Email: "a@a.com", CustomerGroupID: 5},
			{ID: 2, Email: "b@b.com", CustomerGroupID: 5},
		}, nil)
	res, err := s.callTool("customers/assign_group", map[string]any{
		"customer_ids": []any{float64(1), float64(2)},
		"group_id":     float64(9),
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

type CustomerAddressesHandlerSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	mockBC   *MockBigCommerceCustomersAPI
	addrs    *customers.CustomerAddresses
	registry *discovery.Registry
}

func TestCustomerAddressesHandlerSuite(t *testing.T) {
	suite.Run(t, new(CustomerAddressesHandlerSuite))
}

func (s *CustomerAddressesHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceCustomersAPI(s.ctrl)
	s.addrs = customers.NewCustomerAddresses(s.mockBC)
	s.registry = discovery.NewRegistry()
	s.registry.RegisterCategory("customers", "Customers")
	s.registry.RegisterCategory("customers/addresses", "Addresses")
	s.addrs.RegisterTools(s.registry)
}

func (s *CustomerAddressesHandlerSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *CustomerAddressesHandlerSuite) callTool(path string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.registry.GetTool(path)
	s.Require().NotNil(def)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: path, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *CustomerAddressesHandlerSuite) TestListRequiresFilter() {
	res, err := s.callTool("customers/addresses/list", map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CustomerAddressesHandlerSuite) TestListByCustomerID() {
	s.mockBC.EXPECT().SearchCustomerAddresses(gomock.Any(), map[string]string{"customer_id:in": "3"}).
		Return([]bigcommerce.CustomerAddress{{ID: 10, CustomerID: 3, FirstName: "A", LastName: "B", Address1: "1 St", City: "C", CountryCode: "US"}}, nil)
	res, err := s.callTool("customers/addresses/list", map[string]any{"customer_id": float64(3)})
	s.NoError(err)
	s.False(res.IsError)
}
