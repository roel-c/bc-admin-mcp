package bigcommerce_test

import (
	"encoding/json"
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/stretchr/testify/suite"
)

type CustomerGroupTypesSuite struct {
	suite.Suite
}

func TestCustomerGroupTypesSuite(t *testing.T) {
	suite.Run(t, new(CustomerGroupTypesSuite))
}

func (s *CustomerGroupTypesSuite) TestUnmarshalFullGroup() {
	raw := []byte(`{
		"id": 7,
		"name": "Wholesale",
		"is_default": false,
		"is_group_for_guests": false,
		"category_access": {"type": "specific", "categories": [18, 19, 23]},
		"discount_rules": [
			{"type": "category", "method": "percent", "amount": "5.0000", "category_id": 30},
			{"type": "all", "method": "percent", "amount": "2.0000"}
		],
		"date_created": "2024-09-05 10:00:00",
		"date_modified": "2024-09-06 11:00:00"
	}`)

	var g bigcommerce.CustomerGroup
	s.Require().NoError(json.Unmarshal(raw, &g))
	s.Equal(7, g.ID)
	s.Equal("Wholesale", g.Name)
	s.False(g.IsDefault)
	s.Require().NotNil(g.CategoryAccess)
	s.Equal("specific", g.CategoryAccess.Type)
	s.Equal([]int{18, 19, 23}, g.CategoryAccess.Categories)
	s.Require().Len(g.DiscountRules, 2)
	s.Equal("category", g.DiscountRules[0].Type)
	s.Equal("5.0000", g.DiscountRules[0].Amount)
	s.Equal(30, g.DiscountRules[0].CategoryID)
	s.Equal("all", g.DiscountRules[1].Type)
}

func (s *CustomerGroupTypesSuite) TestUnmarshalPriceListGroup() {
	raw := []byte(`{
		"id": 11,
		"name": "VIP",
		"is_default": false,
		"is_group_for_guests": false,
		"category_access": {"type": "all"},
		"discount_rules": [{"type": "price_list", "price_list_id": 3}]
	}`)

	var g bigcommerce.CustomerGroup
	s.Require().NoError(json.Unmarshal(raw, &g))
	s.Require().Len(g.DiscountRules, 1)
	s.Equal("price_list", g.DiscountRules[0].Type)
	s.Equal(3, g.DiscountRules[0].PriceListID)
	s.Empty(g.DiscountRules[0].Method)
	s.Empty(g.DiscountRules[0].Amount)
}

func (s *CustomerGroupTypesSuite) TestMarshalCreateOmitsNilFields() {
	payload := bigcommerce.CustomerGroupCreate{Name: "Resellers"}
	raw, err := json.Marshal(payload)
	s.Require().NoError(err)
	// is_default / is_group_for_guests / category_access / discount_rules are
	// all omitempty when nil/empty so the wire body stays minimal.
	s.JSONEq(`{"name":"Resellers"}`, string(raw))
}

func (s *CustomerGroupTypesSuite) TestMarshalUpdatePointerFields() {
	t := true
	payload := bigcommerce.CustomerGroupUpdate{
		Name:      stringPtr("Premium"),
		IsDefault: &t,
	}
	raw, err := json.Marshal(payload)
	s.Require().NoError(err)
	s.JSONEq(`{"name":"Premium","is_default":true}`, string(raw))
}

type APIErrorCustomerGroupHintSuite struct {
	suite.Suite
}

func TestAPIErrorCustomerGroupHintSuite(t *testing.T) {
	suite.Run(t, new(APIErrorCustomerGroupHintSuite))
}

func (s *APIErrorCustomerGroupHintSuite) TestForbiddenReadHint() {
	e := &bigcommerce.APIError{
		StatusCode: 403,
		Method:     "GET",
		Path:       "customer_groups",
	}
	s.Contains(e.Error(), "store_v2_customers_read_only")
}

func (s *APIErrorCustomerGroupHintSuite) TestForbiddenWriteHint() {
	e := &bigcommerce.APIError{
		StatusCode: 403,
		Method:     "POST",
		Path:       "customer_groups",
	}
	s.Contains(e.Error(), "store_v2_customers")
	s.NotContains(e.Error(), "read_only")
}

func (s *APIErrorCustomerGroupHintSuite) TestNotFoundIDHint() {
	e := &bigcommerce.APIError{
		StatusCode: 404,
		Method:     "GET",
		Path:       "customer_groups/99",
	}
	s.Contains(e.Error(), "customer group not found")
}

func stringPtr(s string) *string { return &s }
