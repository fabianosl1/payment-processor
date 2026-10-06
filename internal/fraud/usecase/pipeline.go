// Package use case implements the fraud scoring pipeline as an
// explicit state machine.
package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/huncoding/payment-processor-course/internal/fraud/domain"
	"github.com/huncoding/payment-processor-course/pkg/batch"
	"github.com/huncoding/payment-processor-course/pkg/fsm"
	"github.com/huncoding/payment-processor-course/pkg/memcache"
)

// FSM states of the scoring pipeline
const (
	StateEnrich    = "enrich"
	StateFastRules = "fast_rules"
	StateMLScore   = "ml_score"
	StateTiebreak  = "tiebreak"
	StateFinalize  = "finalize"
	StateDone      = "done"
)

// GeoResolver enriches the attempt with a country code
type GeoResolver interface {
	CountryCode(ctx context.Context, ip string) (string, error)
}

// Embedder convert text to a vector for the semantic cache
type Embedder interface {
	Embed(text string) []float64
}

// SemanticCache resolve ambiguos scores by similarity to recently
type SemanticCache interface {
	Lookup(vector []float64) (domain.Verdict, float64, bool)
	Store(vector []float64, verdict domain.Verdict)
}

// Decisision is the pipeline output.
type Decision struct {
	Verdict   domain.Verdict
	RiskScore float64
	Reasons   []string
	Degraded  bool
}

type scoringData struct {
	attempt domain.Attempt
	verdict domain.Verdict
	final   bool
	vector  []float64
}

// Pipeline score payment attempts
type Pipeline struct {
	machine  *fsm.Machine[scoringData]
	model    *domain.TreeModel
	geo      GeoResolver
	embedder Embedder
	semCache SemanticCache
	velocity *memcache.Cache[int]
	batcher  *batch.Batcher[[]float64, float64]
	logger   *slog.Logger
}

func NewPipeline(
	model *domain.TreeModel,
	geo GeoResolver,
	embedder Embedder,
	semCache SemanticCache,
	logger *slog.Logger,
	batchSize int,
	batchWait time.Duration,
) *Pipeline {
	p := &Pipeline{
		model:    model,
		geo:      geo,
		embedder: embedder,
		semCache: semCache,
		logger:   logger,
		velocity: memcache.New[int](time.Minute, 30*time.Second),
	}

	p.batcher = batch.New(batchSize, batchWait, func(_ context.Context, vectors [][]float64) ([]float64, error) {
		scores := make([]float64, len(vectors))
		for i, vector := range vectors {
			s, err := model.Score(vector)
			if err != nil {
				p.logger.Warn("model scoring failed", "error", err)
				return nil, err
			}
			scores[i] = s
		}
		return scores, nil
	})

	p.machine = fsm.New[scoringData](StateEnrich).
		Handle(StateEnrich, p.enrich).
		Handle(StateFastRules, p.fastRules).
		Handle(StateMLScore, p.mlScore).
		FallbackTo(StateMLScore, StateFinalize).
		Handle(StateTiebreak, p.tiebreak).
		Handle(StateFinalize, p.finalize).
		Terminal(StateDone)

	return p
}

func (p *Pipeline) Close() {
	p.batcher.Close()
	p.velocity.Close()
}

func (p *Pipeline) Score(ctx context.Context, attempt domain.Attempt) (Decision, []fsm.Transition, error) {
	data := scoringData{attempt: attempt}
	trace, err := p.machine.Run(ctx, &data)
	if err != nil {
		return Decision{}, trace, fmt.Errorf("fraud pipeline: %w", err)
	}

	return Decision{
		Verdict:   data.verdict,
		RiskScore: data.attempt.RiskScore,
		Reasons:   data.attempt.Reasons,
		Degraded:  data.attempt.Degraded,
	}, trace, nil
}

func (p *Pipeline) enrich(ctx context.Context, d *scoringData) (fsm.State, error) {
	var err error
	d.attempt.VelocityLastMin, err = p.velocity.Increment(
		"card:"+d.attempt.CardFingerPrint,
		func(current int, exist bool) int {
			if !exist {
				return 1
			}
			return current + 1
		})
	if err != nil {
		return "", fmt.Errorf("velocity cache: %w", err)
	}

	code, err := p.geo.CountryCode(ctx, d.attempt.ClientIP)
	if err != nil {
		p.logger.Warn("geo lookup failed", slog.String("payment_id", d.attempt.PaymentID), slog.String("error", err.Error()))
		code = ""
	}

	d.attempt.CountryCode = code

	return StateFastRules, nil
}

func (p *Pipeline) fastRules(ctx context.Context, d *scoringData) (fsm.State, error) {
	result := domain.ApplyFastRules(d.attempt)
	d.attempt.Reasons = append(d.attempt.Reasons, result.Reason)

	if result.Final {
		d.verdict = result.Verdict
		d.final = true
		return StateDone, nil
	}

	return StateMLScore, nil
}

func (p *Pipeline) mlScore(ctx context.Context, d *scoringData) (fsm.State, error) {
	features := domain.FeatureVector(d.attempt)
	score, err := p.batcher.Submit(ctx, features)
	if err != nil {
		d.attempt.Degraded = true
		d.attempt.Reasons = append(d.attempt.Reasons, "ml_stage_skipped")
		d.verdict = domain.VerdictManualReview
		return "", fmt.Errorf("ml score: %w", err)
	}

	d.attempt.RiskScore = score
	d.verdict = domain.VerdictFromScore(score)

	if d.verdict == domain.VerdictManualReview {
		return StateTiebreak, nil
	}

	d.attempt.Reasons = append(d.attempt.Reasons, "ml_score_confident")
	return StateDone, nil
}

func (p *Pipeline) finalize(ctx context.Context, d *scoringData) (fsm.State, error) {
	if d.verdict == "" {
		d.verdict = domain.VerdictManualReview
	}

	if d.vector != nil {
		p.semCache.Store(d.vector, d.verdict)
	}

	return StateDone, nil
}

func (p *Pipeline) tiebreak(_ context.Context, d *scoringData) (fsm.State, error) {
	sumary := fmt.Sprintf("%s %s bucket: %d country: %s", d.attempt.MerchantCategory, d.attempt.CardFingerPrint, d.attempt.AmountCents/50000, d.attempt.CountryCode)
	d.vector = p.embedder.Embed(sumary)
	verdict, similarity, hit := p.semCache.Lookup(d.vector)
	if hit {
		d.verdict = verdict
		d.attempt.Reasons = append(d.attempt.Reasons, fmt.Sprintf("semantic_cache_hit:%.2f", similarity))
		return StateFinalize, nil
	}

	d.attempt.Reasons = append(d.attempt.Reasons, "ambiguos_score_manual_review")
	return StateFinalize, nil
}
