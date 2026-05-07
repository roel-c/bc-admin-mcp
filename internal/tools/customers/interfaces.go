// Package customers contains MCP tool handlers for BigCommerce customer-domain
// resources: Customer Groups (V2), customer records, addresses, attributes,
// attribute values, metafields, settings, consent, stored instruments,
// credential validation, customer segments, and shopper profiles (V3).
package customers

import (
	"context"
	"encoding/json"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
)

// Compile-time check that *bigcommerce.Client satisfies BigCommerceCustomersAPI.
var _ BigCommerceCustomersAPI = (*bigcommerce.Client)(nil)

// BigCommerceCustomersAPI defines the BigCommerce client methods used by
// customer-domain tool handlers. Defined consumer-side per Go convention so
// tests can mock it without depending on the full client implementation.
type BigCommerceCustomersAPI interface {
	// Customer Groups (V2: /v2/customer_groups)
	ListCustomerGroups(ctx context.Context, params map[string]string) ([]bigcommerce.CustomerGroup, error)
	GetCustomerGroup(ctx context.Context, id int) (*bigcommerce.CustomerGroup, error)
	CountCustomerGroups(ctx context.Context) (int, error)
	CreateCustomerGroup(ctx context.Context, payload bigcommerce.CustomerGroupCreate) (*bigcommerce.CustomerGroup, error)
	UpdateCustomerGroup(ctx context.Context, id int, payload bigcommerce.CustomerGroupUpdate) (*bigcommerce.CustomerGroup, error)
	DeleteCustomerGroup(ctx context.Context, id int) error

	// Customers (V3: /v3/customers)
	SearchCustomers(ctx context.Context, params map[string]string) ([]bigcommerce.Customer, error)
	GetCustomersByIDs(ctx context.Context, ids []int) ([]bigcommerce.Customer, error)
	CreateCustomers(ctx context.Context, payload []bigcommerce.CustomerCreate) ([]bigcommerce.Customer, error)
	UpdateCustomers(ctx context.Context, payload []bigcommerce.CustomerUpdate) ([]bigcommerce.Customer, error)
	DeleteCustomers(ctx context.Context, ids []int) error

	// Customer addresses (V3: /v3/customers/addresses)
	SearchCustomerAddresses(ctx context.Context, params map[string]string) ([]bigcommerce.CustomerAddress, error)
	CreateCustomerAddresses(ctx context.Context, payload []bigcommerce.CustomerAddressCreate) ([]bigcommerce.CustomerAddress, error)
	UpdateCustomerAddresses(ctx context.Context, payload []bigcommerce.CustomerAddressUpdate) ([]bigcommerce.CustomerAddress, error)
	DeleteCustomerAddresses(ctx context.Context, ids []int) error

	// Customer attributes (V3: /v3/customers/attributes)
	SearchCustomerAttributes(ctx context.Context, params map[string]string) ([]bigcommerce.CustomerAttribute, error)
	GetCustomerAttributesByIDs(ctx context.Context, ids []int) ([]bigcommerce.CustomerAttribute, error)
	CreateCustomerAttributes(ctx context.Context, payload []bigcommerce.CustomerAttributeCreate) ([]bigcommerce.CustomerAttribute, error)
	UpdateCustomerAttributes(ctx context.Context, payload []bigcommerce.CustomerAttributeUpdate) ([]bigcommerce.CustomerAttribute, error)
	DeleteCustomerAttributes(ctx context.Context, ids []int) error

	// Customer attribute values (V3: /v3/customers/attribute-values)
	SearchCustomerAttributeValues(ctx context.Context, params map[string]string) ([]bigcommerce.CustomerAttributeValue, error)
	UpsertCustomerAttributeValues(ctx context.Context, payload []bigcommerce.CustomerAttributeValueUpsert) ([]bigcommerce.CustomerAttributeValue, error)
	DeleteCustomerAttributeValues(ctx context.Context, ids []int) error

	// Customer metafields (V3: /v3/customers/{customerId}/metafields and /v3/customers/metafields)
	ListCustomerMetafields(ctx context.Context, customerID int) ([]bigcommerce.Metafield, error)
	CreateCustomerMetafield(ctx context.Context, customerID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	UpdateCustomerMetafield(ctx context.Context, customerID, metafieldID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	DeleteCustomerMetafield(ctx context.Context, customerID, metafieldID int) error
	SearchAllCustomerMetafields(ctx context.Context, params map[string]string) ([]bigcommerce.Metafield, error)
	CreateCustomerMetafieldsBatch(ctx context.Context, payload []bigcommerce.Metafield) ([]bigcommerce.Metafield, error)
	UpdateCustomerMetafieldsBatch(ctx context.Context, payload []bigcommerce.Metafield) ([]bigcommerce.Metafield, error)
	DeleteCustomerMetafieldsBatch(ctx context.Context, ids []int) error

	// Customer settings (V3: /v3/customers/settings, /v3/customers/settings/channels/{id})
	GetGlobalCustomerSettings(ctx context.Context) (*bigcommerce.CustomerGlobalSettings, error)
	UpdateGlobalCustomerSettings(ctx context.Context, payload bigcommerce.CustomerGlobalSettings) (*bigcommerce.CustomerGlobalSettings, error)
	GetChannelCustomerSettings(ctx context.Context, channelID int) (*bigcommerce.CustomerChannelSettings, error)
	UpdateChannelCustomerSettings(ctx context.Context, channelID int, payload bigcommerce.CustomerChannelSettings) (*bigcommerce.CustomerChannelSettings, error)

	// Consent (V3: /v3/customers/{id}/consent)
	GetCustomerConsent(ctx context.Context, customerID int) (*bigcommerce.CustomerConsent, error)
	UpdateCustomerConsent(ctx context.Context, customerID int, req bigcommerce.DeclareCustomerConsentRequest) (*bigcommerce.CustomerConsent, error)

	// Stored instruments (V3: /v3/customers/{id}/stored-instruments)
	ListCustomerStoredInstruments(ctx context.Context, customerID int) ([]json.RawMessage, error)

	// Validate credentials (V3: POST /v3/customers/validate-credentials)
	ValidateCustomerCredentials(ctx context.Context, req bigcommerce.ValidateCustomerCredentialsRequest) (*bigcommerce.ValidateCustomerCredentialsResponse, error)

	// Customer segmentation — segments (V3: /v3/segments)
	SearchSegments(ctx context.Context, params map[string]string) ([]bigcommerce.Segment, error)
	GetSegmentsByIDs(ctx context.Context, ids []string) ([]bigcommerce.Segment, error)
	CreateSegments(ctx context.Context, payload []bigcommerce.SegmentCreate) ([]bigcommerce.Segment, error)
	UpdateSegments(ctx context.Context, payload []bigcommerce.SegmentUpdate) ([]bigcommerce.Segment, error)
	DeleteSegments(ctx context.Context, ids []string) error

	// Customer segmentation — shopper profiles in a segment (V3: /v3/segments/{id}/shopper-profiles)
	ListShopperProfilesInSegment(ctx context.Context, segmentID string) ([]bigcommerce.ShopperProfile, error)
	AddShopperProfilesToSegment(ctx context.Context, segmentID string, profileIDs []string) ([]bigcommerce.ShopperProfile, error)
	RemoveShopperProfilesFromSegment(ctx context.Context, segmentID string, profileIDs []string) error

	// Customer segmentation — shopper profiles (V3: /v3/shopper-profiles)
	ListShopperProfiles(ctx context.Context, params map[string]string) ([]bigcommerce.ShopperProfile, error)
	CreateShopperProfiles(ctx context.Context, payload []bigcommerce.ShopperProfileCreate) ([]bigcommerce.ShopperProfile, error)
	DeleteShopperProfiles(ctx context.Context, ids []string) error
	ListSegmentsForShopperProfile(ctx context.Context, profileID string) ([]bigcommerce.Segment, error)
}
