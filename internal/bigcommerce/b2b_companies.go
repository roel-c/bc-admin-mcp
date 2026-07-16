package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ---- Company types ----

// B2BCompanyStatus codes returned by the B2B Edition API.
const (
	B2BCompanyStatusPending  = 0 // awaiting admin approval
	B2BCompanyStatusApproved = 1 // active company account
	B2BCompanyStatusRejected = 2 // rejected by admin
	B2BCompanyStatusInactive = 3 // disabled
)

// B2BCompany represents a B2B Edition company account.
// Field names match the live API response (confirmed June 2026).
type B2BCompany struct {
	CompanyID    int    `json:"companyId"`
	CompanyName  string `json:"companyName"`
	CompanyEmail string `json:"companyEmail,omitempty"`
	CompanyPhone string `json:"companyPhone,omitempty"`
	// Address fields use the live API names (not companyAddress1/companyCity/etc.)
	AddressLine1 string `json:"addressLine1,omitempty"`
	AddressLine2 string `json:"addressLine2,omitempty"`
	City         string `json:"city,omitempty"`
	State        string `json:"state,omitempty"`
	Country      string `json:"country,omitempty"`
	ZipCode      string `json:"zipCode,omitempty"`
	// CompanyStatus: 0=pending, 1=approved, 2=rejected, 3=inactive
	CompanyStatus   int    `json:"companyStatus"`
	Description     string `json:"description,omitempty"`
	BCGroupID       int    `json:"bcGroupId,omitempty"`
	BCGroupName     string `json:"bcGroupName,omitempty"`
	CatalogID       *int   `json:"catalogId,omitempty"`
	CatalogName     string `json:"catalogName,omitempty"`
	UUID            string `json:"uuid,omitempty"`
	PriceListAssign []any  `json:"priceListAssign,omitempty"`
	ParentCompany   *B2BParentCompany `json:"parentCompany,omitempty"`
	CreatedAt       int64  `json:"createdAt,omitempty"`
	UpdatedAt       int64  `json:"updatedAt,omitempty"`
}

// B2BParentCompany holds optional parent company info for hierarchical accounts.
type B2BParentCompany struct {
	ID   *int   `json:"id"`
	Name string `json:"name"`
}

// B2BCompanyCreate is the request body for POST /companies.
// This also creates the initial admin user for the company.
type B2BCompanyCreate struct {
	CompanyName  string `json:"companyName"`
	CompanyEmail string `json:"companyEmail,omitempty"`
	CompanyPhone string `json:"companyPhone,omitempty"`
	AddressLine1 string `json:"addressLine1,omitempty"`
	AddressLine2 string `json:"addressLine2,omitempty"`
	City         string `json:"city,omitempty"`
	State        string `json:"state,omitempty"`
	Country      string `json:"country,omitempty"`
	ZipCode      string `json:"zipCode,omitempty"`
	// Admin user fields — required when creating a new company without
	// an existing BigCommerce customer to attach.
	AdminEmail     string `json:"adminEmail,omitempty"`
	AdminFirstName string `json:"adminFirstName,omitempty"`
	AdminLastName  string `json:"adminLastName,omitempty"`
	AdminPhone     string `json:"adminPhone,omitempty"`
	// BCCustomerID links an existing BC customer as the company admin instead
	// of creating a new user.
	BCCustomerID int             `json:"bcCustomerId,omitempty"`
	ExtraFields  []B2BExtraField `json:"extraFields,omitempty"`
}

// B2BCompanyUpdate is the request body for PUT /companies/{companyId}.
// All fields are optional — only provided fields are changed.
type B2BCompanyUpdate struct {
	CompanyName  string `json:"companyName,omitempty"`
	CompanyEmail string `json:"companyEmail,omitempty"`
	CompanyPhone string `json:"companyPhone,omitempty"`
	AddressLine1 string `json:"addressLine1,omitempty"`
	AddressLine2 string `json:"addressLine2,omitempty"`
	City         string `json:"city,omitempty"`
	State        string `json:"state,omitempty"`
	Country      string `json:"country,omitempty"`
	ZipCode      string `json:"zipCode,omitempty"`
	Description  string          `json:"description,omitempty"`
	ExtraFields  []B2BExtraField `json:"extraFields,omitempty"`
}

// B2BAttachment is a file attached to a company account. List responses use
// attachmentFile; the upload (POST) response uses attachmentUrl.
type B2BAttachment struct {
	ID             string `json:"id"`
	AttachmentFile string `json:"attachmentFile,omitempty"`
	AttachmentURL  string `json:"attachmentUrl,omitempty"`
}

// B2BCompanyStatusUpdate is the request body for PUT /companies/{companyId}/status.
// companyStatus: 0=pending, 1=approved, 2=rejected, 3=inactive.
// The B2B Edition API is camelCase throughout (matching the read model's
// `companyStatus` field), so the write payload must use camelCase too.
type B2BCompanyStatusUpdate struct {
	CompanyStatus int `json:"companyStatus"`
}

// B2BStatusFromAction maps human-readable action strings to status codes.
var B2BStatusFromAction = map[string]int{
	"approved": 1,
	"rejected": 2,
	"inactive": 3,
	"active":   1,
	"pending":  0,
}

// ---- Extra field types ----

// B2BExtraField is a name/value pair for a B2B Edition custom (extra) field,
// used on company, user, address, order, and invoice records.
type B2BExtraField struct {
	FieldName  string `json:"fieldName"`
	FieldValue string `json:"fieldValue"`
}

// B2BExtraFieldDef describes an extra-field configuration (definition) for a
// B2B resource. fieldType: 0=text, 1=multiline, 2=number, 3=dropdown.
// configType: 1=built-in, 2=user-defined.
type B2BExtraFieldDef struct {
	ID            json.Number `json:"id,omitempty"`
	UUID          string      `json:"uuid,omitempty"`
	FieldName     string      `json:"fieldName"`
	FieldType     string      `json:"fieldType,omitempty"`
	ConfigType    string      `json:"configType,omitempty"`
	IsRequired    bool        `json:"isRequired"`
	DefaultValue  string      `json:"defaultValue,omitempty"`
	Maximum       *float64    `json:"maximumValue,omitempty"`
	MaximumLength *int        `json:"maximumLength,omitempty"`
	ListOfValue   []string    `json:"listOfValue,omitempty"`
	VisibleToEnd  bool        `json:"visibleToEnandUser,omitempty"`
	IsBuiltIn     bool        `json:"isBuiltIn,omitempty"`
}

// ---- User types ----

// B2BUser represents a B2B Edition buyer portal user.
type B2BUser struct {
	ID           int    `json:"id"`
	CompanyID    int    `json:"companyId"`
	Email        string `json:"email"`
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	PhoneNumber  string `json:"phoneNumber,omitempty"`
	// Role: 0=company admin, 1=senior buyer, 2=junior buyer
	Role         int             `json:"role"`
	BCCustomerID int             `json:"bcCustomerId,omitempty"`
	ExtraFields  []B2BExtraField `json:"extraFields,omitempty"`
	CreatedAt    int64           `json:"createdAt,omitempty"`
	UpdatedAt    int64           `json:"updatedAt,omitempty"`
}

// B2BUserCreate is the request body for POST /users and POST /users/bulk.
type B2BUserCreate struct {
	CompanyID   int    `json:"companyId"`
	Email       string `json:"email"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	PhoneNumber string `json:"phoneNumber,omitempty"`
	// Role: 0=company admin, 1=senior buyer, 2=junior buyer
	Role        int `json:"role"`
	// BCCustomerID links an existing BC customer instead of creating a new one.
	BCCustomerID int             `json:"bcCustomerId,omitempty"`
	ExtraFields  []B2BExtraField `json:"extraFields,omitempty"`
}

// B2BUserUpdate is the request body for PUT /users/{userId}.
type B2BUserUpdate struct {
	FirstName   string `json:"firstName,omitempty"`
	LastName    string `json:"lastName,omitempty"`
	PhoneNumber string `json:"phoneNumber,omitempty"`
	Role        *int   `json:"role,omitempty"`
}

// ---- Address types ----

// B2BAddress represents a B2B Edition company address.
// Field names match the live API response (confirmed June 2026):
//   - companyId is returned as a string (json.Number handles both forms)
//   - state is stateName, country is countryName
//   - defaults are isDefaultBilling and isDefaultShipping (separate booleans)
type B2BAddress struct {
	AddressID         int         `json:"addressId"`
	CompanyID         json.Number `json:"companyId"`
	FirstName         string      `json:"firstName,omitempty"`
	LastName          string      `json:"lastName,omitempty"`
	AddressLine1      string      `json:"addressLine1"`
	AddressLine2      string      `json:"addressLine2,omitempty"`
	City              string      `json:"city"`
	StateName         string      `json:"stateName,omitempty"`
	StateCode         string      `json:"stateCode,omitempty"`
	CountryName       string      `json:"countryName,omitempty"`
	CountryCode       string      `json:"countryCode,omitempty"`
	ZipCode           string      `json:"zipCode,omitempty"`
	PhoneNumber       string      `json:"phoneNumber,omitempty"`
	Label             string      `json:"label,omitempty"`
	IsBilling         bool        `json:"isBilling"`
	IsShipping        bool        `json:"isShipping"`
	IsDefaultBilling  bool        `json:"isDefaultBilling"`
	IsDefaultShipping bool        `json:"isDefaultShipping"`
	CreatedAt         int64       `json:"createdAt,omitempty"`
	UpdatedAt         int64       `json:"updatedAt,omitempty"`
}

// B2BAddressCreate is the request body for POST/PUT /addresses.
// stateCode (2-letter) and countryCode (ISO 2-letter) are required by the
// B2B Edition API in addition to the full state and country names.
// Default designation uses isDefaultBilling and isDefaultShipping separately.
type B2BAddressCreate struct {
	CompanyID         int    `json:"companyId"`
	FirstName         string `json:"firstName,omitempty"`
	LastName          string `json:"lastName,omitempty"`
	AddressLine1      string `json:"addressLine1"`
	AddressLine2      string `json:"addressLine2,omitempty"`
	City              string `json:"city"`
	State             string `json:"stateName,omitempty"`
	StateCode         string `json:"stateCode,omitempty"`
	Country           string `json:"countryName"`
	CountryCode       string `json:"countryCode,omitempty"`
	ZipCode           string `json:"zipCode,omitempty"`
	PhoneNumber       string `json:"phoneNumber,omitempty"`
	Label             string `json:"label,omitempty"`
	IsBilling         bool   `json:"isBilling"`
	IsShipping        bool   `json:"isShipping"`
	IsDefaultBilling  bool   `json:"isDefaultBilling,omitempty"`
	IsDefaultShipping bool   `json:"isDefaultShipping,omitempty"`
}

// ---- Company client methods ----

// ListB2BCompanies returns all companies matching optional query params
// (e.g. "companyStatus=1&companyName=Acme"). Pass empty string for all.
func (c *B2BClient) ListB2BCompanies(ctx context.Context, params string) ([]B2BCompany, error) {
	path := "companies"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B companies: %w", err)
	}
	out := make([]B2BCompany, 0, len(raw))
	for _, r := range raw {
		var co B2BCompany
		if err := json.Unmarshal(r, &co); err != nil {
			return nil, fmt.Errorf("unmarshal B2B company: %w", err)
		}
		out = append(out, co)
	}
	return out, nil
}

// GetB2BCompany fetches a single company by ID.
func (c *B2BClient) GetB2BCompany(ctx context.Context, companyID int) (*B2BCompany, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("companies/%d", companyID))
	if err != nil {
		return nil, fmt.Errorf("get B2B company %d: %w", companyID, err)
	}
	var co B2BCompany
	if err := b2bUnmarshalSingle(body, &co, "get B2B company"); err != nil {
		return nil, err
	}
	return &co, nil
}

// CreateB2BCompany creates a new company account.
func (c *B2BClient) CreateB2BCompany(ctx context.Context, payload B2BCompanyCreate) (*B2BCompany, error) {
	body, err := c.B2BPost(ctx, "companies", payload)
	if err != nil {
		return nil, fmt.Errorf("create B2B company: %w", err)
	}
	var co B2BCompany
	if err := b2bUnmarshalSingle(body, &co, "create B2B company"); err != nil {
		return nil, err
	}
	return &co, nil
}

// UpdateB2BCompany updates a company's profile fields.
func (c *B2BClient) UpdateB2BCompany(ctx context.Context, companyID int, payload B2BCompanyUpdate) (*B2BCompany, error) {
	body, err := c.B2BPut(ctx, fmt.Sprintf("companies/%d", companyID), payload)
	if err != nil {
		return nil, fmt.Errorf("update B2B company %d: %w", companyID, err)
	}
	var co B2BCompany
	if err := b2bUnmarshalSingle(body, &co, "update B2B company"); err != nil {
		return nil, err
	}
	return &co, nil
}

// SetB2BCompanyStatus updates a company's lifecycle status.
// action is a human-readable label: "approved", "rejected", "inactive", "active".
func (c *B2BClient) SetB2BCompanyStatus(ctx context.Context, companyID int, action string) (*B2BCompany, error) {
	// Normalize the action so callers that pass e.g. "Approved" resolve the
	// same as "approved" (the map keys are lowercase).
	statusCode, ok := B2BStatusFromAction[strings.ToLower(strings.TrimSpace(action))]
	if !ok {
		return nil, fmt.Errorf("unknown B2B company status action %q", action)
	}
	body, err := c.B2BPut(ctx, fmt.Sprintf("companies/%d/status", companyID), B2BCompanyStatusUpdate{CompanyStatus: statusCode})
	if err != nil {
		return nil, fmt.Errorf("set B2B company %d status %q: %w", companyID, action, err)
	}
	var co B2BCompany
	if err := b2bUnmarshalSingle(body, &co, "set B2B company status"); err != nil {
		return nil, err
	}
	return &co, nil
}

// DeleteB2BCompany permanently deletes a company account.
func (c *B2BClient) DeleteB2BCompany(ctx context.Context, companyID int) error {
	_, err := c.B2BDelete(ctx, fmt.Sprintf("companies/%d", companyID))
	if err != nil {
		return fmt.Errorf("delete B2B company %d: %w", companyID, err)
	}
	return nil
}

// ListB2BCompanyExtraFields returns the extra-field definitions configured for
// companies. Optional params support offset/limit pagination.
func (c *B2BClient) ListB2BCompanyExtraFields(ctx context.Context, params string) ([]B2BExtraFieldDef, error) {
	path := "companies/extra-fields"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B company extra fields: %w", err)
	}
	out := make([]B2BExtraFieldDef, 0, len(raw))
	for _, r := range raw {
		var f B2BExtraFieldDef
		if err := json.Unmarshal(r, &f); err != nil {
			return nil, fmt.Errorf("unmarshal B2B company extra field: %w", err)
		}
		out = append(out, f)
	}
	return out, nil
}

// UpdateB2BCompanyCatalog assigns a price list / catalog to a company via
// PUT /companies/{companyId}/catalog. Note: this field is read-only for stores
// using Independent Companies behavior and the API will reject the change there.
func (c *B2BClient) UpdateB2BCompanyCatalog(ctx context.Context, companyID int, catalogID string) error {
	body := map[string]string{"catalogId": catalogID}
	_, err := c.B2BPut(ctx, fmt.Sprintf("companies/%d/catalog", companyID), body)
	if err != nil {
		return fmt.Errorf("update B2B company %d catalog: %w", companyID, err)
	}
	return nil
}

// ListB2BCompanyAttachments returns files attached to a company account. The
// endpoint's data field may be a single object or an array depending on the
// store; both forms are handled.
func (c *B2BClient) ListB2BCompanyAttachments(ctx context.Context, companyID int) ([]B2BAttachment, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("companies/%d/attachments", companyID))
	if err != nil {
		return nil, fmt.Errorf("list B2B company %d attachments: %w", companyID, err)
	}
	var resp B2BSingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse B2B attachments response: %w", err)
	}
	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		return nil, nil
	}
	trimmed := strings.TrimSpace(string(resp.Data))
	if strings.HasPrefix(trimmed, "[") {
		var arr []B2BAttachment
		if err := json.Unmarshal(resp.Data, &arr); err != nil {
			return nil, fmt.Errorf("unmarshal B2B attachments: %w", err)
		}
		return arr, nil
	}
	var one B2BAttachment
	if err := json.Unmarshal(resp.Data, &one); err != nil {
		return nil, fmt.Errorf("unmarshal B2B attachment: %w", err)
	}
	if one.ID == "" && one.AttachmentFile == "" {
		return nil, nil
	}
	return []B2BAttachment{one}, nil
}

// AddB2BCompanyAttachment uploads a file (multipart) to a company account,
// making it visible in the Attachments tab of the company's backend record.
// The B2B API rejects files larger than 10MB.
func (c *B2BClient) AddB2BCompanyAttachment(ctx context.Context, companyID int, fileName string, data []byte) (*B2BAttachment, error) {
	body, err := c.B2BPostMultipart(ctx, fmt.Sprintf("companies/%d/attachments", companyID), "attachmentFile", fileName, data)
	if err != nil {
		return nil, fmt.Errorf("add B2B company %d attachment: %w", companyID, err)
	}
	var a B2BAttachment
	if err := b2bUnmarshalSingle(body, &a, "add B2B company attachment"); err != nil {
		// The upload succeeded (2xx); some stores return a minimal body whose
		// shape does not map cleanly. Treat that as success with no detail.
		return &B2BAttachment{}, nil //nolint:nilerr // response body shape varies
	}
	return &a, nil
}

// DeleteB2BCompanyAttachment removes an attachment from a company account.
func (c *B2BClient) DeleteB2BCompanyAttachment(ctx context.Context, companyID int, attachmentID string) error {
	_, err := c.B2BDelete(ctx, fmt.Sprintf("companies/%d/attachments/%s", companyID, attachmentID))
	if err != nil {
		return fmt.Errorf("delete B2B company %d attachment %s: %w", companyID, attachmentID, err)
	}
	return nil
}

// ---- User client methods ----

// ListB2BUsers returns users matching optional params (e.g. "companyId=42&role=2").
func (c *B2BClient) ListB2BUsers(ctx context.Context, params string) ([]B2BUser, error) {
	path := "users"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B users: %w", err)
	}
	out := make([]B2BUser, 0, len(raw))
	for _, r := range raw {
		var u B2BUser
		if err := json.Unmarshal(r, &u); err != nil {
			return nil, fmt.Errorf("unmarshal B2B user: %w", err)
		}
		out = append(out, u)
	}
	return out, nil
}

// CreateB2BUser creates a new buyer portal user and assigns them to a company.
func (c *B2BClient) CreateB2BUser(ctx context.Context, payload B2BUserCreate) (*B2BUser, error) {
	body, err := c.B2BPost(ctx, "users", payload)
	if err != nil {
		return nil, fmt.Errorf("create B2B user: %w", err)
	}
	var u B2BUser
	if err := b2bUnmarshalSingle(body, &u, "create B2B user"); err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdateB2BUser updates a user's profile or role.
func (c *B2BClient) UpdateB2BUser(ctx context.Context, userID int, payload B2BUserUpdate) (*B2BUser, error) {
	body, err := c.B2BPut(ctx, fmt.Sprintf("users/%d", userID), payload)
	if err != nil {
		return nil, fmt.Errorf("update B2B user %d: %w", userID, err)
	}
	var u B2BUser
	if err := b2bUnmarshalSingle(body, &u, "update B2B user"); err != nil {
		return nil, err
	}
	return &u, nil
}

// DeleteB2BUser removes a user from the B2B Edition buyer portal.
func (c *B2BClient) DeleteB2BUser(ctx context.Context, userID int) error {
	_, err := c.B2BDelete(ctx, fmt.Sprintf("users/%d", userID))
	if err != nil {
		return fmt.Errorf("delete B2B user %d: %w", userID, err)
	}
	return nil
}

// GetB2BUser fetches a single user by B2B Edition user ID. This endpoint
// includes the user's extra fields by default.
func (c *B2BClient) GetB2BUser(ctx context.Context, userID int) (*B2BUser, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("users/%d", userID))
	if err != nil {
		return nil, fmt.Errorf("get B2B user %d: %w", userID, err)
	}
	var u B2BUser
	if err := b2bUnmarshalSingle(body, &u, "get B2B user"); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetB2BUserByCustomerID fetches the B2B user linked to a BigCommerce customer
// ID. Returns a 404 (surfaced as an error) if no B2B user is linked.
func (c *B2BClient) GetB2BUserByCustomerID(ctx context.Context, customerID int) (*B2BUser, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("users/customer/%d", customerID))
	if err != nil {
		return nil, fmt.Errorf("get B2B user by customer %d: %w", customerID, err)
	}
	var u B2BUser
	if err := b2bUnmarshalSingle(body, &u, "get B2B user by customer"); err != nil {
		return nil, err
	}
	return &u, nil
}

// B2BNewUserID is a single entry in the Bulk Create Users response: the new
// B2B user ID plus the corresponding BigCommerce customer ID.
type B2BNewUserID struct {
	UserID int `json:"userId"`
	BCID   int `json:"bcId"`
}

// BulkCreateB2BUsers creates up to 10 users in one call via POST /users/bulk.
// The response returns only the new {userId, bcId} pairs, not full records.
func (c *B2BClient) BulkCreateB2BUsers(ctx context.Context, payloads []B2BUserCreate) ([]B2BNewUserID, error) {
	body, err := c.B2BPost(ctx, "users/bulk", payloads)
	if err != nil {
		return nil, fmt.Errorf("bulk create B2B users: %w", err)
	}
	var out []B2BNewUserID
	if err := b2bUnmarshalList(body, &out, "bulk create B2B users"); err != nil {
		return nil, err
	}
	return out, nil
}

// ListB2BUserExtraFields returns the extra-field definitions configured for
// users. Optional params support offset/limit pagination.
func (c *B2BClient) ListB2BUserExtraFields(ctx context.Context, params string) ([]B2BExtraFieldDef, error) {
	path := "users/extra-fields"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B user extra fields: %w", err)
	}
	out := make([]B2BExtraFieldDef, 0, len(raw))
	for _, r := range raw {
		var f B2BExtraFieldDef
		if err := json.Unmarshal(r, &f); err != nil {
			return nil, fmt.Errorf("unmarshal B2B user extra field: %w", err)
		}
		out = append(out, f)
	}
	return out, nil
}

// ---- Address client methods ----

// ListB2BAddresses returns addresses matching optional params (e.g. "companyId=42&isBilling=true").
func (c *B2BClient) ListB2BAddresses(ctx context.Context, params string) ([]B2BAddress, error) {
	path := "addresses"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B addresses: %w", err)
	}
	out := make([]B2BAddress, 0, len(raw))
	for _, r := range raw {
		var a B2BAddress
		if err := json.Unmarshal(r, &a); err != nil {
			return nil, fmt.Errorf("unmarshal B2B address: %w", err)
		}
		out = append(out, a)
	}
	return out, nil
}

// CreateB2BAddress adds an address to a company account.
func (c *B2BClient) CreateB2BAddress(ctx context.Context, payload B2BAddressCreate) (*B2BAddress, error) {
	body, err := c.B2BPost(ctx, "addresses", payload)
	if err != nil {
		return nil, fmt.Errorf("create B2B address: %w", err)
	}
	var a B2BAddress
	if err := b2bUnmarshalSingle(body, &a, "create B2B address"); err != nil {
		return nil, err
	}
	return &a, nil
}

// UpdateB2BAddress updates a company address by ID.
func (c *B2BClient) UpdateB2BAddress(ctx context.Context, addressID int, payload B2BAddressCreate) (*B2BAddress, error) {
	body, err := c.B2BPut(ctx, fmt.Sprintf("addresses/%d", addressID), payload)
	if err != nil {
		return nil, fmt.Errorf("update B2B address %d: %w", addressID, err)
	}
	var a B2BAddress
	if err := b2bUnmarshalSingle(body, &a, "update B2B address"); err != nil {
		return nil, err
	}
	return &a, nil
}

// DeleteB2BAddress removes an address from a company account.
func (c *B2BClient) DeleteB2BAddress(ctx context.Context, addressID int) error {
	_, err := c.B2BDelete(ctx, fmt.Sprintf("addresses/%d", addressID))
	if err != nil {
		return fmt.Errorf("delete B2B address %d: %w", addressID, err)
	}
	return nil
}
