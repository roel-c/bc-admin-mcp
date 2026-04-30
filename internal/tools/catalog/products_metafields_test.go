package catalog_test

import (
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
)

type ProductMetafieldSetParamsSuite struct {
	suite.Suite
}

func TestProductMetafieldSetParamsSuite(t *testing.T) {
	suite.Run(t, new(ProductMetafieldSetParamsSuite))
}

func (s *ProductMetafieldSetParamsSuite) TestMinimalByProductID() {
	args := map[string]any{
		"product_id": float64(101),
		"namespace":  "erp",
		"key":        "external_ref",
		"value":      "SKU-999",
	}
	p, err := catalog.ParseProductMetafieldSetParams(args)
	s.NoError(err)
	s.Equal(101, p.ProductID)
	s.Equal("erp", p.Namespace)
	s.Equal("external_ref", p.Key)
	s.Equal("SKU-999", p.Value)
	s.Equal("", p.PermissionSet)
}

func (s *ProductMetafieldSetParamsSuite) TestBySKU() {
	args := map[string]any{
		"sku":       "ABC-1",
		"namespace": "ns",
		"key":       "k",
		"value":     "v",
	}
	p, err := catalog.ParseProductMetafieldSetParams(args)
	s.NoError(err)
	s.Equal("ABC-1", p.SKU)
}

func (s *ProductMetafieldSetParamsSuite) TestRejectProductIDAndSKUTogether() {
	args := map[string]any{
		"product_id": float64(1),
		"sku":        "X",
		"namespace":  "n",
		"key":        "k",
		"value":      "v",
	}
	_, err := catalog.ParseProductMetafieldSetParams(args)
	s.Error(err)
	s.Contains(err.Error(), "only one of")
}

func (s *ProductMetafieldSetParamsSuite) TestRejectMissingTarget() {
	args := map[string]any{
		"namespace": "n",
		"key":       "k",
		"value":     "v",
	}
	_, err := catalog.ParseProductMetafieldSetParams(args)
	s.Error(err)
	s.Contains(err.Error(), "exactly one of")
}

func (s *ProductMetafieldSetParamsSuite) TestRejectNonPositiveProductID() {
	args := map[string]any{
		"product_id": float64(0),
		"namespace":  "n",
		"key":        "k",
		"value":      "v",
	}
	_, err := catalog.ParseProductMetafieldSetParams(args)
	s.Error(err)
	s.Contains(err.Error(), "positive")
}

func (s *ProductMetafieldSetParamsSuite) TestValidPermissionSet() {
	args := map[string]any{
		"product_id":     float64(1),
		"namespace":      "n",
		"key":            "k",
		"value":          "v",
		"permission_set": "read_and_sf_access",
	}
	p, err := catalog.ParseProductMetafieldSetParams(args)
	s.NoError(err)
	s.Equal("read_and_sf_access", p.PermissionSet)
}

func (s *ProductMetafieldSetParamsSuite) TestInvalidPermissionSet() {
	args := map[string]any{
		"product_id":     float64(1),
		"namespace":      "n",
		"key":            "k",
		"value":          "v",
		"permission_set": "nope",
	}
	_, err := catalog.ParseProductMetafieldSetParams(args)
	s.Error(err)
	s.Contains(err.Error(), "permission_set must be one of")
}

type ProductMetafieldDeleteParamsSuite struct {
	suite.Suite
}

func TestProductMetafieldDeleteParamsSuite(t *testing.T) {
	suite.Run(t, new(ProductMetafieldDeleteParamsSuite))
}

func (s *ProductMetafieldDeleteParamsSuite) TestByMetafieldID() {
	args := map[string]any{
		"product_id":   float64(10),
		"metafield_id": float64(55),
	}
	p, err := catalog.ParseProductMetafieldDeleteParams(args)
	s.NoError(err)
	s.Equal(10, p.ProductID)
	s.Equal(55, p.MetafieldID)
}

func (s *ProductMetafieldDeleteParamsSuite) TestByNamespaceKey() {
	args := map[string]any{
		"product_name": "Exact Name",
		"namespace":    "ns",
		"key":          "k",
	}
	p, err := catalog.ParseProductMetafieldDeleteParams(args)
	s.NoError(err)
	s.Equal("Exact Name", p.ProductName)
	s.Equal("ns", p.Namespace)
	s.Equal("k", p.Key)
}

func (s *ProductMetafieldDeleteParamsSuite) TestRejectMetafieldIDWithNamespace() {
	args := map[string]any{
		"product_id":   float64(1),
		"metafield_id": float64(5),
		"namespace":    "n",
		"key":          "k",
	}
	_, err := catalog.ParseProductMetafieldDeleteParams(args)
	s.Error(err)
	s.Contains(err.Error(), "metafield_id alone")
}

func (s *ProductMetafieldDeleteParamsSuite) TestRejectZeroMetafieldID() {
	args := map[string]any{
		"product_id":   float64(1),
		"metafield_id": float64(0),
	}
	_, err := catalog.ParseProductMetafieldDeleteParams(args)
	s.Error(err)
	s.Contains(err.Error(), "positive")
}
