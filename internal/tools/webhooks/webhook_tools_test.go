package webhooks_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/tools/webhooks"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type WebhookToolsSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockWebhooksAPI
	wt     *webhooks.Webhooks
	reg    *discovery.Registry
}

func TestWebhookToolsSuite(t *testing.T) {
	suite.Run(t, new(WebhookToolsSuite))
}

func (s *WebhookToolsSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockWebhooksAPI(s.ctrl)
	s.wt = webhooks.NewWebhooks(s.mockBC)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("webhooks", "Webhooks")
	s.wt.RegisterTools(s.reg)
}

func (s *WebhookToolsSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *WebhookToolsSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found in registry", toolPath)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: toolPath, Arguments: args},
	}
	return def.Handler(context.Background(), req)
}

func (s *WebhookToolsSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

// --- webhooks/list ---

func (s *WebhookToolsSuite) TestListReturnsWebhooks() {
	s.mockBC.EXPECT().ListWebhooks(gomock.Any(), map[string]string(nil)).Return([]bigcommerce.Webhook{
		{ID: 1, Scope: "store/order/created", Destination: "https://example.com/hook", IsActive: true},
		{ID: 2, Scope: "store/product/updated", Destination: "https://example.com/hook2", IsActive: false},
	}, nil)

	res, err := s.callTool("webhooks/list", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(2), data["total"])
}

func (s *WebhookToolsSuite) TestListFiltersByScope() {
	s.mockBC.EXPECT().ListWebhooks(gomock.Any(), map[string]string{"scope": "store/order/created"}).Return(
		[]bigcommerce.Webhook{{ID: 1, Scope: "store/order/created", Destination: "https://example.com/hook", IsActive: true}},
		nil,
	)

	res, err := s.callTool("webhooks/list", map[string]any{"scope": "store/order/created"})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *WebhookToolsSuite) TestListFiltersByIsActive() {
	s.mockBC.EXPECT().ListWebhooks(gomock.Any(), map[string]string{"is_active": "true"}).Return(
		[]bigcommerce.Webhook{{ID: 1, Scope: "store/order/created", Destination: "https://example.com/hook", IsActive: true}},
		nil,
	)

	res, err := s.callTool("webhooks/list", map[string]any{"is_active": true})
	s.NoError(err)
	s.False(res.IsError)
}

func (s *WebhookToolsSuite) TestListRedactsHeaderValues() {
	s.mockBC.EXPECT().ListWebhooks(gomock.Any(), map[string]string(nil)).Return([]bigcommerce.Webhook{
		{ID: 1, Scope: "store/order/created", Destination: "https://example.com/hook", IsActive: true,
			Headers: map[string]string{"X-Auth": "live-token"}},
	}, nil)

	res, err := s.callTool("webhooks/list", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	hooks := data["webhooks"].([]any)
	wh := hooks[0].(map[string]any)
	headers := wh["headers"].(map[string]any)
	s.Equal("(redacted)", headers["X-Auth"], "header values must be redacted in list")
}

func (s *WebhookToolsSuite) TestListFiltersByChannelID() {
	s.mockBC.EXPECT().ListWebhooks(gomock.Any(), map[string]string{"channel_id": "1763061"}).Return(
		[]bigcommerce.Webhook{{ID: 3, Scope: "store/order/created", Destination: "https://example.com/hook", IsActive: true, ChannelID: 1763061}},
		nil,
	)

	res, err := s.callTool("webhooks/list", map[string]any{"channel_id": float64(1763061)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

// --- webhooks/get ---

func (s *WebhookToolsSuite) TestGetReturnsWebhook() {
	s.mockBC.EXPECT().GetWebhook(gomock.Any(), 42).Return(&bigcommerce.Webhook{
		ID: 42, Scope: "store/order/created", Destination: "https://example.com/hook", IsActive: true,
	}, nil)

	res, err := s.callTool("webhooks/get", map[string]any{"id": float64(42)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	wh := data["webhook"].(map[string]any)
	s.Equal(float64(42), wh["id"])
	s.Equal("store/order/created", wh["scope"])
}

func (s *WebhookToolsSuite) TestGetRejectsMissingID() {
	res, err := s.callTool("webhooks/get", map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *WebhookToolsSuite) TestGetRedactsHeaderValues() {
	s.mockBC.EXPECT().GetWebhook(gomock.Any(), 42).Return(&bigcommerce.Webhook{
		ID: 42, Scope: "store/order/created", Destination: "https://example.com/hook", IsActive: true,
		Headers: map[string]string{"X-Auth": "super-secret", "X-Sig": "hmac-key"},
	}, nil)

	res, err := s.callTool("webhooks/get", map[string]any{"id": float64(42)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	wh := data["webhook"].(map[string]any)
	headers := wh["headers"].(map[string]any)
	s.Equal("(redacted)", headers["X-Auth"], "header values must be redacted")
	s.Equal("(redacted)", headers["X-Sig"], "header values must be redacted")
}

// --- webhooks/events ---

func (s *WebhookToolsSuite) TestEventsReturnsDeliveryHistory() {
	s.mockBC.EXPECT().GetWebhookEvents(gomock.Any(), 42).Return([]bigcommerce.WebhookEvent{
		{ID: 101, Scope: "store/order/created"},
		{ID: 102, Scope: "store/order/created"},
	}, nil)

	res, err := s.callTool("webhooks/events", map[string]any{"id": float64(42)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(42), data["hook_id"])
	s.Equal(float64(2), data["total"])
}

// --- webhooks/create ---

func (s *WebhookToolsSuite) TestCreatePreviewShowsPayload() {
	res, err := s.callTool("webhooks/create", map[string]any{
		"scope":       "store/order/created",
		"destination": "https://example.com/hook",
		"confirmed":   false,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("pending_confirmation", data["status"])
	payload := data["payload"].(map[string]any)
	s.Equal("store/order/created", payload["scope"])
	s.Equal("https://example.com/hook", payload["destination"])
	s.Equal(true, payload["is_active"])
}

func (s *WebhookToolsSuite) TestCreateConfirmedCallsAPI() {
	s.mockBC.EXPECT().CreateWebhook(gomock.Any(), bigcommerce.WebhookCreate{
		Scope:       "store/order/created",
		Destination: "https://example.com/hook",
		IsActive:    true,
	}).Return(&bigcommerce.Webhook{
		ID: 99, Scope: "store/order/created", Destination: "https://example.com/hook", IsActive: true,
	}, nil)

	res, err := s.callTool("webhooks/create", map[string]any{
		"scope":       "store/order/created",
		"destination": "https://example.com/hook",
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("created", data["status"])
	wh := data["webhook"].(map[string]any)
	s.Equal(float64(99), wh["id"])
}

func (s *WebhookToolsSuite) TestCreateWithChannelIDAndHeaders() {
	s.mockBC.EXPECT().CreateWebhook(gomock.Any(), bigcommerce.WebhookCreate{
		Scope:       "store/order/created",
		Destination: "https://example.com/hook",
		IsActive:    true,
		ChannelID:   1763061,
		Headers:     map[string]string{"X-Auth": "secret"},
	}).Return(&bigcommerce.Webhook{
		ID: 100, Scope: "store/order/created", Destination: "https://example.com/hook", IsActive: true, ChannelID: 1763061,
	}, nil)

	res, err := s.callTool("webhooks/create", map[string]any{
		"scope":        "store/order/created",
		"destination":  "https://example.com/hook",
		"channel_id":   float64(1763061),
		"headers_json": `{"X-Auth":"secret"}`,
		"confirmed":    true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("created", data["status"])
}

func (s *WebhookToolsSuite) TestCreatePreviewRedactsHeaders() {
	res, err := s.callTool("webhooks/create", map[string]any{
		"scope":        "store/order/created",
		"destination":  "https://example.com/hook",
		"headers_json": `{"X-Auth":"super-secret"}`,
		"confirmed":    false,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("pending_confirmation", data["status"])
	payload := data["payload"].(map[string]any)
	headers := payload["headers"].(map[string]any)
	s.Equal("(redacted)", headers["X-Auth"], "header values must be redacted in create preview")
}

func (s *WebhookToolsSuite) TestCreateRejectsHTTPDestination() {
	res, err := s.callTool("webhooks/create", map[string]any{
		"scope":       "store/order/created",
		"destination": "http://example.com/hook",
		"confirmed":   false,
	})
	s.NoError(err)
	s.True(res.IsError)
	text := res.Content[0].(mcp.TextContent).Text
	s.Contains(text, "HTTPS")
}

func (s *WebhookToolsSuite) TestCreateRejectsMissingScope() {
	res, err := s.callTool("webhooks/create", map[string]any{
		"destination": "https://example.com/hook",
		"confirmed":   false,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *WebhookToolsSuite) TestCreateRejectsInvalidHeadersJSON() {
	res, err := s.callTool("webhooks/create", map[string]any{
		"scope":        "store/order/created",
		"destination":  "https://example.com/hook",
		"headers_json": `not-json`,
		"confirmed":    false,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *WebhookToolsSuite) TestCreateRejectsNonStringHeaderValues() {
	res, err := s.callTool("webhooks/create", map[string]any{
		"scope":        "store/order/created",
		"destination":  "https://example.com/hook",
		"headers_json": `{"X-Auth": 123}`,
		"confirmed":    false,
	})
	s.NoError(err)
	s.True(res.IsError)
}

// --- webhooks/update ---

func (s *WebhookToolsSuite) TestUpdatePreviewShowsCurrentVsWouldApply() {
	s.mockBC.EXPECT().GetWebhook(gomock.Any(), 42).Return(&bigcommerce.Webhook{
		ID: 42, Scope: "store/order/created", Destination: "https://old.com/hook", IsActive: true,
	}, nil)

	res, err := s.callTool("webhooks/update", map[string]any{
		"id":          float64(42),
		"destination": "https://new.com/hook",
		"confirmed":   false,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("pending_confirmation", data["status"])
	current := data["current"].(map[string]any)
	s.Equal("https://old.com/hook", current["destination"])
	wouldApply := data["would_apply"].(map[string]any)
	s.Equal("https://new.com/hook", wouldApply["destination"])
}

func (s *WebhookToolsSuite) TestUpdateConfirmedMergesAndPuts() {
	s.mockBC.EXPECT().GetWebhook(gomock.Any(), 42).Return(&bigcommerce.Webhook{
		ID: 42, Scope: "store/order/created", Destination: "https://old.com/hook", IsActive: true,
	}, nil)
	s.mockBC.EXPECT().UpdateWebhook(gomock.Any(), 42, bigcommerce.WebhookUpdate{
		Scope:       "store/order/created",
		Destination: "https://new.com/hook",
		IsActive:    true,
	}).Return(&bigcommerce.Webhook{
		ID: 42, Scope: "store/order/created", Destination: "https://new.com/hook", IsActive: true,
	}, nil)

	res, err := s.callTool("webhooks/update", map[string]any{
		"id":          float64(42),
		"destination": "https://new.com/hook",
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("updated", data["status"])
}

func (s *WebhookToolsSuite) TestUpdateTogglesIsActive() {
	s.mockBC.EXPECT().GetWebhook(gomock.Any(), 42).Return(&bigcommerce.Webhook{
		ID: 42, Scope: "store/order/created", Destination: "https://example.com/hook", IsActive: true,
	}, nil)
	s.mockBC.EXPECT().UpdateWebhook(gomock.Any(), 42, bigcommerce.WebhookUpdate{
		Scope:       "store/order/created",
		Destination: "https://example.com/hook",
		IsActive:    false,
	}).Return(&bigcommerce.Webhook{
		ID: 42, Scope: "store/order/created", Destination: "https://example.com/hook", IsActive: false,
	}, nil)

	res, err := s.callTool("webhooks/update", map[string]any{
		"id":        float64(42),
		"is_active": false,
		"confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("updated", data["status"])
}

func (s *WebhookToolsSuite) TestUpdatePreviewRedactsHeadersInCurrentAndWouldApply() {
	s.mockBC.EXPECT().GetWebhook(gomock.Any(), 42).Return(&bigcommerce.Webhook{
		ID: 42, Scope: "store/order/created", Destination: "https://old.com/hook", IsActive: true,
		Headers: map[string]string{"X-Auth": "existing-secret"},
	}, nil)

	res, err := s.callTool("webhooks/update", map[string]any{
		"id":           float64(42),
		"headers_json": `{"X-Auth":"new-secret"}`,
		"confirmed":    false,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("pending_confirmation", data["status"])

	current := data["current"].(map[string]any)
	currentHeaders := current["headers"].(map[string]any)
	s.Equal("(redacted)", currentHeaders["X-Auth"], "current header values must be redacted in update preview")

	wouldApply := data["would_apply"].(map[string]any)
	newHeaders := wouldApply["headers"].(map[string]any)
	s.Equal("(redacted)", newHeaders["X-Auth"], "would_apply header values must be redacted in update preview")
}

func (s *WebhookToolsSuite) TestUpdateRejectsNoFields() {
	res, err := s.callTool("webhooks/update", map[string]any{
		"id":        float64(42),
		"confirmed": false,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *WebhookToolsSuite) TestUpdateRejectsHTTPDestination() {
	res, err := s.callTool("webhooks/update", map[string]any{
		"id":          float64(42),
		"destination": "http://insecure.com/hook",
		"confirmed":   false,
	})
	s.NoError(err)
	s.True(res.IsError)
	text := res.Content[0].(mcp.TextContent).Text
	s.Contains(text, "HTTPS")
}

// --- webhooks/delete ---

func (s *WebhookToolsSuite) TestDeletePreviewShowsWebhookDetails() {
	s.mockBC.EXPECT().GetWebhook(gomock.Any(), 42).Return(&bigcommerce.Webhook{
		ID: 42, Scope: "store/order/created", Destination: "https://example.com/hook", IsActive: true,
	}, nil)

	res, err := s.callTool("webhooks/delete", map[string]any{
		"id":        float64(42),
		"confirmed": false,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(42), data["hook_id"])
	s.Equal("store/order/created", data["scope"])
	s.Contains(data["message"].(string), "cannot be undone")
}

func (s *WebhookToolsSuite) TestDeleteConfirmedCallsAPI() {
	s.mockBC.EXPECT().GetWebhook(gomock.Any(), 42).Return(&bigcommerce.Webhook{
		ID: 42, Scope: "store/order/created", Destination: "https://example.com/hook", IsActive: true,
	}, nil)
	s.mockBC.EXPECT().DeleteWebhook(gomock.Any(), 42).Return(nil)

	res, err := s.callTool("webhooks/delete", map[string]any{
		"id":        float64(42),
		"confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("deleted", data["status"])
	s.Equal(float64(42), data["hook_id"])
}

func (s *WebhookToolsSuite) TestDeleteRejectsMissingID() {
	res, err := s.callTool("webhooks/delete", map[string]any{"confirmed": true})
	s.NoError(err)
	s.True(res.IsError)
}
