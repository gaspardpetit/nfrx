package worker

import (
    "errors"
)

// Scheduler picks the best worker for a given task key.
type Scheduler interface { PickWorker(task string) (*Worker, error) }

// Scorer computes a compatibility score between a task and a worker.
// A score <= 0 means the worker is ineligible. Higher scores are preferred.
type Scorer interface { Score(task string, w *Worker) float64 }

// DefaultExactMatchScorer scores 1.0 for exact model matches, 0.0 otherwise.
type DefaultExactMatchScorer struct{}

func (DefaultExactMatchScorer) Score(task string, w *Worker) float64 {
    if w == nil { return 0 }
    if w.Labels != nil && w.Labels[task] { return 1.0 }
    return 0.0
}

// ScoreThenLeastBusyScheduler selects all eligible workers (score > 0, has capacity),
// keeps only those with the highest score, then picks the least busy among them.
type ScoreThenLeastBusyScheduler struct {
    Reg     *Registry
    Scorer  Scorer
    // MinScore is the minimum score required for a worker to be considered.
    // If all workers score below MinScore, no worker is selected.
    MinScore float64
}

// NewScoreScheduler constructs a scheduler using the provided registry and scorer.
func NewScoreScheduler(reg *Registry, scorer Scorer) *ScoreThenLeastBusyScheduler {
    if scorer == nil { scorer = DefaultExactMatchScorer{} }
    return &ScoreThenLeastBusyScheduler{Reg: reg, Scorer: scorer}
}

// NewScoreSchedulerWithMinScore constructs a scheduler with a minimum score threshold.
func NewScoreSchedulerWithMinScore(reg *Registry, scorer Scorer, min float64) *ScoreThenLeastBusyScheduler {
    s := NewScoreScheduler(reg, scorer)
    s.MinScore = min
    return s
}

func (s *ScoreThenLeastBusyScheduler) PickWorker(task string) (*Worker, error) {
    s.Reg.mu.RLock()
    // snapshot pointers; we won't mutate workers inside lock except reading fields
    workers := make([]*Worker, 0, len(s.Reg.workers))
    for _, w := range s.Reg.workers { workers = append(workers, w) }
    s.Reg.mu.RUnlock()

    var (
        best []*Worker
        bestScore float64
    )
    for _, w := range workers {
        w.mu.Lock()
        capOK := w.InFlight < w.MaxConcurrency
        score := 0.0
        if capOK {
            // Score under worker lock to avoid races with model updates
            score = s.Scorer.Score(task, w)
        }
        w.mu.Unlock()
        if !capOK { continue }
        if score < s.MinScore { continue }
        if len(best) == 0 || score > bestScore {
            best = best[:0]
            best = append(best, w)
            bestScore = score
            continue
        }
        if score == bestScore { best = append(best, w) }
    }
    if len(best) == 0 { return nil, errors.New("no worker") }
    chosen := best[0]
    for _, w := range best[1:] {
        w.mu.Lock(); ci := w.InFlight; w.mu.Unlock()
        chosen.mu.Lock(); bi := chosen.InFlight; chosen.mu.Unlock()
        if ci < bi { chosen = w }
    }
    return chosen, nil
}
