package bigcommerce_test

import (
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/stretchr/testify/suite"
)

type GraphQLHelpersSuite struct {
	suite.Suite
}

func TestGraphQLHelpersSuite(t *testing.T) {
	suite.Run(t, new(GraphQLHelpersSuite))
}

func (s *GraphQLHelpersSuite) TestFormatGlobalIDs() {
	s.Equal("bc/store/channel/1", bigcommerce.FormatChannelGID(1))
	s.Equal("bc/store/product/130", bigcommerce.FormatProductGID(130))
	s.Equal("bc/store/category/9", bigcommerce.FormatCategoryGID(9))
	s.Equal("bc/store/brand/3", bigcommerce.FormatBrandGID(3))
	s.Equal("bc/store/locale/es", bigcommerce.FormatLocaleGID("es"))
	s.Equal("bc/store/locale/en-CA", bigcommerce.FormatLocaleGID(" en-CA "))
	s.Equal("bc/store/product-option/45", bigcommerce.FormatProductOptionGID(45))
	s.Equal("bc/store/productOption/45", bigcommerce.FormatCatalogProductOptionGID(45))
	s.Equal("bc/store/productOptionValue/68", bigcommerce.FormatCatalogProductOptionValueGID(68))
	s.Equal("bc/store/productCustomField/7", bigcommerce.FormatProductCustomFieldGID(7))
}

func (s *GraphQLHelpersSuite) TestFormatTranslationResourceID() {
	id, err := bigcommerce.FormatTranslationResourceID(bigcommerce.TranslationResourceProducts, 130)
	s.Require().NoError(err)
	s.Equal("bc/store/product/130", id)

	id, err = bigcommerce.FormatTranslationResourceID(bigcommerce.TranslationResourceProductOptions, 45)
	s.Require().NoError(err)
	s.Equal("bc/store/productOption/45", id)

	_, err = bigcommerce.FormatTranslationResourceID("SHIPPING_METHODS", 1)
	s.Error(err)
}

func (s *GraphQLHelpersSuite) TestGraphQLErrorSafeMessage() {
	e := &bigcommerce.GraphQLError{
		Messages: []string{"Locale 'fr' not found for channel '1'"},
		Path:     "graphql",
	}
	s.Contains(e.SafeError(), "Locale 'fr' not found")
}
