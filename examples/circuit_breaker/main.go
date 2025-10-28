package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	sox "github.com/thadeu/go-sox"
)

// ProductionExample demonstrates production-ready usage with all fault tolerance features
func main() {
	if err := sox.CheckSoxInstalled(""); err != nil {
		log.Fatalf("SoX not installed: %v", err)
	}

	fmt.Println("Production Example: 100 Parallel RTP Conversions")
	fmt.Println("================================================\n")

	// 1. Create worker pool (limits concurrency)
	pool := sox.NewPool() // reads SOX_MAX_WORKERS env var (default: 500)
	fmt.Printf("Worker pool initialized: max %d workers\n", pool.MaxWorkers())

	// 2. Configure circuit breaker
	circuitBreaker := sox.NewCircuitBreakerWithConfig(
		10,             // maxFailures before opening
		60*time.Second, // resetTimeout
		5,              // halfOpenRequests
	)

	// 3. Configure retry behavior
	retryConfig := sox.RetryConfig{
		MaxAttempts:     3,
		InitialBackoff:  200 * time.Millisecond,
		MaxBackoff:      10 * time.Second,
		BackoffMultiple: 2.0,
	}

	// 4. Simulate 100 parallel SIP calls converting RTP to FLAC
	const numCalls = 100
	var wg sync.WaitGroup
	var successCount, failureCount atomic.Int32

	start := time.Now()

	for i := 0; i < numCalls; i++ {
		wg.Add(1)
		go func(callID int) {
			defer wg.Done()

			// Simulate RTP audio (1 second PCM)
			pcmData := generateRTPAudio(8000, 1, 1000)

			// Create converter with all protections (resilient by default)
			converter := sox.NewConverter(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE).
				WithCircuitBreaker(circuitBreaker).
				WithRetryConfig(retryConfig).
				WithPool(pool)

			// Set timeout
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Convert
			input := newSeekableReader(pcmData)
			output := newBufferWriter()

			err := converter.ConvertWithContext(ctx, input, output)
			if err != nil {
				log.Printf("Call %d failed: %v", callID, err)
				failureCount.Add(1)
			} else {
				successCount.Add(1)
			}
		}(i)

		// Stagger starts slightly
		time.Sleep(time.Millisecond)
	}

	// Wait for all conversions
	wg.Wait()
	elapsed := time.Since(start)

	// 5. Print results
	fmt.Println("\nConversion Results:")
	fmt.Println("------------------")
	fmt.Printf("Total calls: %d\n", numCalls)
	fmt.Printf("Successful: %d\n", successCount.Load())
	fmt.Printf("Failed: %d\n", failureCount.Load())
	fmt.Printf("Total time: %v\n", elapsed)
	fmt.Printf("Average: %.2fms per conversion\n", float64(elapsed.Milliseconds())/float64(numCalls))

	// 6. Print monitoring statistics
	monitor := sox.GetMonitor()
	stats := monitor.GetStats()

	fmt.Println("\nResource Monitor Stats:")
	fmt.Println("----------------------")
	fmt.Printf("Active processes: %d\n", stats.ActiveProcesses)
	fmt.Printf("Total conversions: %d\n", stats.TotalConversions)
	fmt.Printf("Failed conversions: %d\n", stats.FailedConversions)
	fmt.Printf("Success rate: %.2f%%\n", stats.SuccessRate)

	// 7. Print pool statistics
	fmt.Println("\nWorker Pool Stats:")
	fmt.Println("-----------------")
	fmt.Printf("Max workers: %d\n", pool.MaxWorkers())
	fmt.Printf("Active workers: %d\n", pool.ActiveWorkers())
	fmt.Printf("Available slots: %d\n", pool.AvailableSlots())

	// 8. Print circuit breaker state
	fmt.Println("\nCircuit Breaker:")
	fmt.Println("---------------")
	state := "Closed"
	switch circuitBreaker.State() {
	case sox.StateOpen:
		state = "Open (rejecting requests)"
	case sox.StateHalfOpen:
		state = "Half-Open (testing)"
	}
	fmt.Printf("State: %s\n", state)

	if failureCount.Load() > 0 {
		fmt.Printf("\nWARNING: %d conversions failed\n", failureCount.Load())
	} else {
		fmt.Println("\nSUCCESS: All conversions completed successfully!")
	}
}

// generateRTPAudio simulates RTP audio data
func generateRTPAudio(sampleRate, channels, durationMs int) []byte {
	numSamples := (sampleRate * durationMs) / 1000
	buffer := make([]byte, numSamples*channels*2)

	for i := 0; i < numSamples; i++ {
		value := int16((i % 1000) * 32)
		for ch := 0; ch < channels; ch++ {
			idx := (i*channels + ch) * 2
			buffer[idx] = byte(value & 0xFF)
			buffer[idx+1] = byte((value >> 8) & 0xFF)
		}
	}

	return buffer
}

// seekableReader wraps a byte slice to implement io.ReadSeeker
type seekableReader struct {
	data []byte
	pos  int
}

func newSeekableReader(data []byte) *seekableReader {
	return &seekableReader{data: data}
}

func (r *seekableReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, nil
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (r *seekableReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case 0: // io.SeekStart
		r.pos = int(offset)
	case 1: // io.SeekCurrent
		r.pos += int(offset)
	case 2: // io.SeekEnd
		r.pos = len(r.data) + int(offset)
	}
	return int64(r.pos), nil
}

// bufferWriter wraps a byte slice to implement io.Writer
type bufferWriter struct {
	data []byte
}

func newBufferWriter() *bufferWriter {
	return &bufferWriter{}
}

func (w *bufferWriter) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *bufferWriter) Len() int {
	return len(w.data)
}
