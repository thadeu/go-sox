package sox

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	. "github.com/thadeu/go-sox"
)

// StreamerTestSuite defines the test suite for Streamer
type StreamerTestSuite struct {
	suite.Suite
	tmpDir string
}

// SetupSuite runs once before all tests
func (s *StreamerTestSuite) SetupSuite() {
	if err := CheckSoxInstalled(""); err != nil {
		s.T().Skip("SoX not installed")
	}
}

// SetupTest runs before each test
func (s *StreamerTestSuite) SetupTest() {
	s.tmpDir = s.T().TempDir()
}

// TestStreamerSuite runs the test suite
func TestStreamerSuite(t *testing.T) {
	suite.Run(t, new(StreamerTestSuite))
}

// generatePCMData generates test PCM audio data
func (s *StreamerTestSuite) generatePCMData(sampleRate, durationMs int) []byte {
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
func (s *StreamerTestSuite) TestBasicStreaming() {
	stream := NewStreamer(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE)
	stream.Start(1 * time.Second)

	// Write multiple chunks
	for i := 0; i < 10; i++ {
		chunk := s.generatePCMData(16000, 100) // 100ms chunks
		_, err := stream.Write(chunk)
		require.NoError(s.T(), err, "Failed to write chunk %d", i)
	}

	// Flush and get output
	err := stream.End()
	require.NoError(s.T(), err)
}

// TestWithOutputPath tests streaming with file output
func (s *StreamerTestSuite) TestWithOutputPath() {
	outputPath := filepath.Join(s.tmpDir, "test_stream.flac")

	stream := NewStreamer(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE).
		WithOutputPath(outputPath)

	stream.Start(1 * time.Second)

	// Write data
	for i := 0; i < 20; i++ {
		chunk := s.generatePCMData(16000, 50) // 50ms chunks
		_, err := stream.Write(chunk)
		require.NoError(s.T(), err)
	}

	// Flush to file
	err := stream.End()
	require.NoError(s.T(), err)

	// Verify file exists and has content
	fileInfo, err := os.Stat(outputPath)
	require.NoError(s.T(), err, "Output file not created")
	assert.Greater(s.T(), fileInfo.Size(), int64(0), "Output file is empty")

	s.T().Logf("Created file %s with size %d bytes", outputPath, fileInfo.Size())
}

// TestBufferAccumulation tests that buffer accumulates all data
func (s *StreamerTestSuite) TestBufferAccumulation() {
	outputPath := filepath.Join(s.tmpDir, "test_accumulation.flac")

	stream := NewStreamer(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE).
		WithOutputPath(outputPath)

	stream.Start(1 * time.Second)

	// Write data in multiple stages
	totalChunks := 30
	for i := 0; i < totalChunks; i++ {
		chunk := s.generatePCMData(16000, 100) // 100ms chunks
		_, err := stream.Write(chunk)
		require.NoError(s.T(), err)
	}

	// Flush all accumulated data
	err := stream.End()
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
func (s *StreamerTestSuite) TestMultipleFormats() {
	formats := []struct {
		name   string
		format AudioFormat
		ext    string
	}{
		{"FLAC", FLAC_16K_MONO_LE, ".flac"},
		{"WAV", WAV_16K_MONO, ".wav"},
		{"ULAW", ULAW_8K_MONO, ".ul"},
	}

	for _, tc := range formats {
		s.Run(tc.name, func() {
			outputPath := filepath.Join(s.tmpDir, "test_"+tc.name+tc.ext)

			stream := NewStreamer(PCM_RAW_8K_MONO, tc.format).
				WithOutputPath(outputPath)

			stream.Start(1 * time.Second)

			// Write data
			for i := 0; i < 10; i++ {
				chunk := s.generatePCMData(16000, 100)
				_, err := stream.Write(chunk)
				require.NoError(s.T(), err)
			}

			// Flush
			err := stream.End()
			require.NoError(s.T(), err)
		})
	}
}
