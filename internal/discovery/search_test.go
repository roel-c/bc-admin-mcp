package discovery_test

import (
	"strings"
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/stretchr/testify/suite"
)

type SearchSuite struct {
	suite.Suite
	registry *discovery.Registry
}

func TestSearchSuite(t *testing.T) {
	suite.Run(t, new(SearchSuite))
}

func (s *SearchSuite) SetupTest() {
	s.registry = discovery.NewRegistry()
	s.registry.RegisterCategory("catalog", "Product catalog")
	s.registry.RegisterCategory("catalog/channels", "Sales channels and MSF")
	s.registry.RegisterCategory("catalog/products", "Product operations")
	s.registry.RegisterCategory("b2b", "B2B Edition")
	s.registry.RegisterCategory("b2b/channels", "B2B storefront channels")
	s.registry.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/channels/list",
		Tier:        middleware.TierR0,
		Summary:     "List sales channels for the connected store",
		Description: "List channels",
		Tool:        toolWithoutConfirmed("list", "List"),
	})
	s.registry.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/products/search",
		Tier:        middleware.TierR0,
		Summary:     "Search products",
		Description: "Search",
		Tool:        toolWithoutConfirmed("search", "Search"),
	})
	s.registry.RegisterTool(&discovery.ToolDef{
		Path:        "b2b/channels/list",
		Tier:        middleware.TierR0,
		Summary:     "List B2B Edition channels",
		Description: "B2B channels",
		Tool:        toolWithoutConfirmed("b2b_list", "List"),
	})
}

func (s *SearchSuite) TestSearchBySegmentFindsChannels() {
	entries, err := s.registry.Search("channels", "")
	s.NoError(err)
	s.NotEmpty(entries)

	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.Path
	}
	s.Contains(paths, "catalog/channels")
	s.Contains(paths, "catalog/channels/list")
	s.Contains(paths, "b2b/channels")
	s.Contains(paths, "b2b/channels/list")
}

func (s *SearchSuite) TestSearchScopedToCatalog() {
	entries, err := s.registry.Search("channels", "catalog")
	s.NoError(err)
	s.NotEmpty(entries)
	for _, e := range entries {
		s.True(e.Path == "catalog" || strings.HasPrefix(e.Path, "catalog/"),
			"unexpected path outside catalog scope: %s", e.Path)
		s.False(strings.HasPrefix(e.Path, "b2b/"), "b2b path leaked into catalog scope: %s", e.Path)
	}
}

func (s *SearchSuite) TestSearchInvalidScopeErrors() {
	_, err := s.registry.Search("channels", "nope")
	s.Error(err)
	s.Contains(err.Error(), "not a category")
}

func (s *SearchSuite) TestSearchEmptyQueryErrors() {
	_, err := s.registry.Search("  ", "")
	s.Error(err)
}

func (s *SearchSuite) TestDiscoverToolPathDeepLink() {
	entries, err := s.registry.Discover("catalog/channels/list")
	s.NoError(err)
	s.Len(entries, 1)
	s.Equal("catalog/channels/list", entries[0].Path)
	s.Equal("tool", entries[0].Type)
	s.Equal("R0", entries[0].Tier)
}

func (s *SearchSuite) TestDiscoverUnknownSuggestsChannels() {
	_, err := s.registry.Discover("channels")
	s.Error(err)
	s.Contains(err.Error(), "did you mean")
	s.Contains(err.Error(), "catalog/channels")
	s.Contains(err.Error(), `query="channels"`)
}
