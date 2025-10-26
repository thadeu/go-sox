package sox

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
)

// Pool manages a pool of concurrent SoX conversions
// to prevent resource exhaustion under high load
type Pool struct {
	maxWorkers int
	semaphore  chan struct{}
	active     int
	mu         sync.Mutex
}

// NewPool creates a pool with maximum concurrent conversions
// Default: 500 workers (configurable via SOX_MAX_WORKERS env var)
func NewPool() *Pool {
	maxWorkers := 500 // default

	if envMax := os.Getenv("SOX_MAX_WORKERS"); envMax != "" {
		if parsed, err := strconv.Atoi(envMax); err == nil && parsed > 0 {
			maxWorkers = parsed
		}
	}

	return &Pool{
		maxWorkers: maxWorkers,
		semaphore:  make(chan struct{}, maxWorkers),
	}
}

// NewPoolWithLimit creates a pool with specific max workers
func NewPoolWithLimit(maxWorkers int) *Pool {
	if maxWorkers <= 0 {
		maxWorkers = 500
	}

	return &Pool{
		maxWorkers: maxWorkers,
		semaphore:  make(chan struct{}, maxWorkers),
	}
}

// Acquire blocks until a worker slot is available
func (p *Pool) Acquire(ctx context.Context) error {
	select {
	case p.semaphore <- struct{}{}:
		p.mu.Lock()
		p.active++
		p.mu.Unlock()
		return nil
	case <-ctx.Done():
		return fmt.Errorf("pool acquire cancelled: %w", ctx.Err())
	}
}

// Release frees a worker slot
func (p *Pool) Release() {
	p.mu.Lock()
	p.active--
	p.mu.Unlock()
	<-p.semaphore
}

// ActiveWorkers returns the number of active conversions
func (p *Pool) ActiveWorkers() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.active
}

// MaxWorkers returns the maximum concurrent conversions allowed
func (p *Pool) MaxWorkers() int {
	return p.maxWorkers
}

// AvailableSlots returns the number of available worker slots
func (p *Pool) AvailableSlots() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.maxWorkers - p.active
}
