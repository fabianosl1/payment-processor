package domain

const (
	MaxAmountCents      = 5_000_000
	VelocityPerMinLimit = 10
)

type RuleResult struct {
	Verdict Verdict
	Reason  string
	// Final is true when the rule decission cannot be overriden
	// by later stages.
	Final bool
}

// Blocklist is a list of card numbers that should be blocked.
var BlockList = map[string]bool{
	"fp_stolen_card_001": true,
	"fp_stolen_card_002": true,
}

// ApplyFastRules runs the deterministic check thaht need no model.
func ApplyFastRules(a Attempt) RuleResult {
	if BlockList[a.CardFingerPrint] {
		return RuleResult{Verdict: VerdictDeny, Reason: "card_blocklisted", Final: true}
	}
	if a.AmountCents > MaxAmountCents {
		return RuleResult{Verdict: VerdictManualReview, Reason: "amount_above_hard_limit", Final: true}
	}
	if a.VelocityLastMin > VelocityPerMinLimit {
		return RuleResult{Verdict: VerdictDeny, Reason: "velocity_limit_exceeded", Final: true}
	}
	return RuleResult{Verdict: VerdictApprove, Reason: "fast_rules_passed", Final: false}
}
