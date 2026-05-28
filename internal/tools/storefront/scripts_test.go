package storefront_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/tools/storefront"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// --------------------------------------------------------------------------
// Suite bootstrap
// --------------------------------------------------------------------------

type ScriptHandlerSuite struct {
	suite.Suite
	ctrl    *gomock.Controller
	mockBC  *MockScriptAPI
	scripts *storefront.Scripts
	reg     *discovery.Registry
}

func TestScriptHandlerSuite(t *testing.T) {
	suite.Run(t, new(ScriptHandlerSuite))
}

func (s *ScriptHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockScriptAPI(s.ctrl)
	s.scripts = storefront.NewScripts(s.mockBC)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("storefront", "Storefront")
	s.reg.RegisterCategory("storefront/scripts", "Scripts")
	s.scripts.RegisterTools(s.reg)
}

func (s *ScriptHandlerSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *ScriptHandlerSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found in registry", toolPath)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolPath,
			Arguments: args,
		},
	}
	return def.Handler(context.Background(), req)
}

func (s *ScriptHandlerSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

// --------------------------------------------------------------------------
// list
// --------------------------------------------------------------------------

func (s *ScriptHandlerSuite) TestListReturnsScripts() {
	s.mockBC.EXPECT().ListScripts(gomock.Any(), gomock.Any()).Return([]bigcommerce.Script{
		{UUID: "abc-123", Name: "Analytics", Kind: "src", Enabled: true},
		{UUID: "def-456", Name: "Chat Widget", Kind: "script_tag", Enabled: false},
	}, nil)

	result, err := s.callTool("storefront/scripts/list", map[string]any{})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal(float64(2), data["total"])
}

func (s *ScriptHandlerSuite) TestListWithChannelID() {
	s.mockBC.EXPECT().ListScripts(gomock.Any(), bigcommerce.ScriptListParams{ChannelID: 7}).
		Return([]bigcommerce.Script{{UUID: "abc-123", Name: "Channel Script"}}, nil)

	result, err := s.callTool("storefront/scripts/list", map[string]any{
		"channel_id": float64(7),
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal(float64(1), data["total"])
}

func (s *ScriptHandlerSuite) TestListPropagatesError() {
	s.mockBC.EXPECT().ListScripts(gomock.Any(), gomock.Any()).Return(nil, errors.New("api error"))

	result, err := s.callTool("storefront/scripts/list", map[string]any{})
	s.NoError(err)
	s.True(result.IsError)
}

// --------------------------------------------------------------------------
// get
// --------------------------------------------------------------------------

func (s *ScriptHandlerSuite) TestGetReturnsScript() {
	s.mockBC.EXPECT().GetScript(gomock.Any(), "abc-123").Return(&bigcommerce.Script{
		UUID: "abc-123", Name: "Analytics", Kind: "src", Enabled: true,
	}, nil)

	result, err := s.callTool("storefront/scripts/get", map[string]any{
		"uuid": "abc-123",
	})
	s.NoError(err)
	s.False(result.IsError)
}

func (s *ScriptHandlerSuite) TestGetRequiresUUID() {
	result, err := s.callTool("storefront/scripts/get", map[string]any{})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *ScriptHandlerSuite) TestGetPropagatesError() {
	s.mockBC.EXPECT().GetScript(gomock.Any(), "missing-uuid").Return(nil, errors.New("not found"))

	result, err := s.callTool("storefront/scripts/get", map[string]any{
		"uuid": "missing-uuid",
	})
	s.NoError(err)
	s.True(result.IsError)
}

// --------------------------------------------------------------------------
// create
// --------------------------------------------------------------------------

func (s *ScriptHandlerSuite) TestCreatePreviewSrcScript() {
	result, err := s.callTool("storefront/scripts/create", map[string]any{
		"name": "My Analytics",
		"kind": "src",
		"src":  "https://cdn.example.com/analytics.js",
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
}

func (s *ScriptHandlerSuite) TestCreatePreviewInlineScript() {
	result, err := s.callTool("storefront/scripts/create", map[string]any{
		"name": "Inline Tracker",
		"kind": "script_tag",
		"html": "<script>console.log('hello')</script>",
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
}

func (s *ScriptHandlerSuite) TestCreateRequiresName() {
	result, err := s.callTool("storefront/scripts/create", map[string]any{
		"kind": "src",
		"src":  "https://cdn.example.com/analytics.js",
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *ScriptHandlerSuite) TestCreateRejectsSrcWithHTML() {
	result, err := s.callTool("storefront/scripts/create", map[string]any{
		"name": "Bad Script",
		"kind": "src",
		"src":  "https://cdn.example.com/analytics.js",
		"html": "<script>bad</script>",
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *ScriptHandlerSuite) TestCreateRejectsScriptTagWithSrc() {
	result, err := s.callTool("storefront/scripts/create", map[string]any{
		"name": "Bad Script",
		"kind": "script_tag",
		"src":  "https://cdn.example.com/analytics.js",
		"html": "<script>ok</script>",
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *ScriptHandlerSuite) TestCreateRejectsSrcWithoutURL() {
	result, err := s.callTool("storefront/scripts/create", map[string]any{
		"name": "Bad Script",
		"kind": "src",
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *ScriptHandlerSuite) TestCreateScriptTagWithoutHTMLReturnsScaffoldPreview() {
	// kind=script_tag without html should return a pending_confirmation preview
	// with a script_scaffold (not an error). The BC API enforces html presence
	// on the actual POST; the preview step surfaces the scaffold first so the
	// LLM can fill it in before confirming.
	result, err := s.callTool("storefront/scripts/create", map[string]any{
		"name": "My Script",
		"kind": "script_tag",
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.NotEmpty(data["script_scaffold"], "scaffold should be surfaced when html is absent")
}

func (s *ScriptHandlerSuite) TestCreateExecuteSrcScript() {
	s.mockBC.EXPECT().CreateScript(gomock.Any(), gomock.Any()).Return(&bigcommerce.Script{
		UUID: "new-uuid-001", Name: "My Analytics", Kind: "src", Enabled: true,
	}, nil)

	result, err := s.callTool("storefront/scripts/create", map[string]any{
		"name":      "My Analytics",
		"kind":      "src",
		"src":       "https://cdn.example.com/analytics.js",
		"confirmed": true,
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("created", data["status"])
	script := data["script"].(map[string]any)
	s.Equal("new-uuid-001", script["uuid"])
}

func (s *ScriptHandlerSuite) TestCreateExecuteInlineScript() {
	s.mockBC.EXPECT().CreateScript(gomock.Any(), gomock.Any()).Return(&bigcommerce.Script{
		UUID: "new-uuid-002", Name: "Inline Tracker", Kind: "script_tag", Enabled: true,
	}, nil)

	result, err := s.callTool("storefront/scripts/create", map[string]any{
		"name":      "Inline Tracker",
		"kind":      "script_tag",
		"html":      "<script>console.log('hi')</script>",
		"confirmed": true,
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("created", data["status"])
}

func (s *ScriptHandlerSuite) TestCreateCheckoutVisibilityAddsWarnings() {
	// Preview should include warnings for checkout scope.
	result, err := s.callTool("storefront/scripts/create", map[string]any{
		"name":       "Checkout Tracker",
		"kind":       "src",
		"src":        "https://cdn.example.com/checkout.js",
		"visibility": "checkout",
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.NotEmpty(data["warnings"])
}

func (s *ScriptHandlerSuite) TestCreateAllPagesVisibilityAddsWarning() {
	result, err := s.callTool("storefront/scripts/create", map[string]any{
		"name":       "All Pages Script",
		"kind":       "src",
		"src":        "https://cdn.example.com/all.js",
		"visibility": "all_pages",
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.NotEmpty(data["warnings"])
}

func (s *ScriptHandlerSuite) TestCreatePropagatesError() {
	s.mockBC.EXPECT().CreateScript(gomock.Any(), gomock.Any()).Return(nil, errors.New("api error"))

	result, err := s.callTool("storefront/scripts/create", map[string]any{
		"name":      "Fail Script",
		"kind":      "src",
		"src":       "https://cdn.example.com/fail.js",
		"confirmed": true,
	})
	s.NoError(err)
	s.True(result.IsError)
}

// --------------------------------------------------------------------------
// update
// --------------------------------------------------------------------------

func (s *ScriptHandlerSuite) TestUpdatePreview() {
	result, err := s.callTool("storefront/scripts/update", map[string]any{
		"uuid": "abc-123",
		"name": "Updated Name",
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal("abc-123", data["uuid"])
}

func (s *ScriptHandlerSuite) TestUpdateRequiresUUID() {
	result, err := s.callTool("storefront/scripts/update", map[string]any{
		"name": "Updated Name",
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *ScriptHandlerSuite) TestUpdateRejectsEmptyPayload() {
	// Providing only uuid with no updateable fields should error.
	result, err := s.callTool("storefront/scripts/update", map[string]any{
		"uuid": "abc-123",
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *ScriptHandlerSuite) TestUpdateExecute() {
	updatedName := "Renamed Script"
	s.mockBC.EXPECT().UpdateScript(gomock.Any(), "abc-123", gomock.Any()).Return(&bigcommerce.Script{
		UUID: "abc-123", Name: updatedName, Kind: "src", Enabled: true,
	}, nil)

	result, err := s.callTool("storefront/scripts/update", map[string]any{
		"uuid":      "abc-123",
		"name":      updatedName,
		"confirmed": true,
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("updated", data["status"])
	script := data["script"].(map[string]any)
	s.Equal(updatedName, script["name"])
}

func (s *ScriptHandlerSuite) TestUpdateCheckoutVisibilityAddsWarnings() {
	result, err := s.callTool("storefront/scripts/update", map[string]any{
		"uuid":       "abc-123",
		"visibility": "checkout",
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.NotEmpty(data["warnings"])
}

func (s *ScriptHandlerSuite) TestUpdatePropagatesError() {
	s.mockBC.EXPECT().UpdateScript(gomock.Any(), "abc-123", gomock.Any()).Return(nil, errors.New("not found"))

	result, err := s.callTool("storefront/scripts/update", map[string]any{
		"uuid":      "abc-123",
		"name":      "New Name",
		"confirmed": true,
	})
	s.NoError(err)
	s.True(result.IsError)
}

// --------------------------------------------------------------------------
// delete
// --------------------------------------------------------------------------

func (s *ScriptHandlerSuite) TestDeletePreview() {
	result, err := s.callTool("storefront/scripts/delete", map[string]any{
		"uuid": "abc-123",
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal("abc-123", data["uuid"])
}

func (s *ScriptHandlerSuite) TestDeleteRequiresUUID() {
	result, err := s.callTool("storefront/scripts/delete", map[string]any{})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *ScriptHandlerSuite) TestDeleteExecute() {
	s.mockBC.EXPECT().DeleteScript(gomock.Any(), "abc-123").Return(nil)

	result, err := s.callTool("storefront/scripts/delete", map[string]any{
		"uuid":      "abc-123",
		"confirmed": true,
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("deleted", data["status"])
	s.Equal("abc-123", data["uuid"])
}

func (s *ScriptHandlerSuite) TestDeletePropagatesError() {
	s.mockBC.EXPECT().DeleteScript(gomock.Any(), "abc-123").Return(errors.New("api error"))

	result, err := s.callTool("storefront/scripts/delete", map[string]any{
		"uuid":      "abc-123",
		"confirmed": true,
	})
	s.NoError(err)
	s.True(result.IsError)
}

// --------------------------------------------------------------------------
// toggle
// --------------------------------------------------------------------------

func (s *ScriptHandlerSuite) TestTogglePreviewEnable() {
	result, err := s.callTool("storefront/scripts/toggle", map[string]any{
		"uuid":    "abc-123",
		"enabled": true,
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(true, data["enabled"])
}

func (s *ScriptHandlerSuite) TestTogglePreviewDisable() {
	result, err := s.callTool("storefront/scripts/toggle", map[string]any{
		"uuid":    "abc-123",
		"enabled": false,
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(false, data["enabled"])
}

func (s *ScriptHandlerSuite) TestToggleRequiresUUID() {
	result, err := s.callTool("storefront/scripts/toggle", map[string]any{
		"enabled": true,
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *ScriptHandlerSuite) TestToggleRequiresEnabled() {
	result, err := s.callTool("storefront/scripts/toggle", map[string]any{
		"uuid": "abc-123",
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *ScriptHandlerSuite) TestToggleExecuteEnable() {
	s.mockBC.EXPECT().UpdateScript(gomock.Any(), "abc-123", gomock.Any()).Return(&bigcommerce.Script{
		UUID: "abc-123", Name: "My Script", Enabled: true,
	}, nil)

	result, err := s.callTool("storefront/scripts/toggle", map[string]any{
		"uuid":      "abc-123",
		"enabled":   true,
		"confirmed": true,
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("enabled", data["status"])
}

func (s *ScriptHandlerSuite) TestToggleExecuteDisable() {
	s.mockBC.EXPECT().UpdateScript(gomock.Any(), "abc-123", gomock.Any()).Return(&bigcommerce.Script{
		UUID: "abc-123", Name: "My Script", Enabled: false,
	}, nil)

	result, err := s.callTool("storefront/scripts/toggle", map[string]any{
		"uuid":      "abc-123",
		"enabled":   false,
		"confirmed": true,
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("disabled", data["status"])
}

func (s *ScriptHandlerSuite) TestTogglePropagatesError() {
	s.mockBC.EXPECT().UpdateScript(gomock.Any(), "abc-123", gomock.Any()).Return(nil, errors.New("api error"))

	result, err := s.callTool("storefront/scripts/toggle", map[string]any{
		"uuid":      "abc-123",
		"enabled":   true,
		"confirmed": true,
	})
	s.NoError(err)
	s.True(result.IsError)
}
