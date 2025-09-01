package asr

import (
	ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
	baseworker "github.com/gaspardpetit/nfrx/sdk/base/worker"
)

// asrScorer implements exact model match with alias fallback.
// Exact matches score 1.0, alias matches score 0.5, others 0.0.
type asrScorer struct{}

func NewASRScorer() baseworker.Scorer { return asrScorer{} }

func (asrScorer) Score(task string, w *baseworker.Worker) float64 {
	if w == nil {
		return 0
	}
	if w.Labels != nil && w.Labels[task] {
		return 1.0
	}
	if ak, ok := ctrl.AliasKey(task); ok {
		for m := range w.Labels {
			if mk, ok2 := ctrl.AliasKey(m); ok2 && mk == ak {
				return 0.5
			}
		}
	}
	return 0
}
