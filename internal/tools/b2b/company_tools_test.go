package b2b_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/b2b"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type B2BCompanyToolsSuite struct {
	suite.Suite
	ctrl        *gomock.Controller
	mockBC      *MockB2BCompanyAPI
	mockDeleter *MockBCCustomerManager
	ct          *b2b.CompanyTools
	reg         *discovery.Registry
}

func TestB2BCompanyToolsSuite(t *testing.T) {
	suite.Run(t, new(B2BCompanyToolsSuite))
}

func (s *B2BCompanyToolsSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockB2BCompanyAPI(s.ctrl)
	s.mockDeleter = NewMockBCCustomerManager(s.ctrl)
	s.ct = b2b.NewCompanyTools(s.mockBC, s.mockDeleter, session.NewStore(60*time.Second))
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("b2b", "B2B Edition")
	s.reg.RegisterCategory("b2b/companies", "Company management")
	s.reg.RegisterCategory("b2b/companies/users", "Company users")
	s.reg.RegisterCategory("b2b/companies/addresses", "Company addresses")
	s.reg.RegisterCategory("b2b/companies/attachments", "Company attachments")
	s.reg.RegisterCategory("b2b/companies/roles", "Company roles")
	s.reg.RegisterCategory("b2b/companies/permissions", "Company permissions")
	s.reg.RegisterCategory("b2b/companies/hierarchy", "Account hierarchy")
	s.reg.RegisterCategory("b2b/channels", "B2B channels")
	s.reg.RegisterCategory("b2b/orders", "B2B orders")
	s.ct.RegisterTools(s.reg)
}

func (s *B2BCompanyToolsSuite) TearDownTest() { s.ctrl.Finish() }

func (s *B2BCompanyToolsSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found", toolPath)
	return def.Handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: toolPath, Arguments: args},
	})
}

func (s *B2BCompanyToolsSuite) parseJSON(r *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(r)
	s.Require().NotEmpty(r.Content)
	var out map[string]any
	s.Require().NoError(json.Unmarshal([]byte(r.Content[0].(mcp.TextContent).Text), &out))
	return out
}

func sampleCompany() bigcommerce.B2BCompany {
	return bigcommerce.B2BCompany{
		CompanyID: 42, CompanyName: "Acme Corp", CompanyEmail: "acme@example.com",
		CompanyStatus: 1, City: "New York", Country: "US",
	}
}

// --- b2b/companies/list ---

func (s *B2BCompanyToolsSuite) TestCompanyListReturnsCompanies() {
	s.mockBC.EXPECT().ListB2BCompanies(gomock.Any(), "").Return([]bigcommerce.B2BCompany{sampleCompany()}, nil)

	res, err := s.callTool("b2b/companies/list", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestCompanyListWithStatusFilter() {
	s.mockBC.EXPECT().ListB2BCompanies(gomock.Any(), "companyStatus=1").Return([]bigcommerce.B2BCompany{sampleCompany()}, nil)

	res, err := s.callTool("b2b/companies/list", map[string]any{"status": float64(1)})
	s.NoError(err)
	s.False(res.IsError)
}

// --- b2b/companies/get ---

func (s *B2BCompanyToolsSuite) TestCompanyGetReturnsCompany() {
	co := sampleCompany()
	s.mockBC.EXPECT().GetB2BCompany(gomock.Any(), 42).Return(&co, nil)

	res, err := s.callTool("b2b/companies/get", map[string]any{"company_id": float64(42)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	company := data["company"].(map[string]any)
	s.Equal("Acme Corp", company["company_name"])
	s.Equal("approved", company["status_label"])
}

// --- b2b/companies/create ---

func (s *B2BCompanyToolsSuite) TestCompanyCreatePreview() {
	res, err := s.callTool("b2b/companies/create", map[string]any{
		"company_name":     "Acme Corp",
		"company_email":    "info@acme.com",
		"company_phone":    "5555550100",
		"company_country":  "US",
		"admin_email":      "admin@acme.com",
		"admin_first_name": "Admin",
		"admin_last_name":  "User",
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *B2BCompanyToolsSuite) TestCompanyCreateRejectsNoCompanyEmail() {
	res, err := s.callTool("b2b/companies/create", map[string]any{
		"company_name":     "Acme Corp",
		"company_phone":    "5555550100",
		"company_country":  "US",
		"admin_email":      "admin@acme.com",
		"admin_first_name": "Admin",
		"admin_last_name":  "User",
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestCompanyCreateRejectsNoCountry() {
	res, err := s.callTool("b2b/companies/create", map[string]any{
		"company_name":     "Acme Corp",
		"company_email":    "info@acme.com",
		"company_phone":    "5555550100",
		"admin_email":      "admin@acme.com",
		"admin_first_name": "Admin",
		"admin_last_name":  "User",
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestCompanyCreateConfirmed() {
	co := sampleCompany()
	s.mockBC.EXPECT().CreateB2BCompany(gomock.Any(), gomock.Any()).Return(&co, nil)
	// Create now re-fetches the full record (sparse create response).
	full := sampleCompany()
	s.mockBC.EXPECT().GetB2BCompany(gomock.Any(), 42).Return(&full, nil)

	res, err := s.callTool("b2b/companies/create", map[string]any{
		"company_name":     "Acme Corp",
		"company_email":    "info@acme.com",
		"company_phone":    "5555550100",
		"company_country":  "US",
		"admin_email":      "admin@acme.com",
		"admin_first_name": "Admin",
		"admin_last_name":  "User",
		"confirmed":        true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("created", data["status"])
}

func (s *B2BCompanyToolsSuite) TestCompanyCreateRejectsNoAdminEmail() {
	res, err := s.callTool("b2b/companies/create", map[string]any{
		"company_name":  "Acme Corp",
		"company_phone": "5555550100",
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestCompanyCreateRejectsNoPhone() {
	res, err := s.callTool("b2b/companies/create", map[string]any{
		"company_name": "Acme Corp",
		"admin_email":  "admin@acme.com",
	})
	s.NoError(err)
	s.True(res.IsError)
}

// --- b2b/companies/update ---

func (s *B2BCompanyToolsSuite) TestCompanyUpdatePreviewFetchesCompany() {
	co := sampleCompany()
	s.mockBC.EXPECT().GetB2BCompany(gomock.Any(), 42).Return(&co, nil)

	res, err := s.callTool("b2b/companies/update", map[string]any{
		"company_id":   float64(42),
		"company_name": "Acme Corp 2",
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *B2BCompanyToolsSuite) TestCompanyUpdateConfirmed() {
	co := sampleCompany()
	updated := sampleCompany()
	updated.CompanyName = "Acme Corp 2"
	s.mockBC.EXPECT().GetB2BCompany(gomock.Any(), 42).Return(&co, nil)
	s.mockBC.EXPECT().UpdateB2BCompany(gomock.Any(), 42, gomock.Any()).Return(&updated, nil)

	res, err := s.callTool("b2b/companies/update", map[string]any{
		"company_id":   float64(42),
		"company_name": "Acme Corp 2",
		"confirmed":    true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("updated", data["status"])
}

// --- b2b/companies/set_status ---

func (s *B2BCompanyToolsSuite) TestCompanySetStatusPreview() {
	res, err := s.callTool("b2b/companies/set_status", map[string]any{
		"company_id": float64(42),
		"action":     "approved",
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *B2BCompanyToolsSuite) TestCompanySetStatusRejectsInvalidAction() {
	res, err := s.callTool("b2b/companies/set_status", map[string]any{
		"company_id": float64(42),
		"action":     "banned",
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestCompanySetStatusConfirmed() {
	co := sampleCompany()
	s.mockBC.EXPECT().SetB2BCompanyStatus(gomock.Any(), 42, "approved").Return(&co, nil)

	res, err := s.callTool("b2b/companies/set_status", map[string]any{
		"company_id": float64(42),
		"action":     "approved",
		"confirmed":  true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("updated", data["status"])
}

// --- b2b/companies/delete ---

func (s *B2BCompanyToolsSuite) TestCompanyDeletePreviewListsLinkedCustomers() {
	co := sampleCompany()
	s.mockBC.EXPECT().GetB2BCompany(gomock.Any(), 42).Return(&co, nil)
	// Default delete_bc_customers=true, so the preview lists company users to
	// surface the linked BC customer accounts that will also be removed. The
	// admin's bcCustomerId is 0 (typical of company-create), so it is resolved
	// by email; the buyer carries an explicit bcCustomerId.
	s.mockBC.EXPECT().ListB2BUsers(gomock.Any(), "companyId=42").Return([]bigcommerce.B2BUser{
		{ID: 1, CompanyID: 42, Email: "admin@acme.com", FirstName: "Ada", LastName: "Admin", Role: 0, BCCustomerID: 0},
		{ID: 2, CompanyID: 42, Email: "buyer@acme.com", FirstName: "Bo", LastName: "Buyer", Role: 1, BCCustomerID: 52},
	}, nil)
	s.mockDeleter.EXPECT().SearchCustomers(gomock.Any(), gomock.Any()).Return([]bigcommerce.Customer{
		{ID: 51, Email: "admin@acme.com"},
	}, nil)

	res, err := s.callTool("b2b/companies/delete", map[string]any{"company_id": float64(42)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	s.Equal(true, data["delete_bc_customers"])
	linked := data["linked_bc_customers"].([]any)
	s.Len(linked, 2)
}

func (s *B2BCompanyToolsSuite) TestCompanyDeleteConfirmedCascadesToCustomers() {
	co := sampleCompany()
	s.mockBC.EXPECT().GetB2BCompany(gomock.Any(), 42).Return(&co, nil)
	s.mockBC.EXPECT().ListB2BUsers(gomock.Any(), "companyId=42").Return([]bigcommerce.B2BUser{
		{ID: 1, CompanyID: 42, Email: "admin@acme.com", Role: 0, BCCustomerID: 0},
		{ID: 2, CompanyID: 42, Email: "buyer@acme.com", Role: 1, BCCustomerID: 52},
		{ID: 3, CompanyID: 42, Email: "portal-only@acme.com", Role: 2, BCCustomerID: 0},
	}, nil)
	// Emails with bcCustomerId=0 are resolved against the core store. The admin
	// maps to customer 51; the portal-only user has no BC account.
	s.mockDeleter.EXPECT().SearchCustomers(gomock.Any(), gomock.Any()).Return([]bigcommerce.Customer{
		{ID: 51, Email: "admin@acme.com"},
	}, nil)
	s.mockBC.EXPECT().DeleteB2BCompany(gomock.Any(), 42).Return(nil)
	// Only users with a resolved BC customer (51 via email, 52 explicit) are
	// deleted; the portal-only user (no BC account) is skipped.
	s.mockDeleter.EXPECT().DeleteCustomers(gomock.Any(), []int{51, 52}).Return(nil)

	res, err := s.callTool("b2b/companies/delete", map[string]any{
		"company_id": float64(42),
		"confirmed":  true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("deleted", data["status"])
	s.Equal(true, data["bc_customers_deleted"])
}

func (s *B2BCompanyToolsSuite) TestCompanyDeleteKeepsCustomersWhenOptedOut() {
	co := sampleCompany()
	s.mockBC.EXPECT().GetB2BCompany(gomock.Any(), 42).Return(&co, nil)
	s.mockBC.EXPECT().DeleteB2BCompany(gomock.Any(), 42).Return(nil)
	// delete_bc_customers=false: no user lookup, no customer deletion.

	res, err := s.callTool("b2b/companies/delete", map[string]any{
		"company_id":          float64(42),
		"delete_bc_customers": false,
		"confirmed":           true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("deleted", data["status"])
	s.Nil(data["bc_customers_deleted"])
}

func (s *B2BCompanyToolsSuite) TestCompanyDeletePartialSuccessOnCustomerFailure() {
	co := sampleCompany()
	s.mockBC.EXPECT().GetB2BCompany(gomock.Any(), 42).Return(&co, nil)
	s.mockBC.EXPECT().ListB2BUsers(gomock.Any(), "companyId=42").Return([]bigcommerce.B2BUser{
		{ID: 1, CompanyID: 42, Email: "admin@acme.com", Role: 0, BCCustomerID: 51},
	}, nil)
	s.mockBC.EXPECT().DeleteB2BCompany(gomock.Any(), 42).Return(nil)
	s.mockDeleter.EXPECT().DeleteCustomers(gomock.Any(), []int{51}).Return(errors.New("boom"))

	res, err := s.callTool("b2b/companies/delete", map[string]any{
		"company_id": float64(42),
		"confirmed":  true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("partial_success", data["status"])
	s.Equal(false, data["bc_customers_deleted"])
}

// --- b2b/companies/users/list ---

func (s *B2BCompanyToolsSuite) TestUserListReturnsUsers() {
	s.mockBC.EXPECT().ListB2BUsers(gomock.Any(), "companyId=42").Return([]bigcommerce.B2BUser{
		{ID: 1, CompanyID: 42, Email: "buyer@acme.com", FirstName: "Jane", LastName: "Doe", Role: 1},
	}, nil)

	res, err := s.callTool("b2b/companies/users/list", map[string]any{"company_id": float64(42)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

// --- b2b/companies/users/create ---

func (s *B2BCompanyToolsSuite) TestUserCreatePreview() {
	res, err := s.callTool("b2b/companies/users/create", map[string]any{
		"company_id": float64(42),
		"email":      "buyer@acme.com",
		"first_name": "Jane",
		"last_name":  "Doe",
		"role":       float64(1),
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *B2BCompanyToolsSuite) TestUserCreateConfirmed() {
	u := bigcommerce.B2BUser{ID: 10, CompanyID: 42, Email: "buyer@acme.com", Role: 1}
	s.mockBC.EXPECT().CreateB2BUser(gomock.Any(), gomock.Any()).Return(&u, nil)

	res, err := s.callTool("b2b/companies/users/create", map[string]any{
		"company_id": float64(42),
		"email":      "buyer@acme.com",
		"first_name": "Jane",
		"last_name":  "Doe",
		"role":       float64(1),
		"confirmed":  true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("created", data["status"])
}

// --- b2b/companies/extra_fields + update_catalog + attachments ---

func (s *B2BCompanyToolsSuite) TestCompanyExtraFieldsList() {
	s.mockBC.EXPECT().ListB2BCompanyExtraFields(gomock.Any(), gomock.Any()).Return([]bigcommerce.B2BExtraFieldDef{
		{FieldName: "License No", FieldType: "2", IsRequired: true},
	}, nil)

	res, err := s.callTool("b2b/companies/extra_fields", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestCompanyCreateWithExtraFieldsPreview() {
	res, err := s.callTool("b2b/companies/create", map[string]any{
		"company_name":      "Acme Corp",
		"company_email":     "info@acme.com",
		"company_phone":     "5555550100",
		"company_country":   "US",
		"admin_email":       "admin@acme.com",
		"admin_first_name":  "Admin",
		"admin_last_name":   "User",
		"extra_fields_json": `[{"fieldName":"License No","fieldValue":"12345"}]`,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *B2BCompanyToolsSuite) TestCompanyUpdateCatalogConfirmed() {
	s.mockBC.EXPECT().UpdateB2BCompanyCatalog(gomock.Any(), 42, "7").Return(nil)

	res, err := s.callTool("b2b/companies/update_catalog", map[string]any{
		"company_id": float64(42),
		"catalog_id": "7",
		"confirmed":  true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("updated", data["status"])
}

func (s *B2BCompanyToolsSuite) TestAttachmentListReturnsAttachments() {
	s.mockBC.EXPECT().ListB2BCompanyAttachments(gomock.Any(), 42).Return([]bigcommerce.B2BAttachment{
		{ID: "abc-uuid", AttachmentFile: "https://cdn.example.com/file.pdf"},
	}, nil)

	res, err := s.callTool("b2b/companies/attachments/list", map[string]any{"company_id": float64(42)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestAttachmentAddPreviewThenUpload() {
	// Write a small temp file to upload.
	dir := s.T().TempDir()
	fp := dir + "/purchase_order.pdf"
	s.Require().NoError(os.WriteFile(fp, []byte("%PDF-1.7 test"), 0o600))

	// Preview (no confirm) must not call the API.
	prev, err := s.callTool("b2b/companies/attachments/add", map[string]any{
		"company_id": float64(42),
		"file_path":  fp,
	})
	s.NoError(err)
	pd := s.parseJSON(prev)
	s.Equal("preview", pd["status"])
	s.Equal("purchase_order.pdf", pd["file_name"])

	// Confirm → uploads.
	s.mockBC.EXPECT().AddB2BCompanyAttachment(gomock.Any(), 42, "purchase_order.pdf", gomock.Any()).
		Return(&bigcommerce.B2BAttachment{ID: "att-uuid", AttachmentFile: "https://cdn.example.com/po.pdf"}, nil)
	res, err := s.callTool("b2b/companies/attachments/add", map[string]any{
		"company_id": float64(42),
		"file_path":  fp,
		"confirmed":  true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("uploaded", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestAttachmentAddRejectsMissingFile() {
	res, err := s.callTool("b2b/companies/attachments/add", map[string]any{
		"company_id": float64(42),
		"file_path":  "/no/such/file-xyz.pdf",
		"confirmed":  true,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestAttachmentDeleteConfirmed() {
	s.mockBC.EXPECT().DeleteB2BCompanyAttachment(gomock.Any(), 42, "abc-uuid").Return(nil)

	res, err := s.callTool("b2b/companies/attachments/delete", map[string]any{
		"company_id":    float64(42),
		"attachment_id": "abc-uuid",
		"confirmed":     true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("deleted", data["status"])
}

// --- b2b/companies/users/get + get_by_customer ---

func (s *B2BCompanyToolsSuite) TestUserGetReturnsUser() {
	u := bigcommerce.B2BUser{ID: 7, CompanyID: 42, Email: "buyer@acme.com", Role: 1,
		ExtraFields: []bigcommerce.B2BExtraField{{FieldName: "PO", FieldValue: "123"}}}
	s.mockBC.EXPECT().GetB2BUser(gomock.Any(), 7).Return(&u, nil)

	res, err := s.callTool("b2b/companies/users/get", map[string]any{"user_id": float64(7)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	user := data["user"].(map[string]any)
	s.Equal("buyer@acme.com", user["email"])
	s.NotNil(user["extra_fields"])
}

func (s *B2BCompanyToolsSuite) TestUserGetByCustomerReturnsUser() {
	u := bigcommerce.B2BUser{ID: 7, CompanyID: 42, Email: "buyer@acme.com", Role: 1, BCCustomerID: 51}
	s.mockBC.EXPECT().GetB2BUserByCustomerID(gomock.Any(), 51).Return(&u, nil)

	res, err := s.callTool("b2b/companies/users/get_by_customer", map[string]any{"bc_customer_id": float64(51)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	user := data["user"].(map[string]any)
	s.Equal(float64(51), user["bc_customer_id"])
}

// --- b2b/companies/users/bulk_create ---

func (s *B2BCompanyToolsSuite) TestUserBulkCreatePreview() {
	res, err := s.callTool("b2b/companies/users/bulk_create", map[string]any{
		"users_json": `[{"company_id":42,"email":"a@acme.com","first_name":"A","last_name":"One","role":1},{"company_id":42,"email":"b@acme.com","first_name":"B","last_name":"Two","role":2}]`,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	s.Equal(float64(2), data["count"])
}

func (s *B2BCompanyToolsSuite) TestUserBulkCreateConfirmed() {
	s.mockBC.EXPECT().BulkCreateB2BUsers(gomock.Any(), gomock.Any()).Return([]bigcommerce.B2BNewUserID{
		{UserID: 1, BCID: 101},
		{UserID: 2, BCID: 102},
	}, nil)

	res, err := s.callTool("b2b/companies/users/bulk_create", map[string]any{
		"users_json": `[{"company_id":42,"email":"a@acme.com","first_name":"A","last_name":"One","role":1},{"company_id":42,"email":"b@acme.com","first_name":"B","last_name":"Two","role":2}]`,
		"confirmed":  true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("created", data["status"])
	s.Equal(float64(2), data["count"])
}

func (s *B2BCompanyToolsSuite) TestUserBulkCreateRejectsOverTen() {
	rows := make([]string, 11)
	for i := range rows {
		rows[i] = `{"company_id":42,"email":"x@acme.com","first_name":"X","last_name":"Y","role":2}`
	}
	res, err := s.callTool("b2b/companies/users/bulk_create", map[string]any{
		"users_json": "[" + strings.Join(rows, ",") + "]",
		"confirmed":  true,
	})
	s.NoError(err)
	s.True(res.IsError)
}

// --- b2b/companies/users/extra_fields ---

func (s *B2BCompanyToolsSuite) TestUserExtraFieldsList() {
	s.mockBC.EXPECT().ListB2BUserExtraFields(gomock.Any(), gomock.Any()).Return([]bigcommerce.B2BExtraFieldDef{
		{FieldName: "PO Number", FieldType: "0", IsRequired: true},
	}, nil)

	res, err := s.callTool("b2b/companies/users/extra_fields", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

// --- b2b/companies/addresses/list ---

func (s *B2BCompanyToolsSuite) TestAddressListReturnsAddresses() {
	s.mockBC.EXPECT().ListB2BAddresses(gomock.Any(), "companyId=42").Return([]bigcommerce.B2BAddress{
		{AddressID: 1, CompanyID: "42", AddressLine1: "123 Main St", City: "NYC", CountryName: "US", IsBilling: true},
	}, nil)

	res, err := s.callTool("b2b/companies/addresses/list", map[string]any{"company_id": float64(42)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

// --- b2b/companies/addresses/create ---

func (s *B2BCompanyToolsSuite) TestAddressCreatePreview() {
	res, err := s.callTool("b2b/companies/addresses/create", map[string]any{
		"company_id":   float64(42),
		"address_line1": "123 Main St",
		"city":         "New York",
		"country":      "US",
		"is_billing":   true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *B2BCompanyToolsSuite) TestAddressCreateConfirmed() {
	a := bigcommerce.B2BAddress{AddressID: 5, CompanyID: "42", AddressLine1: "123 Main St", City: "New York", CountryName: "US"}
	s.mockBC.EXPECT().CreateB2BAddress(gomock.Any(), gomock.Any()).Return(&a, nil)

	res, err := s.callTool("b2b/companies/addresses/create", map[string]any{
		"company_id":   float64(42),
		"address_line1": "123 Main St",
		"city":         "New York",
		"country":      "US",
		"confirmed":    true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("created", data["status"])
}
