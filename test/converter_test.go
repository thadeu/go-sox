package sox

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	. "github.com/thadeu/go-sox"
)

// ConverterTestSuite defines the test suite for Converter
type ConverterTestSuite struct {
	suite.Suite
	tmpDir string
}

// SetupSuite runs once before all tests
func (s *ConverterTestSuite) SetupSuite() {
	err := CheckSoxInstalled("")
	if err != nil {
		s.T().Skipf("SoX not installed, skipping tests: %v", err)
	}
}

// SetupTest runs before each test
func (s *ConverterTestSuite) SetupTest() {
	s.tmpDir = s.T().TempDir()
}

// generateTestPCM generates a simple PCM audio buffer
func (s *ConverterTestSuite) generateTestPCM(sampleRate, channels, durationMs int) []byte {
	numSamples := (sampleRate * durationMs) / 1000
	buffer := make([]byte, numSamples*channels*2) // 16-bit = 2 bytes per sample

	// Generate a simple pattern for testing
	for i := 0; i < numSamples; i++ {
		// Simple ramp pattern
		value := int16(32767.0 * 0.5 * (1.0 + float64(i%100)/100.0))

		// Write value for each channel
		for ch := 0; ch < channels; ch++ {
			idx := (i*channels + ch) * 2
			buffer[idx] = byte(value & 0xFF)
			buffer[idx+1] = byte((value >> 8) & 0xFF)
		}
	}

	return buffer
}

// TestAudioFormatValidate tests format validation
func (s *ConverterTestSuite) TestAudioFormatValidate() {
	tests := []struct {
		name    string
		format  AudioFormat
		wantErr bool
	}{
		{
			name:    "valid raw format",
			format:  PCM_RAW_16K_MONO,
			wantErr: false,
		},
		{
			name: "invalid raw format - no encoding and no CustomArgs",
			format: AudioFormat{
				Type:       "raw",
				SampleRate: 16000,
				Channels:   1,
			},
			wantErr: true,
		},
		{
			name: "valid raw format with CustomArgs",
			format: AudioFormat{
				Type:       "raw",
				SampleRate: 16000,
				Channels:   1,
				CustomArgs: []string{"-e", "signed-integer", "-b", "16"},
			},
			wantErr: false,
		},
		{
			name:    "valid flac format",
			format:  FLAC_16K_MONO,
			wantErr: false,
		},
		{
			name: "invalid endian value",
			format: AudioFormat{
				Type:   "flac",
				Endian: "invalid",
			},
			wantErr: true,
		},
		{
			name: "valid endian value - little",
			format: AudioFormat{
				Type:   "flac",
				Endian: "little",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := tt.format.Validate()
			if tt.wantErr {
				assert.Error(s.T(), err)
			} else {
				assert.NoError(s.T(), err)
			}
		})
	}
}

// TestConverterConvert tests basic conversion
func (s *ConverterTestSuite) TestConverterConvert() {
	pcmData := s.generateTestPCM(16000, 1, 100) // 100ms of audio

	converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)

	input := bytes.NewReader(pcmData)
	output := &bytes.Buffer{}

	err := converter.Convert(input, output)
	require.NoError(s.T(), err)

	assert.Greater(s.T(), output.Len(), 0, "Expected non-empty output")
	s.T().Logf("Converted %d bytes PCM to %d bytes FLAC", len(pcmData), output.Len())
}

// TestConverterConvertFile tests file-based conversion
func (s *ConverterTestSuite) TestConverterConvertFile() {
	// Create input file
	inputPath := s.tmpDir + "/input.raw"
	outputPath := s.tmpDir + "/output.flac"

	pcmData := s.generateTestPCM(16000, 1, 100)
	err := os.WriteFile(inputPath, pcmData, 0644)
	require.NoError(s.T(), err)

	// Convert
	converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
	err = converter.ConvertFile(inputPath, outputPath)
	require.NoError(s.T(), err)

	// Verify output file exists
	fileInfo, err := os.Stat(outputPath)
	require.NoError(s.T(), err)
	assert.Greater(s.T(), fileInfo.Size(), int64(0))

	s.T().Logf("Created output file: %d bytes", fileInfo.Size())
}

// TestStreamConverter tests streaming conversion
func (s *ConverterTestSuite) TestStreamConverter() {
	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
	require.NoError(s.T(), stream.Start())

	// Write some data
	pcmData := s.generateTestPCM(16000, 1, 100)
	_, err := stream.Write(pcmData)
	require.NoError(s.T(), err)

	// Flush
	output, err := stream.Flush()
	require.NoError(s.T(), err)
	assert.Greater(s.T(), len(output), 0)

	s.T().Logf("Stream output: %d bytes", len(output))
}

// TestStreamConverterMultipleWrites tests multiple writes
func (s *ConverterTestSuite) TestStreamConverterMultipleWrites() {
	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
	require.NoError(s.T(), stream.Start())

	// Write multiple chunks
	for i := 0; i < 5; i++ {
		pcmData := s.generateTestPCM(16000, 1, 20) // 20ms chunks
		_, err := stream.Write(pcmData)
		require.NoError(s.T(), err)
	}

	// Flush
	output, err := stream.Flush()
	require.NoError(s.T(), err)
	assert.Greater(s.T(), len(output), 0)

	s.T().Logf("Multiple writes output: %d bytes", len(output))
}

// TestPresets tests format presets
func (s *ConverterTestSuite) TestPresets() {
	presets := []AudioFormat{
		PCM_RAW_8K_MONO,
		PCM_RAW_16K_MONO,
		PCM_RAW_48K_MONO,
		FLAC_16K_MONO,
		FLAC_44K_STEREO,
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

// TestConverterSuite runs the converter test suite
func TestConverterSuite(t *testing.T) {
	suite.Run(t, new(ConverterTestSuite))
}

// Standalone test for SoX installation check
func TestCheckSoxInstalled(t *testing.T) {
	err := CheckSoxInstalled("")
	if err != nil {
		t.Skipf("SoX not installed, skipping tests: %v", err)
	}
}

// Benchmarks

// BenchmarkConverter_Convert benchmarks basic conversion
func BenchmarkConverter_Convert(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skip("SoX not installed")
	}

	// Generate test data once
	pcmData := generateBenchmarkPCM(16000, 1, 1000) // 1 second of audio
	converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)

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

	pcmData := generateBenchmarkPCM(16000, 1, 100) // 100ms
	converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)

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

	pcmData := generateBenchmarkPCM(16000, 1, 5000) // 5 seconds
	converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := bytes.NewReader(pcmData)
		output := &bytes.Buffer{}
		converter.Convert(input, output)
	}
}

// BenchmarkStreamConverter benchmarks streaming conversion
func BenchmarkStreamConverter(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skip("SoX not installed")
	}

	pcmData := generateBenchmarkPCM(16000, 1, 1000) // 1 second

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
		stream.Start()
		stream.Write(pcmData)
		stream.Flush()
	}
}

// BenchmarkStreamConverter_MultipleWrites benchmarks streaming with multiple writes
func BenchmarkStreamConverter_MultipleWrites(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skip("SoX not installed")
	}

	chunk := generateBenchmarkPCM(16000, 1, 20) // 20ms chunks

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
		stream.Start()

		// Write 50 chunks (1 second total)
		for j := 0; j < 50; j++ {
			stream.Write(chunk)
		}

		stream.Flush()
	}
}

// BenchmarkConverter_PCMToFLAC benchmarks PCM to FLAC conversion
func BenchmarkConverter_PCMToFLAC(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skip("SoX not installed")
	}

	pcmData := generateBenchmarkPCM(16000, 1, 1000)
	converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)

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

	pcmData := generateBenchmarkPCM(16000, 1, 1000)
	converter := NewConverter(PCM_RAW_16K_MONO, WAV_16K_MONO)

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

	pcmData := generateBenchmarkPCM(16000, 1, 1000)
	converter := NewConverter(PCM_RAW_16K_MONO, ULAW_8K_MONO)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := bytes.NewReader(pcmData)
		output := &bytes.Buffer{}
		converter.Convert(input, output)
	}
}

// BenchmarkConverter_WithPool benchmarks conversion with pool
func BenchmarkConverter_WithPool(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skip("SoX not installed")
	}

	pcmData := generateBenchmarkPCM(16000, 1, 1000)
	pool := NewPoolWithLimit(10)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO).
			WithPool(pool)

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

	pcmData := generateBenchmarkPCM(16000, 1, 100) // Smaller for parallel

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
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
