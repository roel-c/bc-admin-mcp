package catalog_test

import (
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
)

type BrandMetafieldSetParamsSuite struct {
	suite.Suite
}

func TestBrandMetafieldSetParamsSuite(t *testing.T) {
	suite.Run(t, new(BrandMetafieldSetParamsSuite))
}

func (s *BrandMetafieldSetParamsSuite) TestMinimalValidWithBrandID() {
	p, err := catalog.ParseBrandMetafieldSetParams(map[string]any{
		"brand_id":  float64(10),
		"namespace": "my_app",
		"key":       "k1",
		"value":     "v1",
	})
	s.NoError(err)
	s.Equal(10, p.BrandID)
	s.Equal("my_app", p.Namespace)
	s.Equal("k1", p.Key)
	s.Equal("v1", p.Value)
}

func (s *BrandMetafieldSetParamsSuite) TestWithBrandName() {
	p, err := catalog.ParseBrandMetafieldSetParams(map[string]any{
		"brand_name": "Acme",
		"namespace":  "ns",
		"key":        "k",
		"value":      "v",
	})
	s.NoError(err)
	s.Equal("Acme", p.BrandName)
}

func (s *BrandMetafieldSetParamsSuite) TestRejectBothIDAndName() {
	_, err := catalog.ParseBrandMetafieldSetParams(map[string]any{
		"brand_id":   float64(1),
		"brand_name": "X",
		"namespace":  "n",
		"key":        "k",
		"value":      "v",
	})
	s.Error(err)
	s.Contains(err.Error(), "mutually exclusive")
}

func (s *BrandMetafieldSetParamsSuite) TestRejectInvalidPermissionSet() {
	_, err := catalog.ParseBrandMetafieldSetParams(map[string]any{
		"brand_id":       float64(1),
		"namespace":      "n",
		"key":            "k",
		"value":          "v",
		"permission_set": "nope",
	})
	s.Error(err)
	s.Contains(err.Error(), "permission_set must be one of")
}

type BrandMetafieldDeleteParamsSuite struct {
	suite.Suite
}

func TestBrandMetafieldDeleteParamsSuite(t *testing.T) {
	suite.Run(t, new(BrandMetafieldDeleteParamsSuite))
}

func (s *BrandMetafieldDeleteParamsSuite) TestByMetafieldID() {
	p, err := catalog.ParseBrandMetafieldDeleteParams(map[string]any{
		"brand_id":     float64(5),
		"metafield_id": float64(99),
	})
	s.NoError(err)
	s.Equal(99, p.MetafieldID)
}

func (s *BrandMetafieldDeleteParamsSuite) TestByNamespaceKey() {
	p, err := catalog.ParseBrandMetafieldDeleteParams(map[string]any{
		"brand_id":  float64(5),
		"namespace": "my_app",
		"key":       "banner",
	})
	s.NoError(err)
	s.Equal("my_app", p.Namespace)
	s.Equal("banner", p.Key)
}

func (s *BrandMetafieldDeleteParamsSuite) TestRejectMixedModes() {
	_, err := catalog.ParseBrandMetafieldDeleteParams(map[string]any{
		"brand_id":     float64(1),
		"metafield_id": float64(1),
		"namespace":    "ns",
	})
	s.Error(err)
	s.Contains(err.Error(), "do not combine")
}
