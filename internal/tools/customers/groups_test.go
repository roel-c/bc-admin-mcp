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

// ---------------------------------------------------------------------------
// Filter table sanity
// ---------------------------------------------------------------------------

type CustomerGroupFilterTableSuite struct {
	suite.Suite
}

func TestCustomerGroupFilterTableSuite(t *testing.T) {
	suite.Run(t, new(CustomerGroupFilterTableSuite))
}

func (s *CustomerGroupFilterTableSuite) TestAllEntriesHaveNonEmptyKeys() {
	for _, f := range customers.CustomerGroupSearchFilters {
		s.NotEmpty(f.ToolKey)
		s.NotEmpty(f.BCKey, "BCKey for %s", f.ToolKey)
	}
}

func (s *CustomerGroupFilterTableSuite) TestAllEntriesHaveValidKind() {
	validKinds := map[string]bool{"string": true, "number": true, "bool": true}
	for _, f := range customers.CustomerGroupSearchFilters {
		s.True(validKinds[f.Kind], "invalid Kind %q for %s", f.Kind, f.ToolKey)
	}
}

func (s *CustomerGroupFilterTableSuite) TestNoDuplicateToolKeys() {
	seen := map[string]bool{}
	for _, f := range customers.CustomerGroupSearchFilters {
		s.False(seen[f.ToolKey], "duplicate ToolKey: %s", f.ToolKey)
		seen[f.ToolKey] = true
	}
}

func (s *CustomerGroupFilterTableSuite) TestNoDuplicateBCKeys() {
	seen := map[string]bool{}
	for _, f := range customers.CustomerGroupSearchFilters {
		s.False(seen[f.BCKey], "duplicate BCKey: %s", f.BCKey)
		seen[f.BCKey] = true
	}
}

// ---------------------------------------------------------------------------
// Param parser tests (no mock BC needed)
// ---------------------------------------------------------------------------

type GroupCreateParamsSuite struct {
	suite.Suite
}

func TestGroupCreateParamsSuite(t *testing.T) {
	suite.Run(t, new(GroupCreateParamsSuite))
}

func (s *GroupCreateParamsSuite) TestMinimalValid() {
	p, err := customers.ParseGroupCreateParams(map[string]any{"name": "Wholesale"})
	s.NoError(err)
	s.Equal("Wholesale", p.Payload.Name)
	s.False(p.Confirmed)
	s.Empty(p.Warnings)
}

func (s *GroupCreateParamsSuite) TestMissingNameReturnsError() {
	_, err := customers.ParseGroupCreateParams(map[string]any{"is_default": true})
	s.Error(err)
	s.Contains(err.Error(), "name is required")
}

func (s *GroupCreateParamsSuite) TestEmptyNameReturnsError() {
	_, err := customers.ParseGroupCreateParams(map[string]any{"name": ""})
	s.Error(err)
}

func (s *GroupCreateParamsSuite) TestCategoryAccessSpecificRequiresCategories() {
	_, err := customers.ParseGroupCreateParams(map[string]any{
		"name":                 "VIP",
		"category_access_type": "specific",
	})
	s.Error(err)
	s.Contains(err.Error(), "non-empty category_access_categories")
}

func (s *GroupCreateParamsSuite) TestCategoriesWithoutSpecificRejected() {
	_, err := customers.ParseGroupCreateParams(map[string]any{
		"name":                       "VIP",
		"category_access_type":       "all",
		"category_access_categories": []any{float64(1), float64(2)},
	})
	s.Error(err)
	s.Contains(err.Error(), "only be set when category_access_type='specific'")
}

func (s *GroupCreateParamsSuite) TestCategoryAccessHappyPath() {
	p, err := customers.ParseGroupCreateParams(map[string]any{
		"name":                       "VIP",
		"category_access_type":       "specific",
		"category_access_categories": []any{float64(18), float64(19)},
	})
	s.NoError(err)
	s.Require().NotNil(p.Payload.CategoryAccess)
	s.Equal("specific", p.Payload.CategoryAccess.Type)
	s.Equal([]int{18, 19}, p.Payload.CategoryAccess.Categories)
}

func (s *GroupCreateParamsSuite) TestCategoryAccessRejectsDecimalCategoryID() {
	_, err := customers.ParseGroupCreateParams(map[string]any{
		"name":                       "VIP",
		"category_access_type":       "specific",
		"category_access_categories": []any{float64(18.5)},
	})
	s.Error(err)
	s.Contains(err.Error(), "must be an integer")
}

func (s *GroupCreateParamsSuite) TestInvalidCategoryAccessType() {
	_, err := customers.ParseGroupCreateParams(map[string]any{
		"name":                 "X",
		"category_access_type": "everything",
	})
	s.Error(err)
}

func (s *GroupCreateParamsSuite) TestDiscountRulesPriceListExclusivePruneAndWarn() {
	p, err := customers.ParseGroupCreateParams(map[string]any{
		"name": "VIP",
		"discount_rules": []any{
			map[string]any{"type": "price_list", "price_list_id": float64(3)},
			map[string]any{"type": "category", "method": "percent", "amount": "5.0000", "category_id": float64(30)},
		},
	})
	s.NoError(err)
	s.Require().Len(p.Payload.DiscountRules, 1, "non-price_list rules should be pruned")
	s.Equal("price_list", p.Payload.DiscountRules[0].Type)
	s.Equal(3, p.Payload.DiscountRules[0].PriceListID)
	s.Require().NotEmpty(p.Warnings, "agent should be warned that other rules were dropped")
	s.Contains(p.Warnings[0], "price_list")
}

func (s *GroupCreateParamsSuite) TestDiscountRuleRejectsDecimalIDs() {
	_, err := customers.ParseGroupCreateParams(map[string]any{
		"name": "VIP",
		"discount_rules": []any{
			map[string]any{
				"type":        "price_list",
				"price_list_id": float64(3.2),
			},
		},
	})
	s.Error(err)
	s.Contains(err.Error(), "price_list_id must be an integer")
}

func (s *GroupCreateParamsSuite) TestDiscountRulesMultipleAllRejected() {
	_, err := customers.ParseGroupCreateParams(map[string]any{
		"name": "VIP",
		"discount_rules": []any{
			map[string]any{"type": "all", "method": "percent", "amount": "2.0000"},
			map[string]any{"type": "all", "method": "fixed", "amount": "1.0000"},
		},
	})
	s.Error(err)
	s.Contains(err.Error(), "at most one rule with type='all'")
}

func (s *GroupCreateParamsSuite) TestDiscountRuleCategoryRequiresCategoryID() {
	_, err := customers.ParseGroupCreateParams(map[string]any{
		"name": "VIP",
		"discount_rules": []any{
			map[string]any{"type": "category", "method": "percent", "amount": "5.0000"},
		},
	})
	s.Error(err)
	s.Contains(err.Error(), "category_id is required")
}

func (s *GroupCreateParamsSuite) TestDiscountRuleAmountAcceptsNumber() {
	p, err := customers.ParseGroupCreateParams(map[string]any{
		"name": "VIP",
		"discount_rules": []any{
			map[string]any{"type": "category", "method": "percent", "amount": float64(5), "category_id": float64(30)},
		},
	})
	s.NoError(err)
	s.Require().Len(p.Payload.DiscountRules, 1)
	s.Equal("5.0000", p.Payload.DiscountRules[0].Amount)
}

func (s *GroupCreateParamsSuite) TestConfirmedFlag() {
	p, err := customers.ParseGroupCreateParams(map[string]any{"name": "X", "confirmed": true})
	s.NoError(err)
	s.True(p.Confirmed)
}

type GroupUpdateParamsSuite struct {
	suite.Suite
}

func TestGroupUpdateParamsSuite(t *testing.T) {
	suite.Run(t, new(GroupUpdateParamsSuite))
}

func (s *GroupUpdateParamsSuite) TestRequiresPositiveID() {
	_, err := customers.ParseGroupUpdateParams(map[string]any{
		"group_id": float64(0),
		"name":     "X",
	})
	s.Error(err)
}

func (s *GroupUpdateParamsSuite) TestNameAndFlagsAsPointers() {
	p, err := customers.ParseGroupUpdateParams(map[string]any{
		"group_id":   float64(5),
		"name":       "Renamed",
		"is_default": true,
	})
	s.NoError(err)
	s.Equal(5, p.GroupID)
	s.Require().NotNil(p.Update.Name)
	s.Equal("Renamed", *p.Update.Name)
	s.Require().NotNil(p.Update.IsDefault)
	s.True(*p.Update.IsDefault)
}

func (s *GroupUpdateParamsSuite) TestEmptyDiscountRulesIsExplicitClear() {
	p, err := customers.ParseGroupUpdateParams(map[string]any{
		"group_id":       float64(5),
		"discount_rules": []any{},
	})
	s.NoError(err)
	s.True(p.HasFields(), "explicit empty discount_rules counts as a change")
	s.NotNil(p.Update.DiscountRules)
	s.Len(p.Update.DiscountRules, 0)
}

func (s *GroupUpdateParamsSuite) TestNoFieldsErrorAtHandlerLevel() {
	p, err := customers.ParseGroupUpdateParams(map[string]any{"group_id": float64(5)})
	s.NoError(err)
	s.False(p.HasFields(), "no fields → handler should reject")
}

// ---------------------------------------------------------------------------
// Handler suite (mock BC client + discovery registry)
// ---------------------------------------------------------------------------

type GroupHandlerSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceCustomersAPI
	groups *customers.Groups
	reg    *discovery.Registry
}

func TestGroupHandlerSuite(t *testing.T) {
	suite.Run(t, new(GroupHandlerSuite))
}

func (s *GroupHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceCustomersAPI(s.ctrl)
	s.groups = customers.NewGroups(s.mockBC)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("customers", "Customers")
	s.reg.RegisterCategory("customers/groups", "Customer Groups")
	s.groups.RegisterTools(s.reg)
}

func (s *GroupHandlerSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *GroupHandlerSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found", toolPath)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolPath,
			Arguments: args,
		},
	}
	return def.Handler(context.Background(), req)
}

func (s *GroupHandlerSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *GroupHandlerSuite) TestListRequiresFilterOrListAll() {
	result, err := s.callTool("customers/groups/list", map[string]any{})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *GroupHandlerSuite) TestListAllReturnsSummaries() {
	s.mockBC.EXPECT().ListCustomerGroups(gomock.Any(), gomock.Any()).Return([]bigcommerce.CustomerGroup{
		{
			ID:               1,
			Name:             "Wholesale",
			CategoryAccess:   &bigcommerce.CategoryAccess{Type: "all"},
			DiscountRules:    []bigcommerce.CustomerGroupDiscountRule{{Type: "all", Method: "percent", Amount: "5.0000"}},
			IsDefault:        true,
			IsGroupForGuests: false,
		},
	}, nil)

	result, err := s.callTool("customers/groups/list", map[string]any{"list_all": true})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal(float64(1), data["total"])
	groups := data["groups"].([]any)
	s.Require().Len(groups, 1)
	first := groups[0].(map[string]any)
	s.Equal("Wholesale", first["name"])
	s.Equal("all", first["category_access"])
	s.Equal(float64(1), first["discount_rules"])
}

func (s *GroupHandlerSuite) TestListByNameLikeBuildsBCParams() {
	s.mockBC.EXPECT().ListCustomerGroups(gomock.Any(), map[string]string{"name:like": "Whole"}).
		Return([]bigcommerce.CustomerGroup{{ID: 1, Name: "Wholesale"}}, nil)

	result, err := s.callTool("customers/groups/list", map[string]any{"name_like": "Whole"})
	s.NoError(err)
	s.False(result.IsError)
}

func (s *GroupHandlerSuite) TestGetReturnsGroup() {
	s.mockBC.EXPECT().GetCustomerGroup(gomock.Any(), 7).Return(&bigcommerce.CustomerGroup{
		ID: 7, Name: "VIP",
	}, nil)
	result, err := s.callTool("customers/groups/get", map[string]any{"group_id": float64(7)})
	s.NoError(err)
	s.False(result.IsError)
}

func (s *GroupHandlerSuite) TestGetRejectsZero() {
	result, err := s.callTool("customers/groups/get", map[string]any{"group_id": float64(0)})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *GroupHandlerSuite) TestCount() {
	s.mockBC.EXPECT().CountCustomerGroups(gomock.Any()).Return(27, nil)
	result, err := s.callTool("customers/groups/count", map[string]any{})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal(float64(27), data["count"])
}

func (s *GroupHandlerSuite) TestCreatePreview() {
	result, err := s.callTool("customers/groups/create", map[string]any{"name": "NewGroup"})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal("preview", data["status"])
	s.Equal("create", data["action"])
}

func (s *GroupHandlerSuite) TestCreatePreviewWithMixedRulesIncludesWarning() {
	result, err := s.callTool("customers/groups/create", map[string]any{
		"name": "VIP",
		"discount_rules": []any{
			map[string]any{"type": "price_list", "price_list_id": float64(3)},
			map[string]any{"type": "category", "method": "percent", "amount": "5.0000", "category_id": float64(30)},
		},
	})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal("preview", data["status"])
	warns, ok := data["warnings"].([]any)
	s.Require().True(ok, "warnings array should be present")
	s.Require().NotEmpty(warns)
}

func (s *GroupHandlerSuite) TestCreateExecuteCallsClient() {
	s.mockBC.EXPECT().
		CreateCustomerGroup(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, payload bigcommerce.CustomerGroupCreate) (*bigcommerce.CustomerGroup, error) {
			s.Equal("NewGroup", payload.Name)
			return &bigcommerce.CustomerGroup{ID: 100, Name: "NewGroup"}, nil
		})

	result, err := s.callTool("customers/groups/create", map[string]any{
		"name":      "NewGroup",
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("created", data["status"])
}

func (s *GroupHandlerSuite) TestUpdateRequiresField() {
	result, err := s.callTool("customers/groups/update", map[string]any{
		"group_id": float64(1),
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *GroupHandlerSuite) TestUpdatePreviewWithDiscountRulesSurfacesNote() {
	result, err := s.callTool("customers/groups/update", map[string]any{
		"group_id": float64(3),
		"discount_rules": []any{
			map[string]any{"type": "all", "method": "percent", "amount": "10.0000"},
		},
	})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal("preview", data["status"])
	note, ok := data["note_discount_rules"].(string)
	s.Require().True(ok, "note_discount_rules note should be present")
	s.Contains(note, "overwrites")
}

func (s *GroupHandlerSuite) TestUpdateExecuteCallsClient() {
	s.mockBC.EXPECT().
		UpdateCustomerGroup(gomock.Any(), 3, gomock.Any()).
		Return(&bigcommerce.CustomerGroup{ID: 3, Name: "Renamed"}, nil)

	result, err := s.callTool("customers/groups/update", map[string]any{
		"group_id":  float64(3),
		"name":      "Renamed",
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("updated", data["status"])
}

func (s *GroupHandlerSuite) TestDeletePreviewIsR3AndShowsWarning() {
	s.mockBC.EXPECT().GetCustomerGroup(gomock.Any(), 4).Return(&bigcommerce.CustomerGroup{
		ID: 4, Name: "ToRemove",
	}, nil)

	result, err := s.callTool("customers/groups/delete", map[string]any{"group_id": float64(4)})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal("preview", data["status"])
	s.Equal("delete", data["action"])
	warning, ok := data["warning"].(string)
	s.Require().True(ok)
	s.Contains(warning, "irreversible")
	s.Contains(warning, "unassigned")
}

func (s *GroupHandlerSuite) TestDeletePreviewSurfacesGetErrorAsWarning() {
	s.mockBC.EXPECT().GetCustomerGroup(gomock.Any(), 4).Return(nil, contextError())

	result, err := s.callTool("customers/groups/delete", map[string]any{"group_id": float64(4)})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal("preview", data["status"])
	// Warning text should hint that pre-fetch failed.
	w, _ := data["warning"].(string)
	s.Contains(w, "pre-fetch")
}

func (s *GroupHandlerSuite) TestDeleteExecute() {
	s.mockBC.EXPECT().DeleteCustomerGroup(gomock.Any(), 4).Return(nil)
	result, err := s.callTool("customers/groups/delete", map[string]any{
		"group_id":  float64(4),
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("deleted", data["status"])
}

// contextError is a tiny helper that returns a non-nil error usable in mocks
// without pulling in a fmt dependency for one-line test setup.
func contextError() error {
	return &errString{s: "boom"}
}

type errString struct{ s string }

func (e *errString) Error() string { return e.s }
