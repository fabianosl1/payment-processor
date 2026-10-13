package infra

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

const EmbeddingDim = 256

type HashingEmbedder struct{}

func NewHashingEmbedder() *HashingEmbedder {
	return &HashingEmbedder{}
}

func (e *HashingEmbedder) Embed(text string) []float64 {
	vec := make([]float64, EmbeddingDim)
	tokens := strings.Fields(strings.ToLower(text))

	for _, t := range tokens {
		for _, g := range ngrams(t, 3) {
			h := sha256.Sum256([]byte(g))
			idx := binary.BigEndian.Uint32(h[:4]) % EmbeddingDim
			sign := 1.0
			if h[4] % 2 == 0 {
				sign = -1.0
			}
			vec[idx] += sign
		}
	}
	normalize(vec)
	return vec
}


func ngrams(s string, n int) []string {
	if len(s) <= n {
		return []string{s}
	}
	out := make([]string, 0, len(s)-n+1)
	for i := 0; i + n <= len(s); i++ {
		out = append(out, s[i:i+n])
	}
	return out
}

func normalize(v []float64) {
	var sum float64
	for _, x := range v {
		sum += x * x
	}
	if sum == 0 { return }
	norm := math.Sqrt(sum)
	for i := range v {
		v[i] /= norm
	}
}

func (e *HashingEmbedder) DefineEmbedder(g *genkit.Genkit) ai.Embedder {
	return genkit.DefineEmbedderAction[any](
		g,
		"hashing",
		&ai.EmbedderOptions{Dimensions: EmbeddingDim},
		func(_ context.Context, re *ai.EmbedRequest, a any) (*ai.EmbedResponse, error) {
			embeddings := make([]*ai.Embedding, len(re.Input))
			for i, doc := range re.Input {
				embeddings[i] = &ai.Embedding{Embedding: toFloat32(e.Embed(docText(doc)))}
			}
			return &ai.EmbedResponse{Embeddings: embeddings}, nil
		},
	)
}

func docText(doc *ai.Document) string {
	var b strings.Builder
	for _, part := range doc.Content {
		b.WriteString(part.Text)
	}
	return b.String()
}

func toFloat32(v []float64) []float32 {
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(x)
	}
	return  out
}

func Cosine(a, b []float64) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("cosine: dimension mismatch %d vs %d", len(a), len(b))
	}

	var dot, na, nb float64

	for i, x := range a {
		dot += x * b[i]
		na += x * x
		nb += b[i] * b[i]
	}

	if na == 0 || nb == 0 {
		return 0, nil
	}

	return dot / (math.Sqrt(na) * math.Sqrt(nb)), nil
}
