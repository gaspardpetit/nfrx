package ctrl

// Job represents a job request/response lifecycle.
type Job struct {
	ID        string
	Responses chan interface{}
}
