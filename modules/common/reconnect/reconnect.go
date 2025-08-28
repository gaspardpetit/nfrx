package reconnect

import "time"

// Schedule defines the backoff durations for successive reconnect attempts.
var Schedule = []time.Duration{
	time.Second, time.Second, time.Second,
	5 * time.Second, 5 * time.Second, 5 * time.Second,
	15 * time.Second, 15 * time.Second, 15 * time.Second,
}

// Delay returns the backoff duration for the given attempt.
// Attempts beyond the length of the schedule default to 30 seconds.
func Delay(attempt int) time.Duration {
	if attempt < len(Schedule) {
		return Schedule[attempt]
	}
	return 30 * time.Second
}
