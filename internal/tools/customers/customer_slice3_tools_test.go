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

type CustomerSliceThreeSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	mockBC   *MockBigCommerceCustomersAPI
	registry *discovery.Registry
}

func TestCustomerSliceThreeSuite(t *testing.T) {
	suite.Run(t, new(CustomerSliceThreeSuite))
}

func (s *CustomerSliceThreeSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceCustomersAPI(s.ctrl)
	s.registry = discovery.NewRegistry()
	s.registry.RegisterCategory("customers", "Customers")
	s.registry.RegisterCategory("customers/settings", "Settings")
	s.registry.RegisterCategory("customers/settings/global", "Global")
	s.registry.RegisterCategory("customers/settings/channel", "Channel")
	s.registry.RegisterCategory("customers/consent", "Consent")
	s.registry.RegisterCategory("customers/stored_instruments", "Stored")
	s.registry.RegisterCategory("customers/credentials", "Creds")

	customers.NewCustomerSettings(s.mockBC).RegisterTools(s.registry)
	customers.NewCustomerConsentTools(s.mockBC).RegisterTools(s.registry)
	customers.NewCustomerStoredInstruments(s.mockBC).RegisterTools(s.registry)
	customers.NewCustomerValidateCredentials(s.mockBC).RegisterTools(s.registry)
}

func (s *CustomerSliceThreeSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *CustomerSliceThreeSuite) callTool(path string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.registry.GetTool(path)
	s.Require().NotNil(def)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: path, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *CustomerSliceThreeSuite) parseJSON(res *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(res)
	s.Require().False(res.IsError)
	txt := res.Content[0].(mcp.TextContent).Text
	var m map[string]any
	s.Require().NoError(json.Unmarshal([]byte(txt), &m))
	return m
}

func (s *CustomerSliceThreeSuite) TestGlobalSettingsGet() {
	s.mockBC.EXPECT().GetGlobalCustomerSettings(gomock.Any()).
		Return(&bigcommerce.CustomerGlobalSettings{}, nil)
	res, err := s.callTool("customers/settings/global/get", map[string]any{})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Contains(data, "settings")
}

func (s *CustomerSliceThreeSuite) TestChannelUpdateAllowGlobalRejectsWithoutConfirmFlag() {
	s.mockBC.EXPECT().GetChannelCustomerSettings(gomock.Any(), 2).Return(&bigcommerce.CustomerChannelSettings{}, nil)
	res, err := s.callTool("customers/settings/channel/update", map[string]any{
		"channel_id": float64(2),
		"settings":   map[string]any{"allow_global_logins": true},
		"confirmed":  true,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CustomerSliceThreeSuite) TestChannelUpdateAllowGlobalSucceedsWithBothFlags() {
	s.mockBC.EXPECT().GetChannelCustomerSettings(gomock.Any(), 2).Return(&bigcommerce.CustomerChannelSettings{}, nil)
	s.mockBC.EXPECT().UpdateChannelCustomerSettings(gomock.Any(), 2, gomock.Any()).
		Return(&bigcommerce.CustomerChannelSettings{AllowGlobalLogins: boolPtr(true)}, nil)
	res, err := s.callTool("customers/settings/channel/update", map[string]any{
		"channel_id":                  float64(2),
		"settings":                    map[string]any{"allow_global_logins": true},
		"confirm_allow_global_logins": true,
		"confirmed":                   true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("updated", data["status"])
}

func (s *CustomerSliceThreeSuite) TestConsentUpdatePreview() {
	s.mockBC.EXPECT().GetCustomerConsent(gomock.Any(), 5).
		Return(&bigcommerce.CustomerConsent{Allow: []string{"essential"}, Deny: []string{}}, nil)
	res, err := s.callTool("customers/consent/update", map[string]any{
		"customer_id": float64(5),
		"allow":       []any{"essential", "analytics"},
		"deny":        []any{},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *CustomerSliceThreeSuite) TestStoredInstrumentsRequiresAck() {
	res, err := s.callTool("customers/stored_instruments/list", map[string]any{
		"customer_id": float64(1),
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CustomerSliceThreeSuite) TestStoredInstrumentsRedactsToken() {
	raw := []json.RawMessage{json.RawMessage(`{"type":"stored_card","token":"secret","last_4":"4242"}`)}
	s.mockBC.EXPECT().ListCustomerStoredInstruments(gomock.Any(), 9).Return(raw, nil)
	res, err := s.callTool("customers/stored_instruments/list", map[string]any{
		"customer_id":                    float64(9),
		"acknowledge_stored_instruments": true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	arr := data["stored_instruments"].([]any)
	s.Require().Len(arr, 1)
	row := arr[0].(map[string]any)
	s.Equal("(redacted)", row["token"])
}

func (s *CustomerSliceThreeSuite) TestValidateCredentialsPreview() {
	res, err := s.callTool("customers/credentials/validate", map[string]any{
		"email": "a@b.com", "password": "x",
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	s.Contains(data["email"].(string), "@")
}

func boolPtr(b bool) *bool {
	return &b
}
