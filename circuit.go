package sox

import (
	"errors"
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	StateClosed   CircuitState = iota // Normal operation
	StateOpen                         // Failing, reject requests
	StateHalfOpen                     // Testing if service recovered
)

// CircuitBreaker implements the circuit breaker pattern for SoX conversions
type CircuitBreaker struct {
	maxFailures      int
	resetTimeout     time.Duration
	halfOpenRequests int

	mu            sync.RWMutex
	state         CircuitState
	failures      int
	lastFailTime  time.Time
	successCount  int
	requestsInFly int
}

// NewCircuitBreaker creates a circuit breaker with default settings
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:      5,
		resetTimeout:     10 * time.Second,
		halfOpenRequests: 3,
		state:            StateClosed,
	}
}

// NewCircuitBreakerWithConfig creates a circuit breaker with custom settings
func NewCircuitBreakerWithConfig(maxFailures int, resetTimeout time.Duration, halfOpenRequests int) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:      maxFailures,
		resetTimeout:     resetTimeout,
		halfOpenRequests: halfOpenRequests,
		state:            StateClosed,
	}
}

var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

// Call executes a function with circuit breaker protection
func (cb *CircuitBreaker) Call(fn func() error) error {
	if err := cb.beforeRequest(); err != nil {
		return err
	}

	err := fn()

	cb.afterRequest(err)
	return err
}

func (cb *CircuitBreaker) beforeRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Check if we should transition from open to half-open
	if cb.state == StateOpen && time.Since(cb.lastFailTime) > cb.resetTimeout {
		cb.state = StateHalfOpen
		cb.successCount = 0
		cb.requestsInFly = 0
	}

	switch cb.state {
	case StateOpen:
		return ErrCircuitOpen
	case StateHalfOpen:
		if cb.requestsInFly >= cb.halfOpenRequests {
			return ErrTooManyRequests
		}
		cb.requestsInFly++
		return nil
	case StateClosed:
		return nil
	}

	return nil
}

func (cb *CircuitBreaker) afterRequest(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		cb.requestsInFly--
	}

	if err != nil {
		cb.onFailure()
	} else {
		cb.onSuccess()
	}
}

func (cb *CircuitBreaker) onSuccess() {
	cb.failures = 0

	if cb.state == StateHalfOpen {
		cb.successCount++
		if cb.successCount >= cb.halfOpenRequests {
			cb.state = StateClosed
		}
	}
}

func (cb *CircuitBreaker) onFailure() {
	cb.failures++
	cb.lastFailTime = timeNow()

	if cb.failures >= cb.maxFailures {
		cb.state = StateOpen
	}
}

// State returns the current circuit breaker state
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.failures = 0
	cb.successCount = 0
}

// RetryConfig defines retry behavior
type RetryConfig struct {
	MaxAttempts     int
	InitialBackoff  time.Duration
	MaxBackoff      time.Duration
	BackoffMultiple float64
}

// DefaultRetryConfig returns sensible defaults for retries
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:     3,
		InitialBackoff:  100 * time.Millisecond,
		MaxBackoff:      5 * time.Second,
		BackoffMultiple: 2.0,
	}
}

var ErrInvalidFormat = errors.New("invalid audio format")

// timeNow is a variable for testing
var timeNow = time.Now
