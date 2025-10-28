package sox

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// SoxTestSuite defines the test suite for Converter
type SoxTestSuite struct {
	suite.Suite
	tmpDir string
}

// SetupSuite runs once before all tests
func (s *SoxTestSuite) SetupSuite() {
	err := CheckSoxInstalled("")
	if err != nil {
		s.T().Skipf("SoX not installed, skipping tests: %v", err)
	}
}

// SetupTest runs before each test
func (s *SoxTestSuite) SetupTest() {
	s.tmpDir = s.T().TempDir()
}

// TestConverterSuite runs the converter test suite
func TestConverterSuite(t *testing.T) {
	suite.Run(t, new(SoxTestSuite))
}

// TestPresets tests format presets
func (s *SoxTestSuite) TestPresets() {
	presets := []AudioFormat{
		PCM_RAW_8K_MONO,
		WAV_16K_MONO,
		ULAW_8K_MONO,
	}

	for _, preset := range presets {
		s.Run(preset.Type, func() {
			err := preset.Validate()
			assert.NoError(s.T(), err, "Preset %s should be valid", preset.Type)
		})
	}
}

// generatePCMData generates test PCM audio data
func (s *SoxTestSuite) generatePCMData(sampleRate, durationMs int) []byte {
	numSamples := (sampleRate * durationMs) / 1000
	buffer := make([]byte, numSamples*2) // mono, 16-bit
	for i := 0; i < numSamples; i++ {
		value := int16((i % 1000) * 32)
		buffer[i*2] = byte(value & 0xFF)
		buffer[i*2+1] = byte((value >> 8) & 0xFF)
	}
	return buffer
}

// TEST SUITE 1: Simple Conversion (Default Mode)
// ═══════════════════════════════════════════════════════════

// TestSimpleConvert_BytesToBytes tests simple bytes-to-bytes conversion
func (s *SoxTestSuite) TestSimpleConvert_BytesToBytes() {
	pcmData := s.generatePCMData(8000, 1000) // 1 second

	conv := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE)
	inputReader := bytes.NewReader(pcmData)
	outputBuffer := &bytes.Buffer{}

	err := conv.Convert(inputReader, outputBuffer)
	require.NoError(s.T(), err)
	assert.Greater(s.T(), outputBuffer.Len(), 0, "Output should not be empty")
}

// TestSimpleConvert_FilesToFiles tests file-to-file conversion
func (s *SoxTestSuite) TestSimpleConvert_FilesToFiles() {
	inputPath := filepath.Join(s.tmpDir, "input.pcm")
	outputPath := filepath.Join(s.tmpDir, "output.flac")

	// Create input file
	pcmData := s.generatePCMData(8000, 1000)
	err := os.WriteFile(inputPath, pcmData, 0644)
	require.NoError(s.T(), err)

	// Convert
	conv := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE)
	err = conv.Convert(inputPath, outputPath)
	require.NoError(s.T(), err)

	// Verify output file
	info, err := os.Stat(outputPath)
	require.NoError(s.T(), err)
	assert.Greater(s.T(), info.Size(), int64(0))
}

// TestSimpleConvert_MixedIO tests mixed io.Reader and file path arguments
func (s *SoxTestSuite) TestSimpleConvert_MixedIO() {
	inputPath := filepath.Join(s.tmpDir, "input.pcm")
	outputPath := filepath.Join(s.tmpDir, "output.flac")

	// Create input file
	pcmData := s.generatePCMData(8000, 1000)
	err := os.WriteFile(inputPath, pcmData, 0644)
	require.NoError(s.T(), err)

	// Convert: file to file path string
	file, err := os.Open(inputPath)
	require.NoError(s.T(), err)
	defer file.Close()

	conv := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE)
	err = conv.Convert(file, outputPath)
	require.NoError(s.T(), err)

	info, err := os.Stat(outputPath)
	require.NoError(s.T(), err)
	assert.Greater(s.T(), info.Size(), int64(0))
}

// TestSimpleConvert_WithContext tests context cancellation
func (s *SoxTestSuite) TestSimpleConvert_WithContext() {
	pcmData := s.generatePCMData(8000, 1000)

	conv := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE)
	inputReader := bytes.NewReader(pcmData)
	outputBuffer := &bytes.Buffer{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := conv.ConvertWithContext(ctx, inputReader, outputBuffer)
	require.NoError(s.T(), err)
	assert.Greater(s.T(), outputBuffer.Len(), 0)
}

// TEST SUITE 2: Ticker Mode (Periodic Batch Processing)
// ═══════════════════════════════════════════════════════════

// TestTicker_BasicOperation tests basic ticker functionality
func (s *SoxTestSuite) TestTicker_BasicOperation() {
	conv := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE).
		WithTicker(1 * time.Second).
		WithStart()

	// Write multiple chunks
	for i := 0; i < 10; i++ {
		chunk := s.generatePCMData(16000, 100) // 100ms chunks
		_, err := conv.Write(chunk)
		require.NoError(s.T(), err, "Failed to write chunk %d", i)
	}

	// Flush and get output
	err := conv.Stop()
	require.NoError(s.T(), err)
}

// TestTicker_WithOutputPath tests ticker mode writing to file
func (s *SoxTestSuite) TestTicker_WithOutputPath() {
	outputPath := filepath.Join(s.tmpDir, "ticker_output.flac")

	conv := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE).
		WithOutputPath(outputPath).
		WithTicker(1 * time.Second)

	err := conv.Start()
	require.NoError(s.T(), err)

	// Write data
	for i := 0; i < 20; i++ {
		chunk := s.generatePCMData(8000, 50) // 50ms chunks
		_, err := conv.Write(chunk)
		require.NoError(s.T(), err)
	}

	// Flush to file
	err = conv.Stop()
	require.NoError(s.T(), err)

	// Verify file exists and has content
	fileInfo, err := os.Stat(outputPath)
	require.NoError(s.T(), err, "Output file not created")
	assert.Greater(s.T(), fileInfo.Size(), int64(0), "Output file is empty")

	s.T().Logf("Ticker mode created file %s with size %d bytes", outputPath, fileInfo.Size())
}

// TestTicker_BufferAccumulation tests that ticker accumulates all data
func (s *SoxTestSuite) TestTicker_BufferAccumulation() {
	outputPath := filepath.Join(s.tmpDir, "ticker_accumulation.flac")

	conv := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE).
		WithOutputPath(outputPath).
		WithTicker(1 * time.Second)

	err := conv.Start()
	require.NoError(s.T(), err)

	// Write data in multiple stages
	totalChunks := 30
	for i := 0; i < totalChunks; i++ {
		chunk := s.generatePCMData(16000, 100) // 100ms chunks
		_, err := conv.Write(chunk)
		require.NoError(s.T(), err)
	}

	// Flush all accumulated data
	err = conv.Stop()
	require.NoError(s.T(), err)

	// Verify file contains all data (should be ~3 seconds of audio)
	fileInfo, err := os.Stat(outputPath)
	require.NoError(s.T(), err)

	// FLAC should compress 3 seconds of 16K mono PCM (96KB) to much less
	expectedMinSize := int64(1000) // At least 1KB
	assert.GreaterOrEqual(s.T(), fileInfo.Size(), expectedMinSize,
		"Output file too small: got %d bytes, expected at least %d", fileInfo.Size(), expectedMinSize)

	s.T().Logf("Ticker accumulated %d chunks (3s) into %d bytes FLAC", totalChunks, fileInfo.Size())
}

// TestTicker_MultipleFormats tests different output formats in ticker mode
func (s *SoxTestSuite) TestTicker_MultipleFormats() {
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

			conv := New(PCM_RAW_8K_MONO, tc.format).
				WithOutputPath(outputPath).
				WithTicker(1 * time.Second)

			err := conv.Start()
			require.NoError(s.T(), err)

			// Write data
			for i := 0; i < 10; i++ {
				chunk := s.generatePCMData(16000, 100)
				_, err := conv.Write(chunk)
				require.NoError(s.T(), err)
			}

			// Flush
			err = conv.Stop()
			require.NoError(s.T(), err)

			// Verify file was created
			info, err := os.Stat(outputPath)
			require.NoError(s.T(), err)
			assert.Greater(s.T(), info.Size(), int64(0))
		})
	}
}

// TEST SUITE 3: Stream Mode (Real-time Streaming)
// ═══════════════════════════════════════════════════════════

// TestStream_Basic tests basic streaming mode
func (s *SoxTestSuite) TestStream_Basic() {
	conv := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE).
		WithStream()

	err := conv.Start()
	require.NoError(s.T(), err)

	// Write some data
	chunk := s.generatePCMData(8000, 100)
	_, err = conv.Write(chunk)
	require.NoError(s.T(), err)

	// Stop streaming
	err = conv.Stop()
	require.NoError(s.T(), err)
}

// TestStream_WriteBeforeStart verifies error handling
func (s *SoxTestSuite) TestStream_WriteBeforeStart() {
	conv := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE).
		WithStream()

	chunk := s.generatePCMData(8000, 100)
	_, err := conv.Write(chunk)
	require.Error(s.T(), err, "Should fail to write before Start()")
}

// TestStream_ReadBeforeStart verifies error handling
func (s *SoxTestSuite) TestStream_ReadBeforeStart() {
	conv := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE).
		WithStream()

	buf := make([]byte, 4096)
	_, err := conv.Read(buf)
	require.Error(s.T(), err, "Should fail to read before Start()")
}

// TEST SUITE 4: Backward Compatibility
// ═══════════════════════════════════════════════════════════

// TestBackwardCompat_New verifies New() still works
func (s *SoxTestSuite) TestBackwardCompat_NewConverter() {
	pcmData := s.generatePCMData(8000, 1000)

	// Old API: New()
	conv := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE)
	inputReader := bytes.NewReader(pcmData)
	outputBuffer := &bytes.Buffer{}

	err := conv.Convert(inputReader, outputBuffer)
	require.NoError(s.T(), err)
	assert.Greater(s.T(), outputBuffer.Len(), 0)
}

// TestBackwardCompat_New verifies New() still works
func (s *SoxTestSuite) TestBackwardCompat_NewTicker() {
	// Old API: New() returns Converter
	conv := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE)
	require.NotNil(s.T(), conv)

	// Should be able to use it as new API
	conv.WithTicker(1 * time.Second)
	err := conv.Start()
	require.NoError(s.T(), err)

	chunk := s.generatePCMData(8000, 100)
	_, err = conv.Write(chunk)
	require.NoError(s.T(), err)

	err = conv.Stop()
	require.NoError(s.T(), err)
}

// TEST SUITE 5: Command Arguments Verification
// ═══════════════════════════════════════════════════════════

// TestCommandArgs_PathMode verifies that path mode uses direct file access (no piping)
func (s *SoxTestSuite) TestCommandArgs_PathMode() {
	inputPath := filepath.Join(s.tmpDir, "input.pcm")
	outputPath := filepath.Join(s.tmpDir, "output.flac")

	// Create input file
	pcmData := s.generatePCMData(8000, 1000)
	err := os.WriteFile(inputPath, pcmData, 0644)
	require.NoError(s.T(), err)

	// Create converter in path mode
	conv := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE)
	conv.pathMode = true
	conv.inputPath = inputPath
	conv.outputPath = outputPath

	// Build command arguments
	args := conv.buildCommandArgs()

	// Verify path mode arguments: sox input.pcm output.flac (no "-" pipes)
	s.T().Logf("Path mode args: %v", args)

	// Should have input file path in arguments
	assert.Contains(s.T(), args, inputPath, "Input path should be in arguments")

	// Should have output file path in arguments
	assert.Contains(s.T(), args, outputPath, "Output path should be in arguments")

	// Should NOT have stdin/stdout pipes for path mode
	// Count occurrences of "-" (should only be in format specifications, not I/O)
	pipesCount := 0
	for _, arg := range args {
		if arg == "-" {
			pipesCount++
		}
	}
	// Path mode should have 0 "-" pipes for I/O
	assert.Equal(s.T(), 0, pipesCount, "Path mode should not have '-' pipes for I/O")

	// Verify order: format args, input file, format args, output file
	inputIdx := -1
	outputIdx := -1
	for i, arg := range args {
		if arg == inputPath {
			inputIdx = i
		}
		if arg == outputPath {
			outputIdx = i
		}
	}
	assert.Greater(s.T(), inputIdx, 0, "Input path should be after format args")
	assert.Greater(s.T(), outputIdx, inputIdx, "Output path should be after input path")
}

// TestCommandArgs_StreamMode verifies that stream mode uses pipes for I/O (- -)
func (s *SoxTestSuite) TestCommandArgs_StreamMode() {
	// Create converter in stream mode (NOT path mode)
	conv := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE)
	conv.pathMode = false
	conv.streamMode = true

	// Build command arguments
	args := conv.buildCommandArgs()

	// Verify stream mode arguments: sox ... - ... - (with pipes)
	s.T().Logf("Stream mode args: %v", args)

	// Should have stdin and stdout pipes
	assert.Contains(s.T(), args, "-", "Stream mode should have '-' pipes")

	// Count pipes - should be exactly 2 (stdin and stdout)
	pipesCount := 0
	for _, arg := range args {
		if arg == "-" {
			pipesCount++
		}
	}
	assert.Equal(s.T(), 2, pipesCount, "Stream mode should have exactly 2 '-' pipes (stdin and stdout)")

	// Should NOT have file paths for stream mode
	assert.NotContains(s.T(), args, "/", "Stream mode should not have file paths")
}

// TestCommandArgs_TickerMode verifies ticker mode uses stdin pipe and output file (- path)
func (s *SoxTestSuite) TestCommandArgs_TickerMode() {
	outputPath := filepath.Join(s.tmpDir, "ticker_output.flac")

	// Create converter in ticker mode
	conv := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE)
	conv.pathMode = false
	conv.tickerMode = true
	conv.outputPath = outputPath

	// Build command arguments
	args := conv.buildCommandArgs()

	// Verify ticker mode arguments: sox ... - ... output.flac (stdin pipe + output file)
	s.T().Logf("Ticker mode args: %v", args)

	// Should have stdin pipe
	assert.Contains(s.T(), args, "-", "Ticker mode should have stdin pipe ('-')")

	// Should have output file path
	assert.Contains(s.T(), args, outputPath, "Ticker mode should have output file path")

	// Count pipes - should be exactly 1 (only stdin)
	pipesCount := 0
	for _, arg := range args {
		if arg == "-" {
			pipesCount++
		}
	}
	assert.Equal(s.T(), 1, pipesCount, "Ticker mode should have exactly 1 '-' pipe (stdin)")

	// Verify order: format args, stdin pipe (-), format args, output path
	stdinIdx := -1
	outputIdx := -1
	for i, arg := range args {
		if arg == "-" && stdinIdx == -1 {
			stdinIdx = i
		}
		if arg == outputPath {
			outputIdx = i
		}
	}
	assert.Greater(s.T(), stdinIdx, 0, "Stdin pipe should be after format args")
	assert.Greater(s.T(), outputIdx, stdinIdx, "Output path should be after stdin pipe")
}

// TestCommandArgs_TickerModeRealWorld verifies real-world use case (mu-law to FLAC)
// This matches the user's actual use case in their RTP recorder
func (s *SoxTestSuite) TestCommandArgs_TickerModeRealWorld() {
	outputPath := filepath.Join(s.tmpDir, "rtp_output.flac")

	// Create converter with formats matching user's real-world use case
	// Input: mu-law (G.711), Output: FLAC
	input := AudioFormat{
		Type:         "raw",
		Encoding:     "mu-law",
		SampleRate:   8000,
		Channels:     1,
		BitDepth:     8,
		IgnoreLength: true,
	}

	output := AudioFormat{
		Type:       "flac",
		SampleRate: 16000,
		Channels:   1,
		BitDepth:   16,
	}

	// Add options for compression and comment
	opts := DefaultOptions()
	opts.CompressionLevel = 0
	opts.CustomGlobalArgs = []string{"--add-comment", "PAPI rtp-recorder"}

	// Create converter in ticker mode
	conv := New(input, output)
	conv.Options = opts
	conv.pathMode = false
	conv.tickerMode = true
	conv.outputPath = outputPath

	// Build command arguments
	args := conv.buildCommandArgs()

	s.T().Logf("Real-world ticker mode command: sox %s", strings.Join(args, " "))

	// Verify key arguments are present
	assert.Contains(s.T(), args, "raw", "Should have 'raw' input type")
	assert.Contains(s.T(), args, "mu-law", "Should have 'mu-law' encoding")
	assert.Contains(s.T(), args, "8000", "Should have 8000 Hz input rate")
	assert.Contains(s.T(), args, "1", "Should have 1 channel")

	assert.Contains(s.T(), args, "flac", "Should have 'flac' output type")
	assert.Contains(s.T(), args, "16000", "Should have 16000 Hz output rate")
	assert.Contains(s.T(), args, "-C", "Should have compression level flag")
	assert.Contains(s.T(), args, "0", "Should have compression level 0")

	assert.Contains(s.T(), args, "--add-comment", "Should have add-comment flag")
	assert.Contains(s.T(), args, "PAPI rtp-recorder", "Should have comment text")

	// Verify structure: should be stdin pipe (-) then output file
	pipesCount := 0
	hasStdin := false
	hasOutputPath := false
	stdinBeforeOutput := false

	for i, arg := range args {
		if arg == "-" && !hasStdin {
			hasStdin = true
			pipesCount++
			// Check if output path comes after stdin
			for j := i + 1; j < len(args); j++ {
				if args[j] == outputPath {
					stdinBeforeOutput = true
					break
				}
			}
		}
		if arg == outputPath {
			hasOutputPath = true
		}
	}

	assert.True(s.T(), hasStdin, "Should have stdin pipe")
	assert.True(s.T(), hasOutputPath, "Should have output path")
	assert.True(s.T(), stdinBeforeOutput, "Stdin pipe should come before output path")

	// Verify this would work with cmd.Stdin = bytes.NewReader(data)
	// This command structure matches:
	// sox -t raw -r 8000 -b 8 -c 1 -e mu-law --ignore-length - -t flac -r 16000 -b 16 -c 1 -C 0 --add-comment "PAPI rtp-recorder" output.flac
	s.T().Logf("✓ Command structure verified for user's RTP recorder use case")
}

// BENCHMARK TESTS
// ═══════════════════════════════════════════════════════════

// BenchmarkConverter_Convert benchmarks basic conversion
func BenchmarkConverter_Convert(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skip("SoX not installed")
	}

	// Generate test data once
	pcmData := generateBenchmarkPCM(8000, 1, 1000) // 1 second of audio
	converter := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := bytes.NewReader(pcmData)
		output := &bytes.Buffer{}

		if err := converter.Convert(input, output); err != nil {
			b.Fatalf("Conversion failed: %v", err)
		}
	}
}

// BenchmarkConverter_ConvertSmall benchmarks small audio conversion (100ms)
func BenchmarkConverter_ConvertSmall(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skip("SoX not installed")
	}

	pcmData := generateBenchmarkPCM(8000, 1, 100) // 100ms
	converter := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := bytes.NewReader(pcmData)
		output := &bytes.Buffer{}
		converter.Convert(input, output)
	}
}

// BenchmarkConverter_ConvertLarge benchmarks large audio conversion (5 seconds)
func BenchmarkConverter_ConvertLarge(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skip("SoX not installed")
	}

	pcmData := generateBenchmarkPCM(8000, 1, 5000) // 5 seconds
	converter := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := bytes.NewReader(pcmData)
		output := &bytes.Buffer{}
		converter.Convert(input, output)
	}
}

// BenchmarkTicker benchmarks ticker mode conversion
func BenchmarkTicker(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skip("SoX not installed")
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		conv := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE).
			WithTicker(1 * time.Second)
		conv.Start()

		for j := 0; j < 10; j++ {
			chunk := generateBenchmarkPCM(8000, 1, 100)
			conv.Write(chunk)
		}

		conv.Stop()
	}
}

// BenchmarkTicker_MultipleWrites benchmarks ticker with multiple writes
func BenchmarkTicker_MultipleWrites(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skip("SoX not installed")
	}

	chunk := generateBenchmarkPCM(16000, 1, 20) // 20ms chunks

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		conv := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE).
			WithTicker(1 * time.Second)
		conv.Start()

		// Write 50 chunks (1 second total)
		for j := 0; j < 50; j++ {
			conv.Write(chunk)
		}

		conv.Stop()
	}
}

// BenchmarkConverter_PCMToFLAC benchmarks PCM to FLAC conversion
func BenchmarkConverter_PCMToFLAC(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skip("SoX not installed")
	}

	pcmData := generateBenchmarkPCM(8000, 1, 1000)
	converter := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := bytes.NewReader(pcmData)
		output := &bytes.Buffer{}
		converter.Convert(input, output)
	}
}

// BenchmarkConverter_PCMToWAV benchmarks PCM to WAV conversion
func BenchmarkConverter_PCMToWAV(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skip("SoX not installed")
	}

	pcmData := generateBenchmarkPCM(8000, 1, 1000)
	converter := New(PCM_RAW_8K_MONO, WAV_8K_MONO_LE)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := bytes.NewReader(pcmData)
		output := &bytes.Buffer{}
		converter.Convert(input, output)
	}
}

// BenchmarkConverter_PCMToULAW benchmarks PCM to ULAW conversion
func BenchmarkConverter_PCMToULAW(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skip("SoX not installed")
	}

	pcmData := generateBenchmarkPCM(8000, 1, 1000)
	converter := New(PCM_RAW_8K_MONO, ULAW_8K_MONO)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := bytes.NewReader(pcmData)
		output := &bytes.Buffer{}
		converter.Convert(input, output)
	}
}

// BenchmarkConverter_Parallel benchmarks parallel conversions
func BenchmarkConverter_Parallel(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skip("SoX not installed")
	}

	pcmData := generateBenchmarkPCM(8000, 1, 100) // Smaller for parallel

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			converter := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE)
			input := bytes.NewReader(pcmData)
			output := &bytes.Buffer{}
			converter.Convert(input, output)
		}
	})
}

// generateBenchmarkPCM generates PCM data for benchmarks
func generateBenchmarkPCM(sampleRate, channels, durationMs int) []byte {
	numSamples := (sampleRate * durationMs) / 1000
	buffer := make([]byte, numSamples*channels*2)

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
