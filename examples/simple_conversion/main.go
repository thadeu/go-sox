package main

import (
	"bytes"
	"fmt"
	"log"
	"os"

	sox "github.com/thadeu/go-sox"
)

func main() {
	// Example 1: Convert using io.Reader and io.Writer
	fmt.Println("Example 1: Convert using io.Reader/io.Writer")
	convertWithReaderWriter()

	// Example 2: Convert using file paths
	fmt.Println("\nExample 2: Convert using file paths")
	convertWithFilePaths()

	// Example 3: Convert with options and resilience
	fmt.Println("\nExample 3: Convert with custom options")
	convertWithOptions()
}

// convertWithReaderWriter demonstrates bytes-to-bytes conversion
func convertWithReaderWriter() {
	// Create a small PCM buffer (8kHz, mono, 16-bit)
	pcmData := generateTestPCM(1000) // 1 second of audio

	// Create converter: PCM 8kHz mono -> FLAC
	conv := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE)

	// Convert from bytes to bytes using io.Reader and io.Writer
	inputReader := bytes.NewReader(pcmData)
	outputBuffer := &bytes.Buffer{}

	if err := conv.Convert(inputReader, outputBuffer); err != nil {
		log.Fatalf("Conversion failed: %v", err)
	}

	fmt.Printf("Input: %d bytes, Output: %d bytes\n", len(pcmData), outputBuffer.Len())
}

// convertWithFilePaths demonstrates file-to-file conversion
func convertWithFilePaths() {
	// Create temporary files for testing
	tmpDir := os.TempDir()
	inputPath := tmpDir + "/test_input.pcm"
	outputPath := tmpDir + "/test_output.flac"

	// Create input file with test data
	pcmData := generateTestPCM(1000)
	if err := os.WriteFile(inputPath, pcmData, 0644); err != nil {
		log.Fatalf("Failed to create input file: %v", err)
	}
	defer os.Remove(inputPath)
	defer os.Remove(outputPath)

	// Create converter and convert using file paths
	conv := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE)

	if err := conv.Convert(inputPath, outputPath); err != nil {
		log.Fatalf("Conversion failed: %v", err)
	}

	// Check output file size
	info, err := os.Stat(outputPath)
	if err != nil {
		log.Fatalf("Failed to stat output file: %v", err)
	}

	fmt.Printf("Created output file: %s (%d bytes)\n", outputPath, info.Size())
}

// convertWithOptions demonstrates conversion with custom options and resilience
func convertWithOptions() {
	pcmData := generateTestPCM(1000)

	// Create converter with custom options and resilience
	options := sox.DefaultOptions()
	options.ShowProgress = false

	conv := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE).
		WithOptions(options).
		WithPool(sox.NewPoolWithLimit(5)).
		WithRetryConfig(sox.RetryConfig{
			MaxAttempts:   3,
			InitialBackoff: 100 * 1e6, // 100ms
			MaxBackoff:    1000 * 1e6, // 1s
			BackoffMultiple: 2.0,
		})

	inputReader := bytes.NewReader(pcmData)
	outputBuffer := &bytes.Buffer{}

	if err := conv.Convert(inputReader, outputBuffer); err != nil {
		log.Fatalf("Conversion failed: %v", err)
	}

	fmt.Printf("Successfully converted with resilience: %d bytes output\n", outputBuffer.Len())
}

// generateTestPCM generates PCM test data
func generateTestPCM(durationMs int) []byte {
	sampleRate := 8000
	channels := 1
	bitDepth := 16

	numSamples := (sampleRate * durationMs) / 1000
	buffer := make([]byte, numSamples*channels*(bitDepth/8))

	for i := 0; i < numSamples; i++ {
		value := int16(32767 * 0.5 * (1.0 + float64(i%100)/100.0))
		for ch := 0; ch < channels; ch++ {
			idx := (i*channels + ch) * 2
			buffer[idx] = byte(value & 0xFF)
			buffer[idx+1] = byte((value >> 8) & 0xFF)
		}
	}

	return buffer
}
