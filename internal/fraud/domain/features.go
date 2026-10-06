// Package domain defines the fraude scoring model: input featureas,
// rules outcomes, the final decision contract.
package domain

import "time"

// Attempt is the payment attempt as seem by the fraud pipeline.
type Attempt struct {
	PaymentID        string
	AccountID        string
	CardFingerPrint  string
	AmountCents      int64
	Currency         string
	ClientIP         string
	OccuredAt        time.Time
	MerchantCategory string

	// Enriched during the pipeline run
	CountryCode     string
	VelocityLastMin int
	RiskScore       float64
	Reasons         []string
	Degraded        bool
}

// Feature names used by the decision tree model
const (
	FeatureAmountCents    = 0
	FeatureVelocityLasMin = 1
	FeatureNightTime      = 2
	FeatureForeignCountry = 3
	FeatureHighRiskMCC    = 4
	FeatureCount          = 5
)

// HighRiskMerchantCategories flag with elevated chargeback
var HighRiskMerchantCategories = map[string]bool{
	"gambling":       true,
	"crypto":         true,
	"gift_cards":     true,
	"money_transfer": true,
}

// featureVecor converts the enriched attemp into the numeric vector
func FeatureVector(a Attempt) []float64 {
	v := make([]float64, FeatureCount)
	v[FeatureAmountCents] = float64(a.AmountCents)
	v[FeatureVelocityLasMin] = float64(a.VelocityLastMin)
	hour := a.OccuredAt.Hour()

	if hour < 6 {
		v[FeatureNightTime] = 1
	}
	if a.CountryCode != "" && a.CountryCode != "BR" {
		v[FeatureForeignCountry] = 1
	}
	if HighRiskMerchantCategories[a.MerchantCategory] {
		v[FeatureHighRiskMCC] = 1
	}
	return v
}

// Veriddict values retunerd by the pipeline
type Verdict string

const (
	VerdictApprove      Verdict = "approve"
	VerdictDeny         Verdict = "denny"
	VerdictManualReview Verdict = "manual_review"
)

// Thresholds convert a continuos risk score into a verdict
const (
	ApproveBellow = 0.35
	DennyAbove    = 0.75
)

// VerdictFromScore applies the banding rule
func VerdictFromScore(score float64) Verdict {
	switch {
	case score < ApproveBellow:
		return VerdictApprove
	case score > DennyAbove:
		return VerdictDeny
	default:
		return VerdictManualReview
	}
}
