package b2b

import (
	"context"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
)

// Compile-time check that *bigcommerce.B2BClient satisfies B2BCompanyAPI.
var _ B2BCompanyAPI = (*bigcommerce.B2BClient)(nil)

// B2BCompanyAPI defines the B2B Edition client methods used by Phase B1
// tool handlers (companies, users, addresses).
type B2BCompanyAPI interface {
	// Companies
	ListB2BCompanies(ctx context.Context, params string) ([]bigcommerce.B2BCompany, error)
	GetB2BCompany(ctx context.Context, companyID int) (*bigcommerce.B2BCompany, error)
	CreateB2BCompany(ctx context.Context, payload bigcommerce.B2BCompanyCreate) (*bigcommerce.B2BCompany, error)
	UpdateB2BCompany(ctx context.Context, companyID int, payload bigcommerce.B2BCompanyUpdate) (*bigcommerce.B2BCompany, error)
	SetB2BCompanyStatus(ctx context.Context, companyID int, action string) (*bigcommerce.B2BCompany, error)
	DeleteB2BCompany(ctx context.Context, companyID int) error
	ListB2BCompanyExtraFields(ctx context.Context, params string) ([]bigcommerce.B2BExtraFieldDef, error)
	UpdateB2BCompanyCatalog(ctx context.Context, companyID int, catalogID string) error
	ListB2BCompanyAttachments(ctx context.Context, companyID int) ([]bigcommerce.B2BAttachment, error)
	AddB2BCompanyAttachment(ctx context.Context, companyID int, fileName string, data []byte) (*bigcommerce.B2BAttachment, error)
	DeleteB2BCompanyAttachment(ctx context.Context, companyID int, attachmentID string) error
	// Users
	ListB2BUsers(ctx context.Context, params string) ([]bigcommerce.B2BUser, error)
	GetB2BUser(ctx context.Context, userID int) (*bigcommerce.B2BUser, error)
	GetB2BUserByCustomerID(ctx context.Context, customerID int) (*bigcommerce.B2BUser, error)
	CreateB2BUser(ctx context.Context, payload bigcommerce.B2BUserCreate) (*bigcommerce.B2BUser, error)
	BulkCreateB2BUsers(ctx context.Context, payloads []bigcommerce.B2BUserCreate) ([]bigcommerce.B2BNewUserID, error)
	UpdateB2BUser(ctx context.Context, userID int, payload bigcommerce.B2BUserUpdate) (*bigcommerce.B2BUser, error)
	DeleteB2BUser(ctx context.Context, userID int) error
	ListB2BUserExtraFields(ctx context.Context, params string) ([]bigcommerce.B2BExtraFieldDef, error)
	// Roles & permissions
	ListB2BRoles(ctx context.Context, params string) ([]bigcommerce.B2BRole, error)
	GetB2BRole(ctx context.Context, roleID int) (*bigcommerce.B2BRole, error)
	CreateB2BRole(ctx context.Context, payload bigcommerce.B2BRoleCreate) (*bigcommerce.B2BRole, error)
	UpdateB2BRole(ctx context.Context, roleID int, payload bigcommerce.B2BRoleCreate) (*bigcommerce.B2BRole, error)
	DeleteB2BRole(ctx context.Context, roleID int) error
	ListB2BPermissions(ctx context.Context, params string) ([]bigcommerce.B2BPermission, error)
	CreateB2BPermission(ctx context.Context, payload bigcommerce.B2BPermissionCreate) (*bigcommerce.B2BPermission, error)
	UpdateB2BPermission(ctx context.Context, permissionID int, payload bigcommerce.B2BPermissionCreate) (*bigcommerce.B2BPermission, error)
	DeleteB2BPermission(ctx context.Context, permissionID int) error
	// Account hierarchies
	ListB2BCompanySubsidiaries(ctx context.Context, companyID int, params string) ([]bigcommerce.B2BHierarchyNode, error)
	ListB2BCompanyHierarchy(ctx context.Context, companyID int, params string) ([]bigcommerce.B2BHierarchyNode, error)
	AttachB2BCompanyParent(ctx context.Context, companyID, parentCompanyID int) error
	DeleteB2BCompanySubsidiary(ctx context.Context, companyID, childCompanyID int) error
	// Channels
	ListB2BChannels(ctx context.Context) ([]bigcommerce.B2BChannel, error)
	GetB2BChannel(ctx context.Context, channelID int) (*bigcommerce.B2BChannel, error)
	// Orders
	GetB2BOrder(ctx context.Context, bcOrderID int) (map[string]any, error)
	UpdateB2BOrder(ctx context.Context, bcOrderID int, payload bigcommerce.B2BOrderUpdate) (map[string]any, error)
	AssignCustomerOrdersToCompany(ctx context.Context, customerID int) error
	ReassignOrdersToCompany(ctx context.Context, customerID, bcGroupID int) error
	ListB2BOrderExtraFields(ctx context.Context, params string) ([]bigcommerce.B2BExtraFieldDef, error)
	// Invoices & receipts (read-only)
	ListB2BInvoices(ctx context.Context, params string) ([]map[string]any, error)
	GetB2BInvoice(ctx context.Context, invoiceID string) (map[string]any, error)
	DownloadB2BInvoicePDF(ctx context.Context, invoiceID string) (map[string]any, error)
	ListB2BInvoiceExtraFields(ctx context.Context, params string) ([]bigcommerce.B2BExtraFieldDef, error)
	ListB2BReceipts(ctx context.Context, params string) ([]map[string]any, error)
	GetB2BReceipt(ctx context.Context, receiptID string) (map[string]any, error)
	ListB2BReceiptLines(ctx context.Context, params string) ([]map[string]any, error)
	ListB2BLinesOfReceipt(ctx context.Context, receiptID, params string) ([]map[string]any, error)
	GetB2BReceiptLine(ctx context.Context, receiptID, lineID string) (map[string]any, error)
	// Quotes
	ListB2BQuotes(ctx context.Context, params string) ([]map[string]any, error)
	GetB2BQuote(ctx context.Context, quoteID int) (map[string]any, error)
	CreateB2BQuote(ctx context.Context, body map[string]any) (map[string]any, error)
	UpdateB2BQuote(ctx context.Context, quoteID int, body map[string]any) (map[string]any, error)
	DeleteB2BQuote(ctx context.Context, quoteID int) error
	GenerateB2BQuoteCheckout(ctx context.Context, quoteID int) (map[string]any, error)
	AssignB2BQuoteToOrder(ctx context.Context, quoteID, orderID int) error
	ExportB2BQuotePDF(ctx context.Context, quoteID int, currency map[string]any) (map[string]any, error)
	ListB2BQuoteShippingRates(ctx context.Context, quoteID int) ([]map[string]any, error)
	SelectB2BQuoteShippingRate(ctx context.Context, quoteID int, shippingMethodID, customName string, customCost float64, hasCustomCost bool) (map[string]any, error)
	RemoveB2BQuoteShippingRate(ctx context.Context, quoteID int) error
	ListB2BQuoteCustomShippingMethods(ctx context.Context) ([]map[string]any, error)
	ListB2BQuoteExtraFields(ctx context.Context, params string) ([]bigcommerce.B2BExtraFieldDef, error)
	// Payments, credit, and net terms (read-only)
	ListB2BPaymentMethods(ctx context.Context) ([]bigcommerce.B2BPaymentMethod, error)
	ListB2BCompanyPaymentMethods(ctx context.Context, companyID int) ([]bigcommerce.B2BCompanyPaymentMethod, error)
	ListB2BActivePaymentMethods(ctx context.Context, params string) ([]map[string]any, error)
	GetB2BCompanyCredit(ctx context.Context, companyID int) (*bigcommerce.B2BCompanyCredit, error)
	GetB2BCompanyPaymentTerms(ctx context.Context, companyID int) (*bigcommerce.B2BPaymentTerms, error)
	// Addresses
	ListB2BAddresses(ctx context.Context, params string) ([]bigcommerce.B2BAddress, error)
	CreateB2BAddress(ctx context.Context, payload bigcommerce.B2BAddressCreate) (*bigcommerce.B2BAddress, error)
	UpdateB2BAddress(ctx context.Context, addressID int, payload bigcommerce.B2BAddressCreate) (*bigcommerce.B2BAddress, error)
	DeleteB2BAddress(ctx context.Context, addressID int) error
}

// Compile-time check that *bigcommerce.Client satisfies BCCustomerManager.
var _ BCCustomerManager = (*bigcommerce.Client)(nil)

// BCCustomerManager resolves and deletes core BigCommerce customer accounts.
// The B2B company delete flow uses it to clean up the BC customer records
// linked to a company's buyer-portal users, since deleting a B2B company on
// its own leaves those customer accounts orphaned in the store. Resolution is
// by email because the B2B user record's bcCustomerId is frequently 0 (e.g.
// for admins created through company-create).
type BCCustomerManager interface {
	SearchCustomers(ctx context.Context, params map[string]string) ([]bigcommerce.Customer, error)
	DeleteCustomers(ctx context.Context, ids []int) error
}
