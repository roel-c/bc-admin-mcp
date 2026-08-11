package catalog_test

import (
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
)

type VariantOptionResolveSuite struct {
	suite.Suite
	options []bigcommerce.ProductOption
}

func TestVariantOptionResolveSuite(t *testing.T) {
	suite.Run(t, new(VariantOptionResolveSuite))
}

func (s *VariantOptionResolveSuite) SetupTest() {
	s.options = []bigcommerce.ProductOption{
		{
			ID: 10, DisplayName: "Size",
			OptionValues: []bigcommerce.ProductOptionValue{
				{ID: 100, Label: "Small"},
				{ID: 101, Label: "Large"},
			},
		},
		{
			ID: 20, DisplayName: "Color",
			OptionValues: []bigcommerce.ProductOptionValue{
				{ID: 200, Label: "Red"},
				{ID: 201, Label: "Blue"},
			},
		},
	}
}

func (s *VariantOptionResolveSuite) TestResolveByDisplayNameAndLabel() {
	in := []bigcommerce.VariantOptionVal{
		{OptionDisplayName: "size", Label: "large"},
		{OptionDisplayName: "Color", Label: "Blue"},
	}
	out, err := catalog.ResolveVariantOptionValues(s.options, in)
	s.NoError(err)
	s.Equal(10, out[0].OptionID)
	s.Equal(101, out[0].ID)
	s.Equal("Size", out[0].OptionDisplayName)
	s.Equal(20, out[1].OptionID)
	s.Equal(201, out[1].ID)
}

func (s *VariantOptionResolveSuite) TestResolveLeavesCompleteIDsUntouched() {
	in := []bigcommerce.VariantOptionVal{
		{OptionID: 99, ID: 999, Label: "X", OptionDisplayName: "Ignored"},
	}
	out, err := catalog.ResolveVariantOptionValues(s.options, in)
	s.NoError(err)
	s.Equal(99, out[0].OptionID)
	s.Equal(999, out[0].ID)
}

func (s *VariantOptionResolveSuite) TestResolveByOptionIDAndLabel() {
	in := []bigcommerce.VariantOptionVal{
		{OptionID: 20, Label: "red"},
	}
	out, err := catalog.ResolveVariantOptionValues(s.options, in)
	s.NoError(err)
	s.Equal(200, out[0].ID)
	s.Equal("Color", out[0].OptionDisplayName)
}

func (s *VariantOptionResolveSuite) TestResolveUnknownLabel() {
	in := []bigcommerce.VariantOptionVal{
		{OptionDisplayName: "Size", Label: "Medium"},
	}
	_, err := catalog.ResolveVariantOptionValues(s.options, in)
	s.Error(err)
}
