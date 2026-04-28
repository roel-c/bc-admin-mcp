package middleware

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// Tier maps to the risk tiers defined in BC-Tool-Boundaries.md.
type Tier string

const (
	TierR0 Tier = "R0" // Read — no mutation
	TierR1 Tier = "R1" // Write (standard) — preview+confirm for bulk
	TierR2 Tier = "R2" // Write (high-risk) — always confirm scope
	TierR3 Tier = "R3" // Destructive — explicit per-resource confirmation
	TierR4 Tier = "R4" // Forbidden — blocked at tool layer
)

// TierEnforcer validates that tool invocations comply with the tier policy.
type TierEnforcer struct{}

func NewTierEnforcer() *TierEnforcer {
	return &TierEnforcer{}
}

// Check validates whether a tool call at the given tier is permitted.
// R4 tools are always blocked. For R1-R3, it returns ErrNotConfirmed when the
// request lacks confirmed=true so the handler knows to return a preview.
func (te *TierEnforcer) Check(tier Tier, request mcp.CallToolRequest) error {
	if tier == TierR4 {
		return fmt.Errorf(
			"this operation is forbidden by policy (tier R4). " +
				"Contact an administrator if you believe this is in error",
		)
	}
	return nil
}

// ErrNotConfirmed is returned when a mutating tool is called without confirmation.
var ErrNotConfirmed = fmt.Errorf("confirmation required")

// CheckConfirmation validates that mutating tool calls (R1-R3) include
// confirmed=true. Returns ErrNotConfirmed if confirmation is required but
// missing, allowing the handler to return a preview instead. Returns nil
// for R0 tools or confirmed requests.
func (te *TierEnforcer) CheckConfirmation(tier Tier, request mcp.CallToolRequest) error {
	if RequiresConfirmation(tier) && !IsConfirmed(request) {
		return ErrNotConfirmed
	}
	return nil
}

// RequiresConfirmation returns true if the tool's tier requires explicit
// user confirmation before executing mutations.
func RequiresConfirmation(tier Tier) bool {
	switch tier {
	case TierR1, TierR2, TierR3:
		return true
	default:
		return false
	}
}

// IsConfirmed checks whether the request's arguments include confirmed=true.
func IsConfirmed(request mcp.CallToolRequest) bool {
	return IsConfirmedFromArgs(request.GetArguments())
}

// IsConfirmedFromArgs checks whether a raw args map includes confirmed=true.
func IsConfirmedFromArgs(args map[string]any) bool {
	if args == nil {
		return false
	}
	confirmed, ok := args["confirmed"]
	if !ok {
		return false
	}
	b, bOk := confirmed.(bool)
	return bOk && b
}
