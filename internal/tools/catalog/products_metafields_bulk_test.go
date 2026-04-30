package catalog_test

import (
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
)

type BulkProductMetafieldIDsSuite struct {
	suite.Suite
}

func TestBulkProductMetafieldIDsSuite(t *testing.T) {
	suite.Run(t, new(BulkProductMetafieldIDsSuite))
}

func (s *BulkProductMetafieldIDsSuite) TestValidDedupes() {
	args := map[string]any{
		"product_ids": []any{float64(1), float64(2), float64(1)},
	}
	ids, err := catalog.ParseBulkProductMetafieldProductIDs(args)
	s.NoError(err)
	s.Equal([]int{1, 2}, ids)
}

func (s *BulkProductMetafieldIDsSuite) TestRejectsEmpty() {
	_, err := catalog.ParseBulkProductMetafieldProductIDs(map[string]any{
		"product_ids": []any{},
	})
	s.Error(err)
	s.Contains(err.Error(), "non-empty")
}

func (s *BulkProductMetafieldIDsSuite) TestRejectsMissingKey() {
	_, err := catalog.ParseBulkProductMetafieldProductIDs(map[string]any{})
	s.Error(err)
	s.Contains(err.Error(), "product_ids is required")
}

func (s *BulkProductMetafieldIDsSuite) TestRejectsNonNumberElement() {
	_, err := catalog.ParseBulkProductMetafieldProductIDs(map[string]any{
		"product_ids": []any{"bad"},
	})
	s.Error(err)
	s.Contains(err.Error(), "must be a number")
}

func (s *BulkProductMetafieldIDsSuite) TestRejectsNonPositive() {
	_, err := catalog.ParseBulkProductMetafieldProductIDs(map[string]any{
		"product_ids": []any{float64(0)},
	})
	s.Error(err)
	s.Contains(err.Error(), "positive")
}

func (s *BulkProductMetafieldIDsSuite) TestRejectsOverMax() {
	arr := make([]any, 51)
	for i := range arr {
		arr[i] = float64(i + 1)
	}
	_, err := catalog.ParseBulkProductMetafieldProductIDs(map[string]any{
		"product_ids": arr,
	})
	s.Error(err)
	s.Contains(err.Error(), "exceeds maximum")
}
