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
	// Users
	ListB2BUsers(ctx context.Context, params string) ([]bigcommerce.B2BUser, error)
	CreateB2BUser(ctx context.Context, payload bigcommerce.B2BUserCreate) (*bigcommerce.B2BUser, error)
	UpdateB2BUser(ctx context.Context, userID int, payload bigcommerce.B2BUserUpdate) (*bigcommerce.B2BUser, error)
	DeleteB2BUser(ctx context.Context, userID int) error
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
