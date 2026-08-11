package bigcommerce_test

import (
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/stretchr/testify/suite"
)

type B2BCompanyBulkParseSuite struct {
	suite.Suite
}

func TestB2BCompanyBulkParseSuite(t *testing.T) {
	suite.Run(t, new(B2BCompanyBulkParseSuite))
}

func (s *B2BCompanyBulkParseSuite) TestParseLiveDataArrayShape() {
	body := []byte(`{"code":200,"data":[{"companyId":13927783},{"companyId":13927784}],"meta":{"message":"SUCCESS"}}`)
	ids, err := bigcommerce.ParseB2BCompanyBulkCreateResponse(body)
	s.NoError(err)
	s.Equal([]int{13927783, 13927784}, ids)
}

func (s *B2BCompanyBulkParseSuite) TestParseOpenAPIMetaArrayShape() {
	body := []byte(`{"code":200,"meta":[{"companyId":1},{"companyId":2}],"data":{"id":"string"}}`)
	ids, err := bigcommerce.ParseB2BCompanyBulkCreateResponse(body)
	s.NoError(err)
	s.Equal([]int{1, 2}, ids)
}

func (s *B2BCompanyBulkParseSuite) TestParseStringCompanyIDs() {
	body := []byte(`{"code":200,"data":[{"companyId":"42"},{"companyId":"43"}],"meta":{"message":"SUCCESS"}}`)
	ids, err := bigcommerce.ParseB2BCompanyBulkCreateResponse(body)
	s.NoError(err)
	s.Equal([]int{42, 43}, ids)
}

func (s *B2BCompanyBulkParseSuite) TestParseMissingIDsErrors() {
	_, err := bigcommerce.ParseB2BCompanyBulkCreateResponse([]byte(`{"code":200,"data":[],"meta":{"message":"SUCCESS"}}`))
	s.Error(err)
	s.Contains(err.Error(), "no company IDs")
}
