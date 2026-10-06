package domain

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrModelInvalid is returned when a model is invalid
var ErrModelInvalid = errors.New("fraud: invalid tree model")

type TreeModel struct {
	Version  string  `json:"version"`
	Features int     `json:"features"`
	Baseline float64 `json:"baseline"`
	Trees    []Tree  `json:"trees"`
}

type Tree struct {
	Nodes []Node `json:"nodes"`
}

type Node struct {
	Feature   int     `json:"feature"`
	Threshold float64 `json:"threshold"`
	Left      int     `json:"left"`
	Right     int     `json:"right"`
	Value     float64 `json:"value"`
}

func ParseTreeModel(data []byte) (*TreeModel, error) {
	var m TreeModel
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrModelInvalid, err)
	}
	if m.Features <= 0 || len(m.Trees) == 0 {
		return nil, fmt.Errorf("%w: empty model", ErrModelInvalid)
	}

	for ti, tree := range m.Trees {
		if len(tree.Nodes) == 0 {
			return nil, fmt.Errorf("%w: tree %d has no nodes", ErrModelInvalid, ti)
		}
		for ni, node := range tree.Nodes {
			if node.Left == -1 {
				continue
			}

			if node.Feature < 0 || node.Feature >= m.Features {
				return nil, fmt.Errorf("%w: node %d has invalid feature %d", ErrModelInvalid, ni, node.Feature)
			}
			if node.Left < 0 || node.Left >= len(tree.Nodes) || node.Right < 0 || node.Right >= len(tree.Nodes) {
				return nil, fmt.Errorf("%w: node %d has invalid left/right %d/%d", ErrModelInvalid, ni, node.Left, node.Right)
			}
		}
	}
	return &m, nil
}

func (m *TreeModel) Score(features []float64) (float64, error) {
	if len(features) != m.Features {
		return 0, fmt.Errorf("%w: expected features %d, got %d", ErrModelInvalid, m.Features, len(features))
	}

	score := m.Baseline
	for i := range m.Trees {
		score += m.Trees[i].eval(features)
	}

	return sigmoid(score), nil
}

func sigmoid(x float64) float64 {
	if x >= 0 {
		z := expNeg(x)
		return 1 / (1 + z)
	}

	z := expNeg(-x)
	return z / (1 + z)
}

func expNeg(x float64) float64 {
	if x > 30 { return 0 }

	half := 0
	for x > 0.5 {
		x /= 2
		half++
	}

	sum := 1.0
	term := 1.0
	for i := 1; i < 16; i++ {
		term *= x / float64(i)
		sum += term
	}

	for ; half > 0; half-- {
		sum *= sum
	}
	return 1 / sum
}

func (t *Tree) eval(features []float64) float64 {
	idx := 0
	for {
		node := t.Nodes[idx]
		if node.Left == -1 {
			return node.Value
		}
		if features[node.Feature] <= node.Threshold {
			idx = node.Left
		} else {
			idx = node.Right
		}
	}
}
