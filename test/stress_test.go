package sox

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	. "github.com/thadeu/go-sox"
)

// StressTestSuite defines the test suite for stress/performance tests
type StressTestSuite struct {
	suite.Suite
}

// SetupSuite runs once before all tests
func (s *StressTestSuite) SetupSuite() {
	err := CheckSoxInstalled("")
	if err != nil {
		s.T().Skipf("SoX not installed: %v", err)
	}
}

// generateTestPCM generates a simple PCM audio buffer
func (s *StressTestSuite) generateTestPCM(sampleRate, channels, durationMs int) []byte {
	numSamples := (sampleRate * durationMs) / 1000
	buffer := make([]byte, numSamples*channels*2) // 16-bit = 2 bytes per sample

	for i := 0; i < numSamples; i++ {
		value := int16(32767.0 * 0.5 * (1.0 + float64(i%100)/100.0))
		for ch := 0; ch < channels; ch++ {
			idx := (i*channels + ch) * 2
			buffer[idx] = byte(value & 0xFF)
			buffer[idx+1] = byte((value >> 8) & 0xFF)
		}
	}

	return buffer
}

// TestParallelConversions tests multiple concurrent conversions
func (s *StressTestSuite) TestParallelConversions() {
	const numConversions = 50
	var wg sync.WaitGroup
	var successCount, failureCount atomic.Int32

	pcmData := s.generateTestPCM(16000, 1, 100)

	for i := 0; i < numConversions; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
			input := bytes.NewReader(pcmData)
			output := &bytes.Buffer{}

			err := converter.Convert(input, output)
			if err != nil {
				s.T().Logf("Conversion %d failed: %v", id, err)
				failureCount.Add(1)
			} else {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	s.T().Logf("Completed %d conversions: %d successful, %d failed",
		numConversions, successCount.Load(), failureCount.Load())

	assert.Equal(s.T(), int32(0), failureCount.Load(),
		"Had %d failures out of %d conversions", failureCount.Load(), numConversions)
}

// TestResilientConversions tests resilient conversions with retry
func (s *StressTestSuite) TestResilientConversions() {
	const numConversions = 20
	var wg sync.WaitGroup
	var successCount, failureCount atomic.Int32

	pcmData := s.generateTestPCM(16000, 1, 100)

	for i := 0; i < numConversions; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)

			input := bytes.NewReader(pcmData)
			output := &bytes.Buffer{}

			err := converter.Convert(input, output)
			if err != nil {
				s.T().Logf("Conversion %d failed: %v", id, err)
				failureCount.Add(1)
			} else {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	s.T().Logf("Resilient conversions: %d successful, %d failed",
		successCount.Load(), failureCount.Load())

	assert.Greater(s.T(), successCount.Load(), int32(0), "Expected at least some successful conversions")
}

// TestStreamParallel tests parallel streaming conversions
func (s *StressTestSuite) TestStreamParallel() {
	const numStreams = 10
	var wg sync.WaitGroup
	var successCount, failureCount atomic.Int32

	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
			if err := stream.Start(); err != nil {
				s.T().Logf("Stream %d failed to start: %v", id, err)
				failureCount.Add(1)
				return
			}

			// Write multiple chunks
			for j := 0; j < 5; j++ {
				pcmData := s.generateTestPCM(16000, 1, 20)
				if _, err := stream.Write(pcmData); err != nil {
					s.T().Logf("Stream %d write failed: %v", id, err)
					failureCount.Add(1)
					return
				}
			}

			// Flush
			if _, err := stream.Flush(); err != nil {
				s.T().Logf("Stream %d flush failed: %v", id, err)
				failureCount.Add(1)
				return
			}

			successCount.Add(1)
		}(i)
	}

	wg.Wait()

	s.T().Logf("Parallel streams: %d successful, %d failed",
		successCount.Load(), failureCount.Load())

	assert.Equal(s.T(), int32(0), failureCount.Load(),
		"Had %d stream failures out of %d", failureCount.Load(), numStreams)
}

// TestCircuitBreaker tests circuit breaker functionality
func (s *StressTestSuite) TestCircuitBreaker() {
	cb := NewCircuitBreaker()

	// Test normal operation
	err := cb.Call(func() error {
		return nil
	})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), StateClosed, cb.State())

	// Test failures
	for i := 0; i < 5; i++ {
		cb.Call(func() error {
			return fmt.Errorf("simulated error")
		})
	}

	// Circuit should be open now
	assert.Equal(s.T(), StateOpen, cb.State())

	// Calls should fail immediately
	err = cb.Call(func() error {
		return nil
	})
	assert.Error(s.T(), err)
	assert.Equal(s.T(), ErrCircuitOpen, err)

	s.T().Log("Circuit breaker working correctly")
}

// TestResourceMonitor tests resource monitoring
func (s *StressTestSuite) TestResourceMonitor() {
	monitor := GetMonitor()

	// Track some processes
	monitor.TrackProcess(1234)
	monitor.TrackProcess(5678)

	activeCount := monitor.ActiveProcesses()
	assert.Equal(s.T(), 2, activeCount)

	// Untrack
	monitor.UntrackProcess(1234)
	activeCount = monitor.ActiveProcesses()
	assert.Equal(s.T(), 1, activeCount)

	// Record failure
	monitor.RecordFailure()
	failures := monitor.FailedConversions()
	assert.Equal(s.T(), int64(1), failures)

	s.T().Log("Resource monitor working correctly")
}

// TestPoolCapacity tests pool capacity management
func (s *StressTestSuite) TestPoolCapacity() {
	pool := NewPoolWithLimit(5)

	assert.Equal(s.T(), 5, pool.MaxWorkers())
	assert.Equal(s.T(), 0, pool.ActiveWorkers())
	assert.Equal(s.T(), 5, pool.AvailableSlots())

	// Acquire slots
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		err := pool.Acquire(ctx)
		require.NoError(s.T(), err)
	}

	assert.Equal(s.T(), 3, pool.ActiveWorkers())
	assert.Equal(s.T(), 2, pool.AvailableSlots())

	// Release slots
	for i := 0; i < 3; i++ {
		pool.Release()
	}

	assert.Equal(s.T(), 0, pool.ActiveWorkers())
	assert.Equal(s.T(), 5, pool.AvailableSlots())

	s.T().Log("Pool capacity management working correctly")
}

// TestStressSuite runs the stress test suite
func TestStressSuite(t *testing.T) {
	suite.Run(t, new(StressTestSuite))
}
