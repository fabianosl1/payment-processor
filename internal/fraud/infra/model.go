package infra

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/huncoding/payment-processor-course/internal/fraud/domain"
)

//go:embed default_model.json
var defaultModel []byte

func LoadModel(path string) (*domain.TreeModel, error) {
	data := defaultModel

	if path != "" {
		var err error
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("model: failed to load model: %w", err)
		}
	}

	return domain.ParseTreeModel(data)
}
