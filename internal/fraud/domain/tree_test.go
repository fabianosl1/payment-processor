package domain_test

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/huncoding/payment-processor-course/internal/fraud/domain"
)

func validModel() domain.TreeModel {
	return domain.TreeModel{
		Version: "v1", Features: 2, Baseline: 0.25,
		Trees: []domain.Tree{
			{Nodes: []domain.Node{
				{Feature: 0, Threshold: 10, Left: 1, Right: 2},
				{Left: -1, Value: -1},
				{Feature: 1, Threshold: 5, Left: 3, Right: 4},
				{Left: -1, Value: 0.5},
				{Left: -1, Value: 1},
			}},
			{Nodes: []domain.Node{{Left: -1, Value: 0.25}}},
		},
	}
}

func TestParseTreeModel(t *testing.T) {
	t.Run("happy path preserves model", func(t *testing.T) {
		want := validModel()
		data, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		got, err := domain.ParseTreeModel(data)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, &want) {
			t.Errorf("ParseTreeModel() = %+v, want %+v", got, &want)
		}
	})
	tests := []struct {
		name string
		data string
	}{
		{"malformed JSON", `{"features":`},
		{"wrong field type", `{"features":"two"}`},
		{"zero features", `{"features":0,"trees":[{"nodes":[{"left":-1}]}]}`},
		{"negative features", `{"features":-1,"trees":[{"nodes":[{"left":-1}]}]}`},
		{"no trees", `{"features":1,"trees":[]}`},
		{"empty second tree", `{"features":1,"trees":[{"nodes":[{"left":-1}]},{"nodes":[]}]}`},
		{"negative feature", `{"features":1,"trees":[{"nodes":[{"feature":-1,"left":1,"right":1},{"left":-1}]}]}`},
		{"feature at count", `{"features":1,"trees":[{"nodes":[{"feature":1,"left":1,"right":1},{"left":-1}]}]}`},
		{"left below leaf sentinel", `{"features":1,"trees":[{"nodes":[{"left":-2,"right":1},{"left":-1}]}]}`},
		{"left at node count", `{"features":1,"trees":[{"nodes":[{"left":2,"right":1},{"left":-1}]}]}`},
		{"negative right", `{"features":1,"trees":[{"nodes":[{"left":1,"right":-1},{"left":-1}]}]}`},
		{"right at node count", `{"features":1,"trees":[{"nodes":[{"left":1,"right":2},{"left":-1}]}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ParseTreeModel([]byte(tt.data))
			if !errors.Is(err, domain.ErrModelInvalid) {
				t.Fatalf("error = %v, want ErrModelInvalid", err)
			}
			if got != nil {
				t.Errorf("invalid model = %+v, want nil", got)
			}
		})
	}
}

func TestTreeModelScore(t *testing.T) {
	m := validModel()
	tests := []struct {
		name     string
		features []float64
		logit    float64
	}{
		{"below root threshold", []float64{math.Nextafter(10, math.Inf(-1)), 6}, -0.5},
		{"at root threshold", []float64{10, 6}, -0.5},
		{"above root at nested threshold", []float64{math.Nextafter(10, math.Inf(1)), 5}, 1},
		{"above nested threshold", []float64{11, math.Nextafter(5, math.Inf(1))}, 1.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.Score(tt.features)
			if err != nil {
				t.Fatal(err)
			}
			want := 1 / (1 + math.Exp(-tt.logit))
			if math.Abs(got-want) > 1e-12 {
				t.Errorf("Score() = %.15g, want %.15g", got, want)
			}
		})
	}
	for _, count := range []int{1, 3} {
		t.Run("feature count "+strconv.Itoa(count), func(t *testing.T) {
			got, err := m.Score(make([]float64, count))
			if !errors.Is(err, domain.ErrModelInvalid) {
				t.Fatalf("error = %v, want ErrModelInvalid", err)
			}
			if got != 0 {
				t.Errorf("score on error = %g, want 0", got)
			}
		})
	}
}

func TestTreeModelScoreNumericalBoundaries(t *testing.T) {
	for _, baseline := range []float64{-31, -30, -0.5, 0, 0.5, 30, 31} {
		t.Run("baseline "+strconv.FormatFloat(baseline, 'g', -1, 64), func(t *testing.T) {
			m := domain.TreeModel{Features: 1, Baseline: baseline, Trees: []domain.Tree{{Nodes: []domain.Node{{Left: -1}}}}}
			got, err := m.Score([]float64{0})
			if err != nil {
				t.Fatal(err)
			}
			want := 1 / (1 + math.Exp(-baseline))
			if math.IsNaN(got) || got < 0 || got > 1 || math.Abs(got-want) > 1e-12 {
				t.Errorf("Score() = %.15g, want %.15g", got, want)
			}
		})
	}
}
