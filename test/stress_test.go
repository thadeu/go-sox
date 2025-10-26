package sox

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/thadeu/go-sox"
)

// TestParallelConversions tests multiple concurrent conversions
func TestParallelConversions(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skipf("SoX not installed: %v", err)
	}

	const numConversions = 50
	var wg sync.WaitGroup
	var successCount, failureCount atomic.Int32

	pcmData := generateTestPCM(16000, 1, 100)

	for i := 0; i < numConversions; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
			input := bytes.NewReader(pcmData)
			output := &bytes.Buffer{}

			err := converter.Convert(input, output)
			if err != nil {
				t.Logf("Conversion %d failed: %v", id, err)
				failureCount.Add(1)
			} else {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Completed %d conversions: %d successful, %d failed",
		numConversions, successCount.Load(), failureCount.Load())

	if failureCount.Load() > 0 {
		t.Errorf("Had %d failures out of %d conversions", failureCount.Load(), numConversions)
	}
}

// TestPooledConversions tests pool-based concurrency control
func TestPooledConversions(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skipf("SoX not installed: %v", err)
	}

	pool := NewPoolWithLimit(10) // Limit to 10 concurrent
	const numConversions = 100

	var wg sync.WaitGroup
	var successCount, failureCount atomic.Int32

	pcmData := generateTestPCM(16000, 1, 100)

	start := time.Now()

	for i := 0; i < numConversions; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO).
				WithPool(pool)
			input := bytes.NewReader(pcmData)
			output := &bytes.Buffer{}

			err := converter.ConvertWithContext(ctx, input, output)
			if err != nil {
				t.Logf("Conversion %d failed: %v", id, err)
				failureCount.Add(1)
			} else {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Completed %d conversions in %v: %d successful, %d failed",
		numConversions, elapsed, successCount.Load(), failureCount.Load())
	t.Logf("Average: %.2fms per conversion", float64(elapsed.Milliseconds())/float64(numConversions))

	if failureCount.Load() > 0 {
		t.Errorf("Had %d failures out of %d conversions", failureCount.Load(), numConversions)
	}
}

// TestResilientConversions tests conversion with retry and circuit breaker
func TestResilientConversions(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skipf("SoX not installed: %v", err)
	}

	const numConversions = 20
	var wg sync.WaitGroup
	var successCount, failureCount atomic.Int32

	pcmData := generateTestPCM(16000, 1, 100)

	for i := 0; i < numConversions; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
			input := bytes.NewReader(pcmData)
			output := &bytes.Buffer{}

			err := converter.ConvertWithContext(ctx, input, output)
			if err != nil {
				t.Logf("Conversion %d failed: %v", id, err)
				failureCount.Add(1)
			} else {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Completed %d conversions: %d successful, %d failed",
		numConversions, successCount.Load(), failureCount.Load())

	stats := GetMonitor().GetStats()
	t.Logf("Monitor stats: active=%d, total=%d, failed=%d, success_rate=%.2f%%",
		stats.ActiveProcesses, stats.TotalConversions, stats.FailedConversions, stats.SuccessRate)
}

// TestStreamParallel tests parallel stream conversions
func TestStreamParallel(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skipf("SoX not installed: %v", err)
	}

	const numStreams = 20
	var wg sync.WaitGroup
	var successCount, failureCount atomic.Int32

	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
			if err := stream.Start(); err != nil {
				t.Logf("Stream %d start failed: %v", id, err)
				failureCount.Add(1)
				return
			}

			// Write 10 chunks
			for j := 0; j < 10; j++ {
				pcmData := generateTestPCM(16000, 1, 20)
				if _, err := stream.Write(pcmData); err != nil {
					t.Logf("Stream %d write failed: %v", id, err)
					failureCount.Add(1)
					stream.Close()
					return
				}
			}

			if _, err := stream.Flush(); err != nil {
				t.Logf("Stream %d flush failed: %v", id, err)
				failureCount.Add(1)
			} else {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Completed %d streams: %d successful, %d failed",
		numStreams, successCount.Load(), failureCount.Load())

	if failureCount.Load() > 0 {
		t.Errorf("Had %d failures out of %d streams", failureCount.Load(), numStreams)
	}
}

// TestCircuitBreaker tests circuit breaker behavior
func TestCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker()

	// Should be closed initially
	if cb.State() != StateClosed {
		t.Errorf("Expected StateClosed, got %v", cb.State())
	}

	// Trigger failures to open circuit
	for i := 0; i < 5; i++ {
		cb.Call(func() error {
			return fmt.Errorf("simulated error")
		})
	}

	// Should be open now
	if cb.State() != StateOpen {
		t.Errorf("Expected StateOpen after failures, got %v", cb.State())
	}

	// Should reject calls when open
	err := cb.Call(func() error {
		return nil
	})

	if err != ErrCircuitOpen {
		t.Errorf("Expected ErrCircuitOpen, got %v", err)
	}
}

// TestResourceMonitor tests resource monitoring
func TestResourceMonitor(t *testing.T) {
	monitor := GetMonitor()
	monitor.Reset()

	// Simulate tracking processes
	monitor.TrackProcess(1000)
	monitor.TrackProcess(1001)

	if active := monitor.ActiveProcesses(); active != 2 {
		t.Errorf("Expected 2 active processes, got %d", active)
	}

	monitor.UntrackProcess(1000)

	if active := monitor.ActiveProcesses(); active != 1 {
		t.Errorf("Expected 1 active process after untrack, got %d", active)
	}

	stats := monitor.GetStats()
	if stats.TotalConversions != 2 {
		t.Errorf("Expected 2 total conversions, got %d", stats.TotalConversions)
	}
}

// TestPoolCapacity tests pool capacity limits
func TestPoolCapacity(t *testing.T) {
	pool := NewPoolWithLimit(5)

	ctx := context.Background()

	// Acquire 5 slots (should succeed)
	for i := 0; i < 5; i++ {
		if err := pool.Acquire(ctx); err != nil {
			t.Fatalf("Failed to acquire slot %d: %v", i, err)
		}
	}

	if pool.ActiveWorkers() != 5 {
		t.Errorf("Expected 5 active workers, got %d", pool.ActiveWorkers())
	}

	// 6th acquire should block
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := pool.Acquire(ctx)
	if err == nil {
		t.Error("Expected timeout error when pool full")
	}

	// Release one and try again
	pool.Release()

	ctx2 := context.Background()
	if err := pool.Acquire(ctx2); err != nil {
		t.Errorf("Failed to acquire after release: %v", err)
	}
}

// BenchmarkParallelConversions benchmarks parallel conversions
func BenchmarkParallelConversions(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skipf("SoX not installed: %v", err)
	}

	pcmData := generateTestPCM(16000, 1, 100)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
			input := bytes.NewReader(pcmData)
			output := &bytes.Buffer{}
			converter.Convert(input, output)
		}
	})
}

// BenchmarkPooledConversions benchmarks pooled conversions
func BenchmarkPooledConversions(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skipf("SoX not installed: %v", err)
	}

	pool := NewPoolWithLimit(50)
	pcmData := generateTestPCM(16000, 1, 100)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO).
				WithPool(pool)
			input := bytes.NewReader(pcmData)
			output := &bytes.Buffer{}
			converter.Convert(input, output)
		}
	})
}
