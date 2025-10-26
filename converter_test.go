package sox

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
)

// TestCheckSoxInstalled verifies that SoX is installed
func TestCheckSoxInstalled(t *testing.T) {
	err := CheckSoxInstalled("")
	if err != nil {
		t.Skipf("SoX not installed, skipping tests: %v", err)
	}
}

// generateTestPCM generates a simple PCM audio buffer
func generateTestPCM(sampleRate, channels, durationMs int) []byte {
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

// TestAudioFormat_Validate tests format validation
func TestAudioFormat_Validate(t *testing.T) {
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
			name: "invalid raw format - no encoding",
			format: AudioFormat{
				Type:       "raw",
				SampleRate: 16000,
				Channels:   1,
				BitDepth:   16,
			},
			wantErr: true,
		},
		{
			name: "invalid raw format - no sample rate",
			format: AudioFormat{
				Type:     "raw",
				Encoding: "signed-integer",
				Channels: 1,
				BitDepth: 16,
			},
			wantErr: true,
		},
		{
			name:    "valid flac format",
			format:  FLAC_16K_MONO,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.format.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestConverter_Convert tests basic conversion
func TestConverter_Convert(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skipf("SoX not installed: %v", err)
	}

	// Generate test PCM data
	pcmData := generateTestPCM(16000, 1, 100) // 100ms of audio

	// Create converter
	converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)

	// Convert
	input := bytes.NewReader(pcmData)
	output := &bytes.Buffer{}

	err := converter.Convert(input, output)
	if err != nil {
		t.Fatalf("Convert() failed: %v", err)
	}

	// Verify output is not empty
	if output.Len() == 0 {
		t.Error("Convert() produced empty output")
	}

	// FLAC files start with "fLaC" magic bytes
	if output.Len() >= 4 {
		magic := output.Bytes()[:4]
		if string(magic) != "fLaC" {
			t.Errorf("Output doesn't appear to be FLAC (magic: %x)", magic)
		}
	}
}

// TestConverter_ConvertFile tests file-based conversion
func TestConverter_ConvertFile(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skipf("SoX not installed: %v", err)
	}

	// Create temporary input file
	tmpInput, err := os.CreateTemp("", "test_input_*.raw")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpInput.Name())

	// Write test PCM data
	pcmData := generateTestPCM(16000, 1, 100)
	if _, err := tmpInput.Write(pcmData); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}
	tmpInput.Close()

	// Create temporary output file
	tmpOutput, err := os.CreateTemp("", "test_output_*.flac")
	if err != nil {
		t.Fatalf("Failed to create temp output file: %v", err)
	}
	tmpOutput.Close()
	defer os.Remove(tmpOutput.Name())

	// Create converter and convert
	converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
	err = converter.ConvertFile(tmpInput.Name(), tmpOutput.Name())
	if err != nil {
		t.Fatalf("ConvertFile() failed: %v", err)
	}

	// Verify output file exists and is not empty
	stat, err := os.Stat(tmpOutput.Name())
	if err != nil {
		t.Fatalf("Output file not created: %v", err)
	}

	if stat.Size() == 0 {
		t.Error("Output file is empty")
	}
}

// TestStreamConverter tests streaming conversion
func TestStreamConverter(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skipf("SoX not installed: %v", err)
	}

	// Create stream converter
	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)

	// Start the stream
	err := stream.Start()
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Write test data in chunks (simulating RTP packets)
	pcmData := generateTestPCM(16000, 1, 500) // 500ms of audio
	chunkSize := 320                          // 20ms at 16kHz mono 16-bit

	for i := 0; i < len(pcmData); i += chunkSize {
		end := i + chunkSize
		if end > len(pcmData) {
			end = len(pcmData)
		}

		_, err := stream.Write(pcmData[i:end])
		if err != nil {
			t.Fatalf("Write() failed: %v", err)
		}
	}

	// Flush and get output
	output, err := stream.Flush()
	if err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	// Verify output
	if len(output) == 0 {
		t.Error("Stream conversion produced empty output")
	}

	// Verify FLAC magic bytes
	if len(output) >= 4 && string(output[:4]) != "fLaC" {
		t.Errorf("Output doesn't appear to be FLAC (magic: %x)", output[:4])
	}
}

// TestStreamConverter_MultipleWrites tests accumulating data over multiple writes
func TestStreamConverter_MultipleWrites(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skipf("SoX not installed: %v", err)
	}

	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)

	err := stream.Start()
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Write multiple chunks with different sizes
	chunks := []int{100, 200, 150, 250} // different durations in ms
	for _, durationMs := range chunks {
		pcmData := generateTestPCM(16000, 1, durationMs)
		_, err := stream.Write(pcmData)
		if err != nil {
			t.Fatalf("Write() failed: %v", err)
		}
	}

	output, err := stream.Flush()
	if err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	if len(output) == 0 {
		t.Error("Stream conversion produced empty output")
	}
}

// TestPresets verifies that preset formats are valid
func TestPresets(t *testing.T) {
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
		if err := preset.Validate(); err != nil {
			t.Errorf("Preset %+v failed validation: %v", preset, err)
		}
	}
}

// BenchmarkConverter_Convert benchmarks the conversion performance
func BenchmarkConverter_Convert(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skipf("SoX not installed: %v", err)
	}

	// Generate 1 second of audio
	pcmData := generateTestPCM(16000, 1, 1000)
	converter := NewConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		input := bytes.NewReader(pcmData)
		output := &bytes.Buffer{}
		err := converter.Convert(input, output)
		if err != nil {
			b.Fatalf("Convert() failed: %v", err)
		}
	}
}

// BenchmarkStreamConverter benchmarks streaming conversion
func BenchmarkStreamConverter(b *testing.B) {
	if err := CheckSoxInstalled(""); err != nil {
		b.Skipf("SoX not installed: %v", err)
	}

	pcmData := generateTestPCM(16000, 1, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
		stream.Start()
		stream.Write(pcmData)
		stream.Flush()
	}
}

// BenchmarkFFmpegComparison provides a baseline comparison with ffmpeg (if available)
func BenchmarkFFmpegComparison(b *testing.B) {
	// Check if ffmpeg is available
	if err := exec.Command("ffmpeg", "-version").Run(); err != nil {
		b.Skipf("ffmpeg not installed")
	}

	// Create temporary files
	tmpInput, _ := os.CreateTemp("", "bench_input_*.raw")
	tmpOutput, _ := os.CreateTemp("", "bench_output_*.flac")
	defer os.Remove(tmpInput.Name())
	defer os.Remove(tmpOutput.Name())

	pcmData := generateTestPCM(16000, 1, 1000)
	tmpInput.Write(pcmData)
	tmpInput.Close()
	tmpOutput.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command("ffmpeg", "-y",
			"-f", "s16le",
			"-ar", "16000",
			"-ac", "1",
			"-i", tmpInput.Name(),
			tmpOutput.Name())
		cmd.Run()
	}
}
