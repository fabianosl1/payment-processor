package domain_test

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/huncoding/payment-processor-course/internal/fraud/domain"
)

func TestFeatureVector(t *testing.T) {
	tests := []struct {
		name              string
		hour              int
		country, category string
		want              []float64
	}{
		{"domestic daytime", 12, "BR", "retail", []float64{12500, 3, 0, 0, 0}},
		{"midnight gambling", 0, "US", "gambling", []float64{12500, 3, 1, 1, 1}},
		{"last night hour crypto", 5, "", "crypto", []float64{12500, 3, 1, 0, 1}},
		{"first daytime hour gift cards", 6, "BR", "gift_cards", []float64{12500, 3, 0, 0, 1}},
		{"last hour money transfer", 23, "BR", "money_transfer", []float64{12500, 3, 0, 0, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := domain.Attempt{AmountCents: 12500, VelocityLastMin: 3, CountryCode: tt.country, MerchantCategory: tt.category,
				OccuredAt: time.Date(2026, 1, 1, tt.hour, 0, 0, 0, time.FixedZone("local", -3*60*60))}
			if got := domain.FeatureVector(a); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FeatureVector() = %v, want %v", got, tt.want)
			}
		})
	}
	t.Run("zero attempt", func(t *testing.T) {
		if got, want := domain.FeatureVector(domain.Attempt{}), []float64{0, 0, 1, 0, 0}; !reflect.DeepEqual(got, want) {
			t.Errorf("FeatureVector() = %v, want %v", got, want)
		}
	})
}

func TestVerdictFromScore(t *testing.T) {
	tests := []struct {
		name  string
		score float64
		want  domain.Verdict
	}{
		{"minimum", 0, domain.VerdictApprove},
		{"below approval boundary", math.Nextafter(domain.ApproveBellow, math.Inf(-1)), domain.VerdictApprove},
		{"approval boundary", domain.ApproveBellow, domain.VerdictManualReview},
		{"above approval boundary", math.Nextafter(domain.ApproveBellow, math.Inf(1)), domain.VerdictManualReview},
		{"below denial boundary", math.Nextafter(domain.DennyAbove, math.Inf(-1)), domain.VerdictManualReview},
		{"denial boundary", domain.DennyAbove, domain.VerdictManualReview},
		{"above denial boundary", math.Nextafter(domain.DennyAbove, math.Inf(1)), domain.VerdictDeny},
		{"maximum", 1, domain.VerdictDeny},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.VerdictFromScore(tt.score); got != tt.want {
				t.Errorf("VerdictFromScore(%g) = %q, want %q", tt.score, got, tt.want)
			}
		})
	}
}
