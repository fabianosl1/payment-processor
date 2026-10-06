package domain_test

import (
	"testing"

	"github.com/huncoding/payment-processor-course/internal/fraud/domain"
)

func TestApplyFastRules(t *testing.T) {
	pass := domain.RuleResult{Verdict: domain.VerdictApprove, Reason: "fast_rules_passed"}
	blocked := domain.RuleResult{Verdict: domain.VerdictDeny, Reason: "card_blocklisted", Final: true}
	tests := []struct {
		name    string
		attempt domain.Attempt
		want    domain.RuleResult
	}{
		{"happy path", domain.Attempt{CardFingerPrint: "allowed", AmountCents: 1000, VelocityLastMin: 1}, pass},
		{"first blocked card takes precedence", domain.Attempt{CardFingerPrint: "fp_stolen_card_001", AmountCents: domain.MaxAmountCents + 1, VelocityLastMin: domain.VelocityPerMinLimit + 1}, blocked},
		{"second blocked card", domain.Attempt{CardFingerPrint: "fp_stolen_card_002"}, blocked},
		{"below limits", domain.Attempt{AmountCents: domain.MaxAmountCents - 1, VelocityLastMin: domain.VelocityPerMinLimit - 1}, pass},
		{"at limits", domain.Attempt{AmountCents: domain.MaxAmountCents, VelocityLastMin: domain.VelocityPerMinLimit}, pass},
		{"amount above limit takes precedence", domain.Attempt{AmountCents: domain.MaxAmountCents + 1, VelocityLastMin: domain.VelocityPerMinLimit + 1}, domain.RuleResult{Verdict: domain.VerdictManualReview, Reason: "amount_above_hard_limit", Final: true}},
		{"velocity above limit", domain.Attempt{AmountCents: domain.MaxAmountCents, VelocityLastMin: domain.VelocityPerMinLimit + 1}, domain.RuleResult{Verdict: domain.VerdictDeny, Reason: "velocity_limit_exceeded", Final: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.ApplyFastRules(tt.attempt); got != tt.want {
				t.Errorf("ApplyFastRules() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
