package catalog_test

import (
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
)

type SEOAuditSuite struct {
	suite.Suite
}

func TestSEOAuditSuite(t *testing.T) {
	suite.Run(t, new(SEOAuditSuite))
}

func (s *SEOAuditSuite) TestAllFieldsPresent() {
	cats := []bigcommerce.Category{
		{ID: 1, Name: "Full SEO", PageTitle: "Title", MetaDescription: "Desc", SearchKeywords: "kw"},
	}
	issues := catalog.AuditSEOFields(cats)
	s.Empty(issues)
}

func (s *SEOAuditSuite) TestMissingPageTitle() {
	cats := []bigcommerce.Category{
		{ID: 2, Name: "No Title", MetaDescription: "Desc", SearchKeywords: "kw"},
	}
	issues := catalog.AuditSEOFields(cats)
	s.Len(issues, 1)
	s.Equal([]string{"page_title"}, issues[0].MissingFields)
}

func (s *SEOAuditSuite) TestMissingAllFields() {
	cats := []bigcommerce.Category{
		{ID: 3, Name: "Empty SEO"},
	}
	issues := catalog.AuditSEOFields(cats)
	s.Len(issues, 1)
	s.Equal([]string{"page_title", "meta_description", "search_keywords"}, issues[0].MissingFields)
}

func (s *SEOAuditSuite) TestMixedResults() {
	cats := []bigcommerce.Category{
		{ID: 1, Name: "OK", PageTitle: "T", MetaDescription: "D", SearchKeywords: "K"},
		{ID: 2, Name: "Partial", PageTitle: "T"},
		{ID: 3, Name: "None"},
	}
	issues := catalog.AuditSEOFields(cats)
	s.Len(issues, 2)
	s.Equal(2, issues[0].ID)
	s.Equal(3, issues[1].ID)
}

func (s *SEOAuditSuite) TestEmptyInput() {
	issues := catalog.AuditSEOFields(nil)
	s.Empty(issues)
}
