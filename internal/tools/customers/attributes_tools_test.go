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

type CustomerAttributesHandlerSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	mockBC   *MockBigCommerceCustomersAPI
	attrs    *customers.CustomerAttributes
	registry *discovery.Registry
}

func TestCustomerAttributesHandlerSuite(t *testing.T) {
	suite.Run(t, new(CustomerAttributesHandlerSuite))
}

func (s *CustomerAttributesHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceCustomersAPI(s.ctrl)
	s.attrs = customers.NewCustomerAttributes(s.mockBC)
	s.registry = discovery.NewRegistry()
	s.registry.RegisterCategory("customers", "Customers")
	s.registry.RegisterCategory("customers/attributes", "Attributes")
	s.attrs.RegisterTools(s.registry)
}

func (s *CustomerAttributesHandlerSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *CustomerAttributesHandlerSuite) callTool(path string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.registry.GetTool(path)
	s.Require().NotNil(def)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: path, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *CustomerAttributesHandlerSuite) parseJSON(res *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(res)
	s.Require().False(res.IsError)
	s.Require().NotEmpty(res.Content)
	txt := res.Content[0].(mcp.TextContent).Text
	var m map[string]any
	s.Require().NoError(json.Unmarshal([]byte(txt), &m))
	return m
}

func (s *CustomerAttributesHandlerSuite) TestListRequiresFilterOrListAll() {
	res, err := s.callTool("customers/attributes/list", map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CustomerAttributesHandlerSuite) TestListByName() {
	s.mockBC.EXPECT().SearchCustomerAttributes(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, p map[string]string) ([]bigcommerce.CustomerAttribute, error) {
			s.Equal("loyalty", p["name"])
			return []bigcommerce.CustomerAttribute{{ID: 1, Name: "loyalty", Type: "string"}}, nil
		})
	res, err := s.callTool("customers/attributes/list", map[string]any{"name": "loyalty"})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *CustomerAttributesHandlerSuite) TestCreateRejectsInvalidType() {
	res, err := s.callTool("customers/attributes/create", map[string]any{
		"name": "bad", "type": "boolean",
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CustomerAttributesHandlerSuite) TestCreatePreviewThenConfirm() {
	res, err := s.callTool("customers/attributes/create", map[string]any{
		"name": "loyalty", "type": "string",
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])

	s.mockBC.EXPECT().CreateCustomerAttributes(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, payload []bigcommerce.CustomerAttributeCreate) ([]bigcommerce.CustomerAttribute, error) {
			s.Require().Len(payload, 1)
			s.Equal("loyalty", payload[0].Name)
			s.Equal("string", payload[0].Type)
			return []bigcommerce.CustomerAttribute{{ID: 5, Name: "loyalty", Type: "string"}}, nil
		})
	res, err = s.callTool("customers/attributes/create", map[string]any{
		"name": "loyalty", "type": "string", "confirmed": true,
	})
	s.NoError(err)
	data = s.parseJSON(res)
	s.Equal("created", data["status"])
}

func (s *CustomerAttributesHandlerSuite) TestUpdateRejectsTypeChange() {
	res, err := s.callTool("customers/attributes/update", map[string]any{
		"attribute_batch": []any{
			map[string]any{"id": float64(1), "name": "x", "type": "number"},
		},
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CustomerAttributesHandlerSuite) TestUpdateRenameOK() {
	s.mockBC.EXPECT().UpdateCustomerAttributes(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, payload []bigcommerce.CustomerAttributeUpdate) ([]bigcommerce.CustomerAttribute, error) {
			s.Require().Len(payload, 1)
			s.Equal(7, payload[0].ID)
			s.Equal("renamed", payload[0].Name)
			return []bigcommerce.CustomerAttribute{{ID: 7, Name: "renamed", Type: "string"}}, nil
		})
	res, err := s.callTool("customers/attributes/update", map[string]any{
		"attribute_batch": []any{
			map[string]any{"id": float64(7), "name": "renamed"},
		},
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("updated", data["status"])
}

func (s *CustomerAttributesHandlerSuite) TestDeletePreviewThenConfirm() {
	s.mockBC.EXPECT().GetCustomerAttributesByIDs(gomock.Any(), []int{3}).
		Return([]bigcommerce.CustomerAttribute{{ID: 3, Name: "old", Type: "string"}}, nil)
	res, err := s.callTool("customers/attributes/delete", map[string]any{
		"attribute_ids": []any{float64(3)},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])

	s.mockBC.EXPECT().DeleteCustomerAttributes(gomock.Any(), []int{3}).Return(nil)
	res, err = s.callTool("customers/attributes/delete", map[string]any{
		"attribute_ids": []any{float64(3)}, "confirmed": true,
	})
	s.NoError(err)
	data = s.parseJSON(res)
	s.Equal("deleted", data["status"])
}
