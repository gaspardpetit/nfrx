package docling

import baseworker "github.com/gaspardpetit/nfrx/sdk/base/worker"

// AlwaysEligibleScorer returns 1.0 for all workers, allowing least-busy selection.
type AlwaysEligibleScorer struct{}

func (AlwaysEligibleScorer) Score(task string, w *baseworker.Worker) float64 { return 1.0 }

