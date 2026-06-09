package webhooks

import (
	"context"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
)

// Compile-time check that *bigcommerce.Client satisfies WebhooksAPI.
var _ WebhooksAPI = (*bigcommerce.Client)(nil)

// WebhooksAPI defines the BigCommerce client methods used by webhook tool
// handlers. Defined on the consumer side per Go convention so tests can
// provide a mock without depending on the full client implementation.
type WebhooksAPI interface {
	ListWebhooks(ctx context.Context, params map[string]string) ([]bigcommerce.Webhook, error)
	GetWebhook(ctx context.Context, hookID int) (*bigcommerce.Webhook, error)
	GetWebhookEvents(ctx context.Context, hookID int) ([]bigcommerce.WebhookEvent, error)
	CreateWebhook(ctx context.Context, payload bigcommerce.WebhookCreate) (*bigcommerce.Webhook, error)
	UpdateWebhook(ctx context.Context, hookID int, payload bigcommerce.WebhookUpdate) (*bigcommerce.Webhook, error)
	DeleteWebhook(ctx context.Context, hookID int) error
}
