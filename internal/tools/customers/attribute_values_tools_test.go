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

type CustomerAttributeValuesHandlerSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	mockBC   *MockBigCommerceCustomersAPI
	values   *customers.CustomerAttributeValues
	registry *discovery.Registry
}

func TestCustomerAttributeValuesHandlerSuite(t *testing.T) {
	suite.Run(t, new(CustomerAttributeValuesHandlerSuite))
}

func (s *CustomerAttributeValuesHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceCustomersAPI(s.ctrl)
	s.values = customers.NewCustomerAttributeValues(s.mockBC)
	s.registry = discovery.NewRegistry()
	s.registry.RegisterCategory("customers", "Customers")
	s.registry.RegisterCategory("customers/attribute_values", "Attribute Values")
	s.values.RegisterTools(s.registry)
}

func (s *CustomerAttributeValuesHandlerSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *CustomerAttributeValuesHandlerSuite) callTool(path string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.registry.GetTool(path)
	s.Require().NotNil(def)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: path, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *CustomerAttributeValuesHandlerSuite) parseJSON(res *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(res)
	s.Require().False(res.IsError)
	s.Require().NotEmpty(res.Content)
	txt := res.Content[0].(mcp.TextContent).Text
	var m map[string]any
	s.Require().NoError(json.Unmarshal([]byte(txt), &m))
	return m
}

func (s *CustomerAttributeValuesHandlerSuite) TestListRequiresFilterOrListAll() {
	res, err := s.callTool("customers/attribute_values/list", map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CustomerAttributeValuesHandlerSuite) TestListByCustomerID() {
	s.mockBC.EXPECT().SearchCustomerAttributeValues(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, p map[string]string) ([]bigcommerce.CustomerAttributeValue, error) {
			s.Equal("11,12", p["customer_id:in"])
			return []bigcommerce.CustomerAttributeValue{{ID: 1, AttributeID: 5, CustomerID: 11, AttributeValue: "v"}}, nil
		})
	res, err := s.callTool("customers/attribute_values/list", map[string]any{
		"customer_ids": []any{float64(11), float64(12)},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *CustomerAttributeValuesHandlerSuite) TestUpsertPreviewThenConfirm() {
	res, err := s.callTool("customers/attribute_values/upsert", map[string]any{
		"value_batch": []any{
			map[string]any{"customer_id": float64(7), "attribute_id": float64(3), "value": "gold"},
		},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])

	s.mockBC.EXPECT().UpsertCustomerAttributeValues(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, payload []bigcommerce.CustomerAttributeValueUpsert) ([]bigcommerce.CustomerAttributeValue, error) {
			s.Require().Len(payload, 1)
			s.Equal(7, payload[0].CustomerID)
			s.Equal(3, payload[0].AttributeID)
			s.Equal("gold", payload[0].Value)
			return []bigcommerce.CustomerAttributeValue{{ID: 99, AttributeID: 3, CustomerID: 7, AttributeValue: "gold"}}, nil
		})
	res, err = s.callTool("customers/attribute_values/upsert", map[string]any{
		"value_batch": []any{
			map[string]any{"customer_id": float64(7), "attribute_id": float64(3), "value": "gold"},
		},
		"confirmed": true,
	})
	s.NoError(err)
	data = s.parseJSON(res)
	s.Equal("upserted", data["status"])
}

func (s *CustomerAttributeValuesHandlerSuite) TestUpsertRejectsMissingFields() {
	res, err := s.callTool("customers/attribute_values/upsert", map[string]any{
		"value_batch": []any{
			map[string]any{"customer_id": float64(7), "value": "gold"},
		},
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CustomerAttributeValuesHandlerSuite) TestDeletePreviewThenConfirm() {
	res, err := s.callTool("customers/attribute_values/delete", map[string]any{
		"value_ids": []any{float64(11), float64(12)},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])

	s.mockBC.EXPECT().DeleteCustomerAttributeValues(gomock.Any(), []int{11, 12}).Return(nil)
	res, err = s.callTool("customers/attribute_values/delete", map[string]any{
		"value_ids": []any{float64(11), float64(12)}, "confirmed": true,
	})
	s.NoError(err)
	data = s.parseJSON(res)
	s.Equal("deleted", data["status"])
}
