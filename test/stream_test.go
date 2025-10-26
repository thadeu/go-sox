package sox

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	. "github.com/thadeu/go-sox"
)

// StreamConverterTestSuite defines the test suite for StreamConverter
type StreamConverterTestSuite struct {
	suite.Suite
	tmpDir string
}

// SetupSuite runs once before all tests
func (s *StreamConverterTestSuite) SetupSuite() {
	if err := CheckSoxInstalled(""); err != nil {
		s.T().Skip("SoX not installed")
	}
}

// SetupTest runs before each test
func (s *StreamConverterTestSuite) SetupTest() {
	s.tmpDir = s.T().TempDir()
}

// generatePCMData generates test PCM audio data
func (s *StreamConverterTestSuite) generatePCMData(sampleRate, durationMs int) []byte {
	numSamples := (sampleRate * durationMs) / 1000
	buffer := make([]byte, numSamples*2) // mono, 16-bit
	for i := 0; i < numSamples; i++ {
		value := int16((i % 1000) * 32)
		buffer[i*2] = byte(value & 0xFF)
		buffer[i*2+1] = byte((value >> 8) & 0xFF)
	}
	return buffer
}

// TestBasicStreaming tests basic streaming conversion
func (s *StreamConverterTestSuite) TestBasicStreaming() {
	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
	require.NoError(s.T(), stream.Start())

	// Write multiple chunks
	for i := 0; i < 10; i++ {
		chunk := s.generatePCMData(16000, 100) // 100ms chunks
		_, err := stream.Write(chunk)
		require.NoError(s.T(), err, "Failed to write chunk %d", i)
	}

	// Flush and get output
	data, err := stream.Flush()
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), data, "Expected non-empty output")

	s.T().Logf("Converted 10 chunks to %d bytes FLAC", len(data))
}

// TestWithOutputPath tests streaming with file output
func (s *StreamConverterTestSuite) TestWithOutputPath() {
	outputPath := filepath.Join(s.tmpDir, "test_stream.flac")

	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO).
		WithOutputPath(outputPath)

	require.NoError(s.T(), stream.Start())

	// Write data
	for i := 0; i < 20; i++ {
		chunk := s.generatePCMData(16000, 50) // 50ms chunks
		_, err := stream.Write(chunk)
		require.NoError(s.T(), err)
	}

	// Flush to file
	_, err := stream.Flush()
	require.NoError(s.T(), err)

	// Verify file exists and has content
	fileInfo, err := os.Stat(outputPath)
	require.NoError(s.T(), err, "Output file not created")
	assert.Greater(s.T(), fileInfo.Size(), int64(0), "Output file is empty")

	s.T().Logf("Created file %s with size %d bytes", outputPath, fileInfo.Size())
}

// TestAutoFlush tests auto-flush functionality
func (s *StreamConverterTestSuite) TestAutoFlush() {
	outputPath := filepath.Join(s.tmpDir, "test_autoflush.flac")

	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO).
		WithOutputPath(outputPath).
		WithAutoFlush(1 * time.Second)

	require.NoError(s.T(), stream.Start())

	// Write data for 500ms
	for i := 0; i < 10; i++ {
		chunk := s.generatePCMData(16000, 50) // 50ms chunks
		_, err := stream.Write(chunk)
		require.NoError(s.T(), err)
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for auto-flush
	time.Sleep(1500 * time.Millisecond)

	// Verify file exists
	fileInfo, err := os.Stat(outputPath)
	require.NoError(s.T(), err, "Auto-flush did not create file")
	assert.Greater(s.T(), fileInfo.Size(), int64(0), "Auto-flushed file is empty")

	s.T().Logf("Auto-flush created file with size %d bytes", fileInfo.Size())
}

// TestBufferAccumulation tests that buffer accumulates all data
func (s *StreamConverterTestSuite) TestBufferAccumulation() {
	outputPath := filepath.Join(s.tmpDir, "test_accumulation.flac")

	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO).
		WithOutputPath(outputPath)

	require.NoError(s.T(), stream.Start())

	// Write data in multiple stages
	totalChunks := 30
	for i := 0; i < totalChunks; i++ {
		chunk := s.generatePCMData(16000, 100) // 100ms chunks
		_, err := stream.Write(chunk)
		require.NoError(s.T(), err)
	}

	// Flush all accumulated data
	_, err := stream.Flush()
	require.NoError(s.T(), err)

	// Verify file contains all data (should be ~3 seconds of audio)
	fileInfo, err := os.Stat(outputPath)
	require.NoError(s.T(), err)

	// FLAC should compress 3 seconds of 16K mono PCM (96KB) to much less
	expectedMinSize := int64(1000) // At least 1KB
	assert.GreaterOrEqual(s.T(), fileInfo.Size(), expectedMinSize,
		"Output file too small: got %d bytes, expected at least %d", fileInfo.Size(), expectedMinSize)

	s.T().Logf("Accumulated %d chunks (3s) into %d bytes FLAC", totalChunks, fileInfo.Size())
}

// TestMultipleFormats tests different output formats
func (s *StreamConverterTestSuite) TestMultipleFormats() {
	formats := []struct {
		name   string
		format AudioFormat
		ext    string
	}{
		{"FLAC", FLAC_16K_MONO, ".flac"},
		{"WAV", WAV_16K_MONO, ".wav"},
		{"ULAW", ULAW_8K_MONO, ".ul"},
	}

	for _, tc := range formats {
		s.Run(tc.name, func() {
			outputPath := filepath.Join(s.tmpDir, "test_"+tc.name+tc.ext)

			stream := NewStreamConverter(PCM_RAW_16K_MONO, tc.format).
				WithOutputPath(outputPath)

			require.NoError(s.T(), stream.Start(), "Failed to start stream for %s", tc.name)

			// Write data
			for i := 0; i < 10; i++ {
				chunk := s.generatePCMData(16000, 100)
				_, err := stream.Write(chunk)
				require.NoError(s.T(), err)
			}

			// Flush
			_, err := stream.Flush()
			require.NoError(s.T(), err, "Failed to flush %s", tc.name)

			// Verify file
			fileInfo, err := os.Stat(outputPath)
			require.NoError(s.T(), err, "File not created for %s", tc.name)
			assert.Greater(s.T(), fileInfo.Size(), int64(0), "File is empty for %s", tc.name)

			s.T().Logf("%s: %d bytes", tc.name, fileInfo.Size())
		})
	}
}

// TestWithPool tests streaming with pool
func (s *StreamConverterTestSuite) TestWithPool() {
	pool := NewPoolWithLimit(2) // Limit to 2 concurrent

	// Start 3 streams (3rd should wait for slot)
	streams := make([]*StreamConverter, 3)
	for i := 0; i < 3; i++ {
		outputPath := filepath.Join(s.tmpDir, "test_pool_"+string(rune('A'+i))+".flac")
		streams[i] = NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO).
			WithOutputPath(outputPath).
			WithPool(pool)
	}

	// Start first 2 (should succeed immediately)
	for i := 0; i < 2; i++ {
		require.NoError(s.T(), streams[i].Start(), "Failed to start stream %d", i)
	}

	// Try to start 3rd (should block or fail)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := streams[2].Start(ctx)
	assert.Error(s.T(), err, "Expected 3rd stream to fail/timeout due to pool limit")

	// Flush first stream to free slot
	for i := 0; i < 5; i++ {
		chunk := s.generatePCMData(16000, 100)
		streams[0].Write(chunk)
	}
	streams[0].Flush()

	// Now 3rd stream should be able to start
	require.NoError(s.T(), streams[2].Start(), "Failed to start 3rd stream after slot freed")

	// Cleanup
	for i := 1; i < 3; i++ {
		chunk := s.generatePCMData(16000, 100)
		streams[i].Write(chunk)
		streams[i].Flush()
	}

	s.T().Log("Pool management working correctly")
}

// TestAvailable tests Available() method
func (s *StreamConverterTestSuite) TestAvailable() {
	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
	require.NoError(s.T(), stream.Start())

	// Initially should be 0
	assert.Equal(s.T(), 0, stream.Available(), "Expected 0 available initially")

	// Write some data
	chunk := s.generatePCMData(16000, 500) // 500ms
	stream.Write(chunk)

	// Give SoX time to process
	time.Sleep(200 * time.Millisecond)

	// Should have some data available
	available := stream.Available()
	if available == 0 {
		s.T().Log("Warning: No data available yet (might be buffering)")
	} else {
		s.T().Logf("Available: %d bytes", available)
	}

	// Flush
	data, err := stream.Flush()
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), data, "Expected non-empty output")

	s.T().Logf("Final output: %d bytes", len(data))
}

// TestClose tests Close() method
func (s *StreamConverterTestSuite) TestClose() {
	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
	require.NoError(s.T(), stream.Start())

	// Write some data
	chunk := s.generatePCMData(16000, 100)
	stream.Write(chunk)

	// Close without flush
	require.NoError(s.T(), stream.Close())

	// Try to write after close (should fail)
	_, err := stream.Write(chunk)
	assert.Error(s.T(), err, "Expected write after close to fail")

	s.T().Log("Close() working correctly")
}

// TestStdoutMode tests streaming without output path
func (s *StreamConverterTestSuite) TestStdoutMode() {
	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
	// No WithOutputPath() - should use stdout mode

	require.NoError(s.T(), stream.Start())

	// Write data
	for i := 0; i < 10; i++ {
		chunk := s.generatePCMData(16000, 100)
		_, err := stream.Write(chunk)
		require.NoError(s.T(), err)
	}

	// Flush and get data
	data, err := stream.Flush()
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), data, "Expected non-empty output in stdout mode")

	// Verify it's valid FLAC data (starts with "fLaC")
	assert.True(s.T(), bytes.HasPrefix(data, []byte("fLaC")), "Output doesn't appear to be valid FLAC")

	s.T().Logf("Stdout mode: %d bytes FLAC", len(data))
}

// TestErrorHandling tests error scenarios
func (s *StreamConverterTestSuite) TestErrorHandling() {
	s.Run("WriteBeforeStart", func() {
		stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
		chunk := s.generatePCMData(16000, 100)
		_, err := stream.Write(chunk)
		assert.Error(s.T(), err, "Expected error when writing before start")
	})

	s.Run("FlushBeforeStart", func() {
		stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
		_, err := stream.Flush()
		assert.Error(s.T(), err, "Expected error when flushing before start")
	})

	s.Run("DoubleStart", func() {
		stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
		stream.Start()
		err := stream.Start()
		assert.Error(s.T(), err, "Expected error on double start")
		stream.Close()
	})

	s.Run("InvalidOutputPath", func() {
		stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO).
			WithOutputPath("/invalid/path/that/does/not/exist/file.flac")

		require.NoError(s.T(), stream.Start(), "Start should succeed")

		chunk := s.generatePCMData(16000, 100)
		stream.Write(chunk)

		// Flush should fail when trying to write to invalid path
		_, err := stream.Flush()
		assert.Error(s.T(), err, "Expected error when flushing to invalid path")
	})
}

// TestStreamConverterSuite runs the test suite
func TestStreamConverterSuite(t *testing.T) {
	suite.Run(t, new(StreamConverterTestSuite))
}
