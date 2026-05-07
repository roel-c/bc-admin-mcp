package discovery_test

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/stretchr/testify/suite"
)

type RegistrySuite struct {
	suite.Suite
	registry *discovery.Registry
}

func TestRegistrySuite(t *testing.T) {
	suite.Run(t, new(RegistrySuite))
}

func (s *RegistrySuite) SetupTest() {
	s.registry = discovery.NewRegistry()
}

func toolWithConfirmed(name, desc string) mcp.Tool {
	return mcp.NewTool(name,
		mcp.WithDescription(desc),
		mcp.WithBoolean("confirmed",
			mcp.Description("Set to true to execute after previewing."),
		),
	)
}

func toolWithoutConfirmed(name, desc string) mcp.Tool {
	return mcp.NewTool(name,
		mcp.WithDescription(desc),
		mcp.WithString("query",
			mcp.Description("Search query."),
		),
	)
}

func (s *RegistrySuite) TestRegisterR0ToolSucceeds() {
	s.registry.RegisterCategory("catalog", "Catalog tools")

	s.NotPanics(func() {
		s.registry.RegisterTool(&discovery.ToolDef{
			Path:        "catalog/search",
			Tier:        middleware.TierR0,
			Summary:     "Search products",
			Description: "Search products by query",
			Tool:        toolWithoutConfirmed("search", "Search products"),
		})
	})
}

func (s *RegistrySuite) TestRegisterR1WithConfirmedSucceeds() {
	s.registry.RegisterCategory("catalog", "Catalog tools")

	s.NotPanics(func() {
		s.registry.RegisterTool(&discovery.ToolDef{
			Path:        "catalog/bulk_update",
			Tier:        middleware.TierR1,
			Summary:     "Bulk price update",
			Description: "Update prices in bulk",
			Tool:        toolWithConfirmed("bulk_update", "Bulk price update"),
		})
	})
}

func (s *RegistrySuite) TestRegisterR1WithoutConfirmedPanics() {
	s.Panics(func() {
		s.registry.RegisterTool(&discovery.ToolDef{
			Path:        "catalog/write_op",
			Tier:        middleware.TierR1,
			Summary:     "Write",
			Description: "Write operation without confirmed",
			Tool:        toolWithoutConfirmed("write_op", "Write without confirmed"),
		})
	})
}

func (s *RegistrySuite) TestRegisterR2WithoutConfirmedPanics() {
	s.Panics(func() {
		s.registry.RegisterTool(&discovery.ToolDef{
			Path:        "catalog/dangerous",
			Tier:        middleware.TierR2,
			Summary:     "Dangerous",
			Description: "Dangerous operation",
			Tool:        toolWithoutConfirmed("dangerous", "Dangerous"),
		})
	})
}

func (s *RegistrySuite) TestRegisterR3WithoutConfirmedPanics() {
	s.Panics(func() {
		s.registry.RegisterTool(&discovery.ToolDef{
			Path:        "catalog/very_dangerous",
			Tier:        middleware.TierR3,
			Summary:     "Very dangerous",
			Description: "Very dangerous operation",
			Tool:        toolWithoutConfirmed("very_dangerous", "Very dangerous"),
		})
	})
}

func (s *RegistrySuite) TestRegisterR2WithConfirmedSucceeds() {
	s.NotPanics(func() {
		s.registry.RegisterTool(&discovery.ToolDef{
			Path:        "catalog/high_risk",
			Tier:        middleware.TierR2,
			Summary:     "High risk",
			Description: "High risk with confirmed",
			Tool:        toolWithConfirmed("high_risk", "High risk op"),
		})
	})
}

func (s *RegistrySuite) TestRegisterToolWithNoPropertiesPanicsForR1() {
	emptyTool := mcp.NewTool("empty_tool", mcp.WithDescription("no props"))

	s.Panics(func() {
		s.registry.RegisterTool(&discovery.ToolDef{
			Path:        "catalog/empty",
			Tier:        middleware.TierR1,
			Summary:     "Empty",
			Description: "No properties at all",
			Tool:        emptyTool,
		})
	})
}

func (s *RegistrySuite) TestGetToolFound() {
	s.registry.RegisterCategory("catalog", "Catalog")
	s.registry.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/search",
		Tier:        middleware.TierR0,
		Summary:     "Search",
		Description: "Search products",
		Tool:        toolWithoutConfirmed("search", "Search"),
	})

	def := s.registry.GetTool("catalog/search")
	s.NotNil(def)
	s.Equal(middleware.TierR0, def.Tier)
}

func (s *RegistrySuite) TestGetToolNotFound() {
	def := s.registry.GetTool("nonexistent/tool")
	s.Nil(def)
}

func (s *RegistrySuite) TestDiscoverRoot() {
	s.registry.RegisterCategory("catalog", "Catalog management")
	s.registry.RegisterCategory("orders", "Order management")

	entries, err := s.registry.Discover("")
	s.NoError(err)
	s.Len(entries, 2)
}

func (s *RegistrySuite) TestDiscoverCategoryChildren() {
	s.registry.RegisterCategory("catalog", "Catalog management")
	s.registry.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/search",
		Tier:        middleware.TierR0,
		Summary:     "Search products",
		Description: "Search products by query",
		Tool:        toolWithoutConfirmed("search", "Search products"),
	})

	entries, err := s.registry.Discover("catalog")
	s.NoError(err)
	s.Len(entries, 1)
	s.Equal("catalog/search", entries[0].Path)
	s.Equal("tool", entries[0].Type)
}

func (s *RegistrySuite) TestDiscoverNonexistentCategory() {
	_, err := s.registry.Discover("nonexistent")
	s.Error(err)
	s.Contains(err.Error(), "not found")
}
