package sox

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
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

// TestConverterSuite runs the converter test suite
func TestConverterSuite(t *testing.T) {
	suite.Run(t, new(ConverterTestSuite))
}

// TestPresets tests format presets
func (s *ConverterTestSuite) TestPresets() {
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
func TestConverter_ConvertSmall(b *testing.B) {
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

// BenchmarkStreamConverter benchmarks streaming conversion
func BenchmarkStreamConverter(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skip("SoX not installed")
	}

	pcmData := generateBenchmarkPCM(8000, 1, 1000) // 1 second

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		stream := NewStreamer(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE)
		stream.Start(1 * time.Second)
		stream.Write(pcmData)
		stream.End()
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
		stream := NewStreamer(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE)
		stream.Start(1 * time.Second)

		// Write 50 chunks (1 second total)
		for j := 0; j < 50; j++ {
			stream.Write(chunk)
		}

		stream.Stop()
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

// BenchmarkConverter_WithPool benchmarks conversion with pool
func BenchmarkConverter_WithPool(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skip("SoX not installed")
	}

	pcmData := generateBenchmarkPCM(8000, 1, 1000)
	pool := NewPoolWithLimit(10)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		converter := New(PCM_RAW_8K_MONO, FLAC_16K_MONO_LE).
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
