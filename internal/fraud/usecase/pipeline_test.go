package usecase

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/huncoding/payment-processor-course/internal/fraud/domain"
	"github.com/huncoding/payment-processor-course/pkg/batch"
	"github.com/huncoding/payment-processor-course/pkg/fsm"
	"github.com/huncoding/payment-processor-course/pkg/memcache"
)

type geoStub struct {
	code  string
	err   error
	calls int
	ip    string
}

func (g *geoStub) CountryCode(_ context.Context, ip string) (string, error) {
	g.calls++
	g.ip = ip
	return g.code, g.err
}

type embedderStub struct {
	text  string
	calls int
}

func (e *embedderStub) Embed(text string) []float64 { e.calls++; e.text = text; return []float64{1, 2} }

type semanticStub struct {
	verdict domain.Verdict
	hit     bool
	lookups int
	stores  int
	vector  []float64
	stored  domain.Verdict
}

func (s *semanticStub) Lookup(v []float64) (domain.Verdict, float64, bool) {
	s.lookups++
	s.vector = v
	return s.verdict, 0.92, s.hit
}
func (s *semanticStub) Store(v []float64, verdict domain.Verdict) {
	s.stores++
	s.vector = v
	s.stored = verdict
}

func newTestPipeline(t *testing.T, baseline float64) (*Pipeline, *geoStub, *embedderStub, *semanticStub) {
	t.Helper()
	geo := &geoStub{code: "BR"}
	embedder := &embedderStub{}
	cache := &semanticStub{}
	model := &domain.TreeModel{Features: domain.FeatureCount, Baseline: baseline, Trees: []domain.Tree{{Nodes: []domain.Node{{Left: -1}}}}}
	p := NewPipeline(model, geo, embedder, cache, slog.New(slog.NewTextHandler(io.Discard, nil)), 1, time.Millisecond)
	t.Cleanup(p.Close)
	return p, geo, embedder, cache
}
func testAttempt() domain.Attempt {
	return domain.Attempt{PaymentID: "payment-1", CardFingerPrint: "card-1", AmountCents: 100_000, ClientIP: "127.0.0.1", MerchantCategory: "retail", OccuredAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
}
func assertTrace(t *testing.T, trace []fsm.Transition, states ...fsm.State) {
	t.Helper()
	if len(trace) != len(states)-1 {
		t.Fatalf("trace = %+v, want states %v", trace, states)
	}
	for i, tr := range trace {
		if tr.From != states[i] || tr.To != states[i+1] {
			t.Errorf("transition %d = %s -> %s, want %s -> %s", i, tr.From, tr.To, states[i], states[i+1])
		}
	}
}

func TestPipelineScoreConfident(t *testing.T) {
	for _, tt := range []struct {
		name     string
		baseline float64
		verdict  domain.Verdict
	}{{"approve", -2, domain.VerdictApprove}, {"deny", 2, domain.VerdictDeny}} {
		t.Run(tt.name, func(t *testing.T) {
			p, geo, embedder, cache := newTestPipeline(t, tt.baseline)
			got, trace, err := p.Score(context.Background(), testAttempt())
			if err != nil {
				t.Fatal(err)
			}
			want := Decision{Verdict: tt.verdict, RiskScore: 1 / (1 + math.Exp(-tt.baseline)), Reasons: []string{"fast_rules_passed", "ml_score_confident"}}
			if got.Verdict != want.Verdict || math.Abs(got.RiskScore-want.RiskScore) > 1e-12 || !reflect.DeepEqual(got.Reasons, want.Reasons) || got.Degraded {
				t.Errorf("decision = %+v, want %+v", got, want)
			}
			assertTrace(t, trace, StateEnrich, StateFastRules, StateMLScore, StateDone)
			for _, tr := range trace {
				if tr.Err != nil {
					t.Errorf("unexpected transition error: %v", tr.Err)
				}
			}
			if geo.calls != 1 || geo.ip != testAttempt().ClientIP || embedder.calls != 0 || cache.lookups != 0 || cache.stores != 0 {
				t.Errorf("unexpected dependency calls: geo=%+v embedder=%+v cache=%+v", geo, embedder, cache)
			}
		})
	}
}

func TestPipelineFastRules(t *testing.T) {
	for _, tt := range []struct {
		name, card string
		amount     int64
		velocity   int
		verdict    domain.Verdict
		reason     string
	}{
		{"blocklisted", "fp_stolen_card_001", 100, 0, domain.VerdictDeny, "card_blocklisted"},
		{"amount limit", "card-1", domain.MaxAmountCents + 1, 0, domain.VerdictManualReview, "amount_above_hard_limit"},
		{"velocity limit", "card-1", 100, domain.VelocityPerMinLimit, domain.VerdictDeny, "velocity_limit_exceeded"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, _, embedder, cache := newTestPipeline(t, 0)
			a := testAttempt()
			a.CardFingerPrint = tt.card
			a.AmountCents = tt.amount
			if tt.velocity > 0 {
				if err := p.velocity.Set("card:"+tt.card, tt.velocity); err != nil {
					t.Fatal(err)
				}
			}
			p.batcher.Close() // Fast rules must finish without submitting to ML.
			got, trace, err := p.Score(context.Background(), a)
			if err != nil {
				t.Fatal(err)
			}
			want := Decision{Verdict: tt.verdict, Reasons: []string{tt.reason}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("decision = %+v, want %+v", got, want)
			}
			assertTrace(t, trace, StateEnrich, StateFastRules, StateDone)
			if embedder.calls != 0 || cache.lookups != 0 || cache.stores != 0 {
				t.Fatal("fast rules called semantic dependencies")
			}
		})
	}
}

func TestPipelineSemanticTiebreak(t *testing.T) {
	for _, tt := range []struct {
		name    string
		hit     bool
		verdict domain.Verdict
		reason  string
	}{
		{"miss", false, domain.VerdictManualReview, "ambiguos_score_manual_review"},
		{"approve hit", true, domain.VerdictApprove, "semantic_cache_hit:0.92"},
		{"deny hit", true, domain.VerdictDeny, "semantic_cache_hit:0.92"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, _, embedder, cache := newTestPipeline(t, 0)
			cache.hit = tt.hit
			cache.verdict = tt.verdict
			got, trace, err := p.Score(context.Background(), testAttempt())
			if err != nil {
				t.Fatal(err)
			}
			want := Decision{Verdict: tt.verdict, RiskScore: 0.5, Reasons: []string{"fast_rules_passed", tt.reason}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("decision = %+v, want %+v", got, want)
			}
			assertTrace(t, trace, StateEnrich, StateFastRules, StateMLScore, StateTiebreak, StateFinalize, StateDone)
			if embedder.calls != 1 || embedder.text != "retail card-1 bucket: 2 country: BR" || cache.lookups != 1 || cache.stores != 1 || cache.stored != tt.verdict || !reflect.DeepEqual(cache.vector, []float64{1, 2}) {
				t.Errorf("unexpected semantic calls: embedder=%+v cache=%+v", embedder, cache)
			}
		})
	}
}

func TestPipelineGeoFailureContinues(t *testing.T) {
	p, geo, embedder, _ := newTestPipeline(t, 0)
	geo.err = errors.New("geo unavailable")
	_, trace, err := p.Score(context.Background(), testAttempt())
	if err != nil {
		t.Fatal(err)
	}
	assertTrace(t, trace, StateEnrich, StateFastRules, StateMLScore, StateTiebreak, StateFinalize, StateDone)
	if embedder.text != "retail card-1 bucket: 2 country: " {
		t.Errorf("summary = %q, want empty country on geo failure", embedder.text)
	}
}

func TestPipelineMLFailureFallsBack(t *testing.T) {
	for _, name := range []string{"invalid model", "closed batcher"} {
		t.Run(name, func(t *testing.T) {
			p, _, embedder, cache := newTestPipeline(t, 0)
			if name == "invalid model" {
				p.model.Features++
			} else {
				p.batcher.Close()
			}
			got, trace, err := p.Score(context.Background(), testAttempt())
			if err != nil {
				t.Fatal(err)
			}
			want := Decision{Verdict: domain.VerdictManualReview, Degraded: true, Reasons: []string{"fast_rules_passed", "ml_stage_skipped"}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("decision = %+v, want %+v", got, want)
			}
			assertTrace(t, trace, StateEnrich, StateFastRules, StateMLScore, StateFinalize, StateDone)
			if trace[2].Err == nil || !strings.Contains(trace[2].Err.Error(), "ml score:") {
				t.Errorf("ML transition error = %v", trace[2].Err)
			}
			if name == "closed batcher" && !errors.Is(trace[2].Err, batch.ErrClosed) {
				t.Errorf("error = %v, want batch.ErrClosed", trace[2].Err)
			}
			if embedder.calls != 0 || cache.lookups != 0 || cache.stores != 0 {
				t.Fatal("ML failure called semantic dependencies")
			}
		})
	}
}

func TestPipelineScoreErrors(t *testing.T) {
	for _, name := range []string{"canceled context", "expired deadline", "closed velocity cache"} {
		t.Run(name, func(t *testing.T) {
			p, geo, _, _ := newTestPipeline(t, 0)
			ctx := context.Background()
			var want error
			switch name {
			case "canceled context":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				want = context.Canceled
			case "expired deadline":
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer cancel()
				want = context.DeadlineExceeded
			default:
				if err := p.velocity.Close(); err != nil {
					t.Fatal(err)
				}
				want = memcache.ErrClosed
			}
			got, trace, err := p.Score(ctx, testAttempt())
			if !errors.Is(err, want) || !strings.HasPrefix(err.Error(), "fraud pipeline:") {
				t.Fatalf("error = %v, want wrapped %v", err, want)
			}
			if !reflect.DeepEqual(got, Decision{}) {
				t.Errorf("decision = %+v, want zero", got)
			}
			if name == "closed velocity cache" {
				assertTrace(t, trace, StateEnrich, "")
				if !errors.Is(trace[0].Err, want) {
					t.Errorf("transition error = %v", trace[0].Err)
				}
			} else if len(trace) != 0 {
				t.Errorf("trace = %+v, want empty", trace)
			}
			if geo.calls != 0 {
				t.Fatal("geo called after early failure")
			}
		})
	}
}
