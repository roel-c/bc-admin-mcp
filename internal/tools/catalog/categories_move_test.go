package catalog_test

import (
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
)

type MoveParamsSuite struct {
	suite.Suite
}

func TestMoveParamsSuite(t *testing.T) {
	suite.Run(t, new(MoveParamsSuite))
}

func (s *MoveParamsSuite) TestMinimalByID() {
	args := map[string]any{
		"category_id":   float64(425),
		"new_parent_id": float64(508),
	}
	p, err := catalog.ParseMoveParams(args)
	s.NoError(err)
	s.Equal(425, p.CategoryID)
	s.Equal(508, p.NewParentID)
	s.False(p.MoveToRoot)
}

func (s *MoveParamsSuite) TestMoveToRoot() {
	args := map[string]any{
		"category_id":   float64(425),
		"new_parent_id": float64(0),
	}
	p, err := catalog.ParseMoveParams(args)
	s.NoError(err)
	s.True(p.MoveToRoot)
	s.Equal(0, p.NewParentID)
}

func (s *MoveParamsSuite) TestByNames() {
	args := map[string]any{
		"category_name":   "Marimekko Fabric",
		"new_parent_name": "New Product Category",
	}
	p, err := catalog.ParseMoveParams(args)
	s.NoError(err)
	s.Equal("Marimekko Fabric", p.CategoryName)
	s.Equal("New Product Category", p.NewParentName)
}

func (s *MoveParamsSuite) TestRejectBothSourceIDAndName() {
	args := map[string]any{
		"category_id":   float64(1),
		"category_name": "X",
		"new_parent_id": float64(0),
	}
	_, err := catalog.ParseMoveParams(args)
	s.Error(err)
	s.Contains(err.Error(), "mutually exclusive")
}

func (s *MoveParamsSuite) TestRejectBothDestIDAndName() {
	args := map[string]any{
		"category_id":     float64(1),
		"new_parent_id":   float64(2),
		"new_parent_name": "X",
	}
	_, err := catalog.ParseMoveParams(args)
	s.Error(err)
	s.Contains(err.Error(), "mutually exclusive")
}

func (s *MoveParamsSuite) TestRejectNoSource() {
	args := map[string]any{
		"new_parent_id": float64(0),
	}
	_, err := catalog.ParseMoveParams(args)
	s.Error(err)
	s.Contains(err.Error(), "provide either category_id or category_name")
}

func (s *MoveParamsSuite) TestRejectNoDestination() {
	args := map[string]any{
		"category_id": float64(1),
	}
	_, err := catalog.ParseMoveParams(args)
	s.Error(err)
	s.Contains(err.Error(), "new_parent_id or new_parent_name")
}
