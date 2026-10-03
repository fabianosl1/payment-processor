package fsm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/huncoding/payment-processor-course/pkg/fsm"
)

type payload struct {
	visited []string
	score   float64
}

func record(p *payload, name string) {
	p.visited = append(p.visited, name)
}

func TestRunHappyPath(t *testing.T) {
	tests := []struct {
		name      string
		wantTrace int
		wantScore float64
	}{
		{
			name:      "happy path",
			wantTrace: 2,
			wantScore: 0.9,
		},
	}

	handleStart := func(ctx context.Context, data *payload) (fsm.State, error) {
		record(data, "start")
		return "score", nil
	}

	handleScore := func(ctx context.Context, data *payload) (fsm.State, error) {
		record(data, "score")
		data.score = 0.9
		return "done", nil
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := payload{}

			m := fsm.New[payload]("start").
				Handle("start", handleStart).
				Handle("score", handleScore).
				Terminal("done")

			trace, err := m.Run(context.Background(), &p)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(trace) != tt.wantTrace {
				t.Errorf("expected %d transitions, got %d", tt.wantTrace, len(trace))
			}

			if p.score != tt.wantScore {
				t.Errorf("expected score %f, got %f", tt.wantScore, p.score)
			}
		})
	}
}


func TestFallbackKepsPipelineAvailable(t *testing.T) {
	dependencyDown := errors.New("upstream timeout")
	p := payload{}

	handleStart := func(ctx context.Context, data *payload) (fsm.State, error) {
		record(data, "start")
		return "ml", nil
	}

	handleMl := func(ctx context.Context, data *payload) (fsm.State, error) {
		record(data, "ml")
		return "", dependencyDown
	}
	handleRulesOnly := func(ctx context.Context, data *payload) (fsm.State, error) {
		record(data, "rules_only")
		return "done", nil
	}

	m := fsm.New[payload]("start").
		Handle("start", handleStart).
		Handle("ml", handleMl).
		Handle("rules_only", handleRulesOnly).
		FallbackTo("ml", "rules_only").
		Terminal("done")

	trace, err := m.Run(context.Background(), &p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(p.visited) == 0 || p.visited[len(p.visited)-1] != "rules_only" {
		t.Errorf("expected last visited rules_only, got %v", p.visited)
	}

	if trace[1].Err == nil {
		t.Error("expected trace[1].Err not nil")
	}
}


func TestErrorWithoutFallbackAborts(t *testing.T) {
	boom := errors.New("boom")
	p := payload{}
	handleStart := func(ctx context.Context, data *payload) (fsm.State, error) {
		return "", boom
	}
	m := fsm.New[payload]("start").Handle("start", handleStart).Terminal("done")

	_, err := m.Run(context.Background(), &p)

	if !errors.Is(err, boom) {
		t.Errorf("expected error %v, got %v", boom, err)
	}


}
