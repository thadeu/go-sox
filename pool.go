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

// PooledStreamConverter wraps a StreamConverter with pool-based concurrency control
type PooledStreamConverter struct {
	*StreamConverter
	pool     *Pool
	acquired bool
	mu       sync.Mutex
}

// NewPooledStreamConverter creates a stream converter that uses a worker pool
func NewPooledStreamConverter(input, output AudioFormat, pool *Pool) *PooledStreamConverter {
	return &PooledStreamConverter{
		StreamConverter: NewStreamConverter(input, output),
		pool:            pool,
	}
}

// Start acquires a worker slot and starts the stream
func (psc *PooledStreamConverter) Start(ctx context.Context) error {
	// Acquire worker slot
	if err := psc.pool.Acquire(ctx); err != nil {
		return fmt.Errorf("failed to acquire worker slot: %w", err)
	}

	psc.mu.Lock()
	psc.acquired = true
	psc.mu.Unlock()

	// Start stream
	if err := psc.StreamConverter.Start(); err != nil {
		psc.pool.Release()
		psc.mu.Lock()
		psc.acquired = false
		psc.mu.Unlock()
		return err
	}

	return nil
}

// Close closes the stream and releases the worker slot
func (psc *PooledStreamConverter) Close() error {
	err := psc.StreamConverter.Close()

	psc.mu.Lock()
	if psc.acquired {
		psc.pool.Release()
		psc.acquired = false
	}
	psc.mu.Unlock()

	return err
}

// Flush flushes the stream and releases the worker slot
func (psc *PooledStreamConverter) Flush() ([]byte, error) {
	data, err := psc.StreamConverter.Flush()

	psc.mu.Lock()
	if psc.acquired {
		psc.pool.Release()
		psc.acquired = false
	}
	psc.mu.Unlock()

	return data, err
}
