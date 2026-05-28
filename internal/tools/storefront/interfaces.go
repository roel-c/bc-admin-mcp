package storefront

import (
	"context"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
)

// Compile-time check that *bigcommerce.Client satisfies ScriptAPI.
var _ ScriptAPI = (*bigcommerce.Client)(nil)

// ScriptAPI defines the BigCommerce client methods used by storefront script
// tool handlers. Defined on the consumer side per Go convention so tests can
// provide a mock without depending on the full client implementation.
type ScriptAPI interface {
	ListScripts(ctx context.Context, params bigcommerce.ScriptListParams) ([]bigcommerce.Script, error)
	GetScript(ctx context.Context, uuid string) (*bigcommerce.Script, error)
	CreateScript(ctx context.Context, payload bigcommerce.ScriptCreate) (*bigcommerce.Script, error)
	UpdateScript(ctx context.Context, uuid string, payload bigcommerce.ScriptUpdate) (*bigcommerce.Script, error)
	DeleteScript(ctx context.Context, uuid string) error
}
