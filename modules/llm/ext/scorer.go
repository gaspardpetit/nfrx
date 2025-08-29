package llm

import (
    ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
    baseworker "github.com/gaspardpetit/nfrx/sdk/base/worker"
)

// llmScorer implements a two-tier compatibility: exact model match > alias match.
// Exact matches score 1.0, alias matches score 0.5, others 0.0.
type llmScorer struct{}

func NewLLMScorer() baseworker.Scorer { return llmScorer{} }

func (llmScorer) Score(task string, w *baseworker.Worker) float64 {
    if w == nil { return 0 }
    if w.Models != nil && w.Models[task] { return 1.0 }
    // alias fallback
    if ak, ok := ctrl.AliasKey(task); ok {
        for m := range w.Models {
            if mk, ok2 := ctrl.AliasKey(m); ok2 && mk == ak {
                return 0.5
            }
        }
    }
    return 0
}
