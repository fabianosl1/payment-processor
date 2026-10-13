package infra

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

func TestHashingEmbedderEmbed(t *testing.T) {
	e := NewHashingEmbedder()
	for _, input := range []string{"", " \t\n", "a", "payment approved", "transação aprovada"} {
		t.Run(input, func(t *testing.T) {
			got := e.Embed(input)
			if len(got) != EmbeddingDim {
				t.Fatalf("dimension = %d, want %d", len(got), EmbeddingDim)
			}
			if !reflect.DeepEqual(got, e.Embed(input)) {
				t.Error("embedding is not deterministic")
			}
			var squaredNorm float64
			for _, value := range got {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					t.Fatalf("non-finite component: %v", value)
				}
				squaredNorm += value * value
			}
			want := 1.0
			if input == "" || input == " \t\n" {
				want = 0
			}
			if math.Abs(squaredNorm-want) > 1e-12 {
				t.Errorf("squared norm = %g, want %g", squaredNorm, want)
			}
		})
	}
	base := e.Embed("payment approved")
	for _, input := range []string{"PAYMENT APPROVED", " payment\tapproved\n", "approved payment", "payment approved payment approved"} {
		if !reflect.DeepEqual(base, e.Embed(input)) {
			t.Errorf("Embed(%q) differs from equivalent input", input)
		}
	}
	if reflect.DeepEqual(base, e.Embed("fraud denied")) {
		t.Error("distinct texts have identical embeddings")
	}
	base[0] = 42
	if e.Embed("payment approved")[0] == 42 {
		t.Error("embedding calls share mutable storage")
	}
}

func TestCosine(t *testing.T) {
	for _, tt := range []struct {
		name    string
		a, b    []float64
		want    float64
		wantErr bool
	}{
		{"same direction", []float64{3, 4}, []float64{6, 8}, 1, false},
		{"opposite", []float64{1, 0}, []float64{-1, 0}, -1, false},
		{"orthogonal", []float64{1, 0}, []float64{0, 1}, 0, false},
		{"partial similarity", []float64{1, 0}, []float64{1, 1}, 1 / math.Sqrt(2), false},
		{"zero first", []float64{0, 0}, []float64{1, 2}, 0, false},
		{"zero second", []float64{1, 2}, []float64{0, 0}, 0, false},
		{"empty", nil, nil, 0, false},
		{"dimension mismatch", []float64{1}, []float64{1, 2}, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Cosine(tt.a, tt.b)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Cosine() error = %v, want error %v", err, tt.wantErr)
			}
			if math.IsNaN(got) || math.Abs(got-tt.want) > 1e-12 {
				t.Errorf("Cosine() = %g, want %g", got, tt.want)
			}
		})
	}
}

func TestNgrams(t *testing.T) {
	for _, tt := range []struct {
		name, input string
		n           int
		want        []string
	}{
		{"empty", "", 3, []string{""}},
		{"short", "ab", 3, []string{"ab"}},
		{"exact length", "abc", 3, []string{"abc"}},
		{"overlapping", "abcde", 3, []string{"abc", "bcd", "cde"}},
		{"repeated", "aaaa", 3, []string{"aaa", "aaa"}},
		{"unigrams", "abc", 1, []string{"a", "b", "c"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ngrams(tt.input, tt.n); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ngrams() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	for _, tt := range []struct {
		name        string
		input, want []float64
	}{
		{"nil", nil, nil},
		{"empty", []float64{}, []float64{}},
		{"zero", []float64{0, 0}, []float64{0, 0}},
		{"positive", []float64{3, 4}, []float64{0.6, 0.8}},
		{"negative", []float64{-3, 4}, []float64{-0.6, 0.8}},
		{"unit", []float64{0, -1}, []float64{0, -1}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			normalize(tt.input)
			if !reflect.DeepEqual(tt.input, tt.want) {
				t.Errorf("normalize() = %v, want %v", tt.input, tt.want)
			}
		})
	}
}

func TestDocText(t *testing.T) {
	for _, tt := range []struct {
		name string
		doc  *ai.Document
		want string
	}{
		{"empty", &ai.Document{}, ""},
		{"multiple parts", &ai.Document{Content: []*ai.Part{{Text: "payment "}, {Text: "approved"}}}, "payment approved"},
		{"non-text part", &ai.Document{Content: []*ai.Part{{Text: "pay"}, {}, {Text: "ment"}}}, "payment"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := docText(tt.doc); got != tt.want {
				t.Errorf("docText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestToFloat32(t *testing.T) {
	input := []float64{0, 0.1, -0.5, 1}
	got := toFloat32(input)
	if want := []float32{0, 0.1, -0.5, 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("toFloat32() = %v, want %v", got, want)
	}
	got[0] = 42
	if input[0] != 0 {
		t.Error("conversion mutated input")
	}
	if len(toFloat32(nil)) != 0 {
		t.Error("nil input should produce an empty vector")
	}
}

func TestHashingEmbedderDefineEmbedder(t *testing.T) {
	t.Setenv("GENKIT_ENV", "prod")
	ctx := context.Background()
	e := NewHashingEmbedder()
	embedder := e.DefineEmbedder(genkit.Init(ctx))
	for _, tt := range []struct {
		name  string
		docs  []*ai.Document
		texts []string
	}{
		{"empty request", nil, nil},
		{"documents", []*ai.Document{
			{Content: []*ai.Part{{Text: "payment "}, {Text: "approved"}}},
			{Content: []*ai.Part{{Text: "fraud denied"}}},
			{},
		}, []string{"payment approved", "fraud denied", ""}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response, err := embedder.Embed(ctx, &ai.EmbedRequest{Input: tt.docs})
			if err != nil {
				t.Fatalf("Embed() error: %v", err)
			}
			if response == nil || len(response.Embeddings) != len(tt.docs) {
				t.Fatalf("response = %+v, want %d embeddings", response, len(tt.docs))
			}
			for i, text := range tt.texts {
				if response.Embeddings[i] == nil {
					t.Fatalf("embedding %d is nil", i)
				}
				if got, want := response.Embeddings[i].Embedding, toFloat32(e.Embed(text)); !reflect.DeepEqual(got, want) {
					t.Errorf("embedding %d differs from Embed(%q)", i, text)
				}
			}
		})
	}
}
