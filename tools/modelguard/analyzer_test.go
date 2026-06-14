package modelguard

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	a, err := newAnalyzer(Settings{})
	if err != nil {
		t.Fatalf("newAnalyzer: %v", err)
	}
	analysistest.Run(t, analysistest.TestData(), a,
		"sample/internal/shop/internal/model",
		"sample/internal/shop/internal/service",
	)
}
