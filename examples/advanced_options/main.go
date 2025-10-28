package main

import (
	"bytes"
	"fmt"
	"log"
	"math"

	sox "github.com/thadeu/go-sox"
)

func main() {
	fmt.Println("=== go-sox Advanced Options Example ===\n")

	// Example 1: Using extended format options
	example1_ExtendedFormatOptions()

	// Example 2: Using custom arguments for full flexibility
	example2_CustomArguments()

	// Example 3: Using global options
	example3_GlobalOptions()

	// Example 4: Complete example with all features
	example4_CompleteExample()
}

// Example 1: Using extended format options
func example1_ExtendedFormatOptions() {
	fmt.Println("Example 1: Extended Format Options")
	fmt.Println("-----------------------------------")

	// Create input format with volume adjustment and compression
	input := sox.AudioFormat{
		Type:       "raw",
		Encoding:   "signed-integer",
		SampleRate: 16000,
		Channels:   1,
		BitDepth:   16,
		Volume:     1.5, // Increase volume by 50%
	}

	// Create output format with compression and comment
	output := sox.AudioFormat{
		Type:        "flac",
		SampleRate:  16000,
		Channels:    1,
		BitDepth:    16,
		Compression: 8.0, // Maximum compression for FLAC
		Comment:     "Generated with go-sox advanced options",
	}

	converter := sox.New(input, output)

	// Generate sample PCM data
	pcmData := generateTestPCM(16000, 1, 100)

	inputReader := bytes.NewReader(pcmData)
	output_buffer := &bytes.Buffer{}

	if err := converter.Convert(inputReader, output_buffer); err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("✓ Converted %d bytes PCM to %d bytes FLAC with compression\n", len(pcmData), output_buffer.Len())
	fmt.Printf("  Volume adjustment: 1.5x\n")
	fmt.Printf("  Compression level: 8\n\n")
}

// Example 2: Using custom arguments for full flexibility
func example2_CustomArguments() {
	fmt.Println("Example 2: Custom Arguments")
	fmt.Println("----------------------------")

	// Use CustomArgs to pass any SoX parameter
	input := sox.AudioFormat{
		Type:       "raw",
		SampleRate: 8000,
		Channels:   1,
		BitDepth:   16,
		// Using CustomArgs for parameters not explicitly defined
		CustomArgs: []string{"-e", "signed-integer"},
	}

	output := sox.AudioFormat{
		Type:       "flac",
		SampleRate: 16000, // Upsample to 16kHz
		Channels:   1,
		BitDepth:   16,
		// Add multiple custom arguments
		CustomArgs: []string{"--add-comment", "Processed with custom args"},
	}

	converter := sox.New(input, output)

	// Generate sample PCM data
	pcmData := generateTestPCM(8000, 1, 100)

	inputReader := bytes.NewReader(pcmData)
	output_buffer := &bytes.Buffer{}

	if err := converter.Convert(inputReader, output_buffer); err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("✓ Converted with custom arguments\n")
	fmt.Printf("  Upsampled from 8kHz to 16kHz\n")
	fmt.Printf("  Output size: %d bytes\n\n", output_buffer.Len())
}

// Example 3: Using global options
func example3_GlobalOptions() {
	fmt.Println("Example 3: Global Options")
	fmt.Println("-------------------------")

	input := sox.PCM_RAW_8K_MONO
	output := sox.FLAC_16K_MONO_LE

	// Configure global options
	options := sox.ConversionOptions{
		SoxPath:          "sox",
		ShowProgress:     false,
		Buffer:           16384,                  // Custom buffer size
		NoDither:         true,                   // Disable dithering
		SingleThreaded:   false,                  // Enable parallel processing
		VerbosityLevel:   0,                      // Quiet mode
		Guard:            true,                   // Guard against clipping
		Norm:             false,                  // Don't normalize
		CustomGlobalArgs: []string{},             // Additional global args if needed
		Effects:          []string{"norm", "-3"}, // Normalize to -3dB
	}

	converter := sox.New(input, output).WithOptions(options)

	// Generate sample PCM data
	pcmData := generateTestPCM(16000, 1, 100)

	inputReader := bytes.NewReader(pcmData)
	output_buffer := &bytes.Buffer{}

	if err := converter.Convert(inputReader, output_buffer); err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("✓ Converted with global options\n")
	fmt.Printf("  Buffer size: 16384 bytes\n")
	fmt.Printf("  No dithering enabled\n")
	fmt.Printf("  Guard against clipping enabled\n")
	fmt.Printf("  Applied normalization effect\n\n")
}

// Example 4: Complete example with all features
func example4_CompleteExample() {
	fmt.Println("Example 4: Complete Advanced Example")
	fmt.Println("-------------------------------------")

	// Input with all extended options
	input := sox.AudioFormat{
		Type:         "raw",
		Encoding:     "signed-integer",
		SampleRate:   8000,
		Channels:     1,
		BitDepth:     16,
		Volume:       2.0,      // Double the volume
		IgnoreLength: true,     // Ignore length in header
		Endian:       "little", // Little-endian byte order
	}

	// Output with all extended options
	output := sox.AudioFormat{
		Type:        "flac",
		SampleRate:  16000, // Upsample
		Channels:    2,     // Convert to stereo
		BitDepth:    24,    // Increase bit depth
		Compression: 8.0,   // Maximum FLAC compression
		Comment:     "Advanced example with all features",
		AddComment:  "Processed on " + "2025-10-27",
	}

	// Advanced global options
	options := sox.ConversionOptions{
		SoxPath:        "sox",
		ShowProgress:   false,
		Buffer:         32768,
		NoDither:       false,
		Guard:          false,
		Norm:           false,
		VerbosityLevel: 0,
		Effects: []string{
			"channels", "2", // Convert mono to stereo
			"gain", "-n", // Normalize
		},
	}

	converter := sox.New(input, output).
		WithOptions(options).
		WithRetryConfig(sox.RetryConfig{
			MaxAttempts: 3,
		})

	// Generate sample PCM data
	pcmData := generateTestPCM(8000, 1, 200)

	inputReader := bytes.NewReader(pcmData)
	output_buffer := &bytes.Buffer{}

	if err := converter.Convert(inputReader, output_buffer); err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("✓ Complete advanced conversion successful\n")
	fmt.Printf("  Input:  8kHz mono 16-bit PCM\n")
	fmt.Printf("  Output: 16kHz stereo 24-bit FLAC\n")
	fmt.Printf("  Volume: 2x\n")
	fmt.Printf("  Compression: 8 (maximum)\n")
	fmt.Printf("  Effects: mono->stereo, normalize\n")
	fmt.Printf("  Size: %d -> %d bytes\n\n", len(pcmData), output_buffer.Len())
}

// generateTestPCM generates simple PCM audio data for testing
func generateTestPCM(sampleRate, channels, durationMs int) []byte {
	numSamples := (sampleRate * durationMs) / 1000
	buffer := make([]byte, numSamples*channels*2) // 16-bit = 2 bytes per sample

	// Generate a simple sine wave
	frequency := 440.0 // A4 note
	for i := 0; i < numSamples; i++ {
		t := float64(i) / float64(sampleRate)
		value := int16(32767.0 * 0.5 * math.Sin(2*math.Pi*frequency*t))

		for ch := 0; ch < channels; ch++ {
			idx := (i*channels + ch) * 2
			buffer[idx] = byte(value & 0xFF)
			buffer[idx+1] = byte((value >> 8) & 0xFF)
		}
	}

	return buffer
}
