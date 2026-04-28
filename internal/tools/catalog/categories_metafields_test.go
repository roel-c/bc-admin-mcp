package catalog_test

import (
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
)

type MetafieldSetParamsSuite struct {
	suite.Suite
}

func TestMetafieldSetParamsSuite(t *testing.T) {
	suite.Run(t, new(MetafieldSetParamsSuite))
}

func (s *MetafieldSetParamsSuite) TestMinimalValid() {
	args := map[string]any{
		"category_id": float64(408),
		"namespace":   "my_app",
		"key":         "banner_text",
		"value":       "Hello World",
	}
	p, err := catalog.ParseMetafieldSetParams(args)
	s.NoError(err)
	s.Equal(408, p.CategoryID)
	s.Equal("my_app", p.Namespace)
	s.Equal("banner_text", p.Key)
	s.Equal("Hello World", p.Value)
}

func (s *MetafieldSetParamsSuite) TestWithCategoryName() {
	args := map[string]any{
		"category_name": "Shop All",
		"namespace":     "seo",
		"key":           "custom",
		"value":         "val",
	}
	p, err := catalog.ParseMetafieldSetParams(args)
	s.NoError(err)
	s.Equal("Shop All", p.CategoryName)
}

func (s *MetafieldSetParamsSuite) TestRejectBothIDAndName() {
	args := map[string]any{
		"category_id":   float64(1),
		"category_name": "X",
		"namespace":     "ns",
		"key":           "k",
		"value":         "v",
	}
	_, err := catalog.ParseMetafieldSetParams(args)
	s.Error(err)
	s.Contains(err.Error(), "mutually exclusive")
}

func (s *MetafieldSetParamsSuite) TestRejectMissingNamespace() {
	args := map[string]any{
		"category_id": float64(1),
		"key":         "k",
		"value":       "v",
	}
	_, err := catalog.ParseMetafieldSetParams(args)
	s.Error(err)
	s.Contains(err.Error(), "namespace is required")
}

func (s *MetafieldSetParamsSuite) TestRejectMissingKey() {
	args := map[string]any{
		"category_id": float64(1),
		"namespace":   "ns",
		"value":       "v",
	}
	_, err := catalog.ParseMetafieldSetParams(args)
	s.Error(err)
	s.Contains(err.Error(), "key is required")
}

func (s *MetafieldSetParamsSuite) TestRejectInvalidPermissionSet() {
	args := map[string]any{
		"category_id":    float64(1),
		"namespace":      "ns",
		"key":            "k",
		"value":          "v",
		"permission_set": "invalid",
	}
	_, err := catalog.ParseMetafieldSetParams(args)
	s.Error(err)
	s.Contains(err.Error(), "permission_set must be one of")
}

func (s *MetafieldSetParamsSuite) TestAcceptsValidPermissionSet() {
	args := map[string]any{
		"category_id":    float64(1),
		"namespace":      "ns",
		"key":            "k",
		"value":          "v",
		"permission_set": "read_and_sf_access",
	}
	p, err := catalog.ParseMetafieldSetParams(args)
	s.NoError(err)
	s.Equal("read_and_sf_access", p.PermissionSet)
}

type MetafieldDeleteParamsSuite struct {
	suite.Suite
}

func TestMetafieldDeleteParamsSuite(t *testing.T) {
	suite.Run(t, new(MetafieldDeleteParamsSuite))
}

func (s *MetafieldDeleteParamsSuite) TestByMetafieldID() {
	args := map[string]any{
		"category_id":  float64(408),
		"metafield_id": float64(99),
	}
	p, err := catalog.ParseMetafieldDeleteParams(args)
	s.NoError(err)
	s.Equal(99, p.MetafieldID)
}

func (s *MetafieldDeleteParamsSuite) TestByNamespaceKey() {
	args := map[string]any{
		"category_id": float64(408),
		"namespace":   "my_app",
		"key":         "banner",
	}
	p, err := catalog.ParseMetafieldDeleteParams(args)
	s.NoError(err)
	s.Equal("my_app", p.Namespace)
	s.Equal("banner", p.Key)
}

func (s *MetafieldDeleteParamsSuite) TestRejectMixedModes() {
	args := map[string]any{
		"category_id":  float64(1),
		"metafield_id": float64(1),
		"namespace":    "ns",
	}
	_, err := catalog.ParseMetafieldDeleteParams(args)
	s.Error(err)
	s.Contains(err.Error(), "do not combine")
}

func (s *MetafieldDeleteParamsSuite) TestRejectNoTarget() {
	args := map[string]any{
		"category_id": float64(1),
	}
	_, err := catalog.ParseMetafieldDeleteParams(args)
	s.Error(err)
}
