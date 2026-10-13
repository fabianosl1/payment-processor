package adapter

import (
	"strings"

	"github.com/huncoding/payment-processor-course/internal/platform/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	scoreDuration = metrics.NewHistogram(
		"frad_score_duration_seconds",
		"total duration of one scoring call",
	)
	stageDuration = metrics.NewHistogram(
		"fraud_stage_duration_seconds",
		"Duration of each FSM stage, from the pipeline trace.",
		"stage",
	)
	verdicts = metrics.NewCounter(
		"fraud_verdicts_total",
		"Scoring verdicts by outcome and degradation.",
		"verdict", "degraded",
	)
	reasons = metrics.NewCounter(
		"fraud_reasons_total",
		"Decicion reasons emitted by the pipeline stages.",
		"reason",
	)
	riskScores = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "fraud_risk_score",
		Help:    "Distribution of model risk scores for scored attempts.",
		Buckets: prometheus.LinearBuckets(0.05, 0.05, 19),
	})
)

func observerStage(stage string, seconds float64) {
	stageDuration.WithLabelValues(stage).Observe(seconds)
}

func countVerdict(verdict string, degraded bool) {
	d := "false"
	if degraded {
		d = "true"
	}
	verdicts.With(prometheus.Labels{"verdict": verdict, "degraded": d}).Inc()
}

func countReasons(list []string) {
	for _, reason := range list {
		if idx := strings.IndexByte(reason, ':'); idx > 0 {
			reason = reason[:idx]
		}
		reasons.With(prometheus.Labels{"reason": reason}).Inc()
	}
}

func observeRiskScore(score float64) {
	if score > 0 {
		riskScores.Observe(score)
	}
}
