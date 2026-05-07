package middleware_test

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/stretchr/testify/suite"
)

type TiersSuite struct {
	suite.Suite
	enforcer *middleware.TierEnforcer
}

func TestTiersSuite(t *testing.T) {
	suite.Run(t, new(TiersSuite))
}

func (s *TiersSuite) SetupTest() {
	s.enforcer = middleware.NewTierEnforcer()
}

func makeRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func (s *TiersSuite) TestR4Blocked() {
	err := s.enforcer.Check(middleware.TierR4, makeRequest(nil))
	s.Error(err)
	s.Contains(err.Error(), "forbidden")
}

func (s *TiersSuite) TestR0Allowed() {
	err := s.enforcer.Check(middleware.TierR0, makeRequest(nil))
	s.NoError(err)
}

func (s *TiersSuite) TestR1Allowed() {
	err := s.enforcer.Check(middleware.TierR1, makeRequest(nil))
	s.NoError(err)
}

func (s *TiersSuite) TestIsConfirmedTrue() {
	req := makeRequest(map[string]any{"confirmed": true})
	s.True(middleware.IsConfirmed(req))
}

func (s *TiersSuite) TestIsConfirmedFalse() {
	req := makeRequest(map[string]any{"confirmed": false})
	s.False(middleware.IsConfirmed(req))
}

func (s *TiersSuite) TestIsConfirmedMissing() {
	req := makeRequest(map[string]any{"other": "param"})
	s.False(middleware.IsConfirmed(req))
}

func (s *TiersSuite) TestIsConfirmedNilArgs() {
	req := makeRequest(nil)
	s.False(middleware.IsConfirmed(req))
}

func (s *TiersSuite) TestIsConfirmedWrongType() {
	req := makeRequest(map[string]any{"confirmed": "yes"})
	s.False(middleware.IsConfirmed(req))
}

func (s *TiersSuite) TestCheckConfirmationR1NoConfirm() {
	req := makeRequest(map[string]any{})
	err := s.enforcer.CheckConfirmation(middleware.TierR1, req)
	s.ErrorIs(err, middleware.ErrNotConfirmed)
}

func (s *TiersSuite) TestCheckConfirmationR1Confirmed() {
	req := makeRequest(map[string]any{"confirmed": true})
	err := s.enforcer.CheckConfirmation(middleware.TierR1, req)
	s.NoError(err)
}

func (s *TiersSuite) TestCheckConfirmationR0AlwaysAllowed() {
	req := makeRequest(map[string]any{})
	err := s.enforcer.CheckConfirmation(middleware.TierR0, req)
	s.NoError(err)
}

func (s *TiersSuite) TestRequiresConfirmationR0() {
	s.False(middleware.RequiresConfirmation(middleware.TierR0))
}

func (s *TiersSuite) TestRequiresConfirmationR1() {
	s.True(middleware.RequiresConfirmation(middleware.TierR1))
}

func (s *TiersSuite) TestRequiresConfirmationR2() {
	s.True(middleware.RequiresConfirmation(middleware.TierR2))
}

func (s *TiersSuite) TestRequiresConfirmationR3() {
	s.True(middleware.RequiresConfirmation(middleware.TierR3))
}

func (s *TiersSuite) TestRequiresConfirmationR4() {
	s.False(middleware.RequiresConfirmation(middleware.TierR4))
}
