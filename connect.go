package dccli

import (
	"math"
	"math/rand"
	"time"
)

const (
	defaultRetryCount     = 10
	defaultBaseRetryDelay = 100 * time.Millisecond
)

// connect attempts to connect to a container using the given connector function.
// Use retryCount and retryDelay to configure the number of retries and the time waited between them (using exponential backoff).
func connect(retryCount int, baseRetryDelay time.Duration, connectFunc func() error) error {
	var err error

	for i := 0; i < retryCount; i++ {
		err = connectFunc()
		if err == nil {
			return nil
		}
		time.Sleep(baseRetryDelay)
		baseRetryDelay *= 2
	}

	return err
}

type RetryPolicy interface {
	AttemptAgain(error) (bool, time.Duration)
}

func NewSimpleRetryPolicy(retries int, wait time.Duration) *SimpleRetryPolicy {
	return &SimpleRetryPolicy{NumRetries: retries, Wait: wait}
}

type SimpleRetryPolicy struct {
	NumRetries int //Number of times to retry a query
	Wait       time.Duration
	n          int
}

func (s *SimpleRetryPolicy) AttemptAgain(err error) (bool, time.Duration) {
	out := s.n < s.NumRetries
	s.n++
	return out, s.Wait
}

type ExponentialBackoffRetryPolicy struct {
	NumRetries int
	Min, Max   time.Duration
	n          int
}

func (e *ExponentialBackoffRetryPolicy) AttemptAgain(err error) (bool, time.Duration) {
	out := e.n < e.NumRetries
	outD := getExponentialTime(e.Min, e.Max, e.n)
	e.n++
	return out, outD
}

// used to calculate exponentially growing time
func getExponentialTime(min time.Duration, max time.Duration, attempts int) time.Duration {
	if min <= 0 {
		min = 100 * time.Millisecond
	}
	if max <= 0 {
		max = 10 * time.Second
	}
	minFloat := float64(min)
	napDuration := minFloat * math.Pow(2, float64(attempts-1))
	// add some jitter
	napDuration += rand.Float64()*minFloat - (minFloat / 2)
	if napDuration > float64(max) {
		return time.Duration(max)
	}
	return time.Duration(napDuration)
}
