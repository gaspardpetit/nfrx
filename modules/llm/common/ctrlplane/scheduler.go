package ctrlplane

import "errors"

type Scheduler interface { PickWorker(model string) (*Worker, error) }

type LeastBusyScheduler struct { Reg *Registry }

func (s *LeastBusyScheduler) PickWorker(model string) (*Worker, error) {
    workers := s.Reg.WorkersForModel(model)
    if len(workers) == 0 {
        workers = s.Reg.WorkersForAlias(model)
        if len(workers) == 0 { return nil, errors.New("no worker") }
    }
    best := workers[0]
    for _, w := range workers[1:] { if w.InFlight < best.InFlight { best = w } }
    return best, nil
}

