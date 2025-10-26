package main

import (
	"bytes"
	"fmt"
	"log"

	sox "github.com/thadeu/go-sox"
)

func main() {
	if err := sox.CheckSoxInstalled(""); err != nil {
		log.Fatalf("SoX not installed: %v", err)
	}

	fmt.Println("Simple Usage Examples")
	fmt.Println("====================\n")

	// Example 1: Basic conversion (resilient by default)
	fmt.Println("1. Basic conversion with default resiliency:")
	basicExample()

	// Example 2: With automatic pool
	fmt.Println("\n2. With worker pool (automatic):")
	poolExample()

	// Example 3: ConvertFile with resiliency
	fmt.Println("\n3. File conversion with automatic retry:")
	fileExample()
}

func basicExample() {
	pcmData := generatePCM(16000, 100) // 100ms audio

	converter := sox.NewConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO)

	input := bytes.NewReader(pcmData)
	output := &bytes.Buffer{}

	if err := converter.Convert(input, output); err != nil {
		log.Printf("Error: %v", err)
		return
	}

	fmt.Printf("   Converted %d bytes PCM → %d bytes FLAC\n", len(pcmData), output.Len())
}

func poolExample() {
	pcmData := generatePCM(16000, 100)

	// WithPool() creates default pool automatically (SOX_MAX_WORKERS)
	converter := sox.NewConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
		WithPool() // No need to create pool manually!

	input := bytes.NewReader(pcmData)
	output := &bytes.Buffer{}

	if err := converter.Convert(input, output); err != nil {
		log.Printf("Error: %v", err)
		return
	}

	fmt.Printf("   Converted with pool: %d bytes PCM → %d bytes FLAC\n", len(pcmData), output.Len())
}

func fileExample() {
	// ConvertFile also has automatic retry + circuit breaker
	_ = sox.NewConverter(sox.PCM_RAW_16K_MONO, sox.WAV_16K_MONO).
		WithPool()

	// Note: This would fail because we don't have actual files
	// Just showing the API
	fmt.Println("   converter.ConvertFile(\"input.raw\", \"output.wav\")")
	fmt.Println("   → Includes automatic retry, circuit breaker, and pool control")
}

func generatePCM(sampleRate, durationMs int) []byte {
	numSamples := (sampleRate * durationMs) / 1000
	buffer := make([]byte, numSamples*2)
	for i := 0; i < numSamples; i++ {
		value := int16((i % 1000) * 32)
		buffer[i*2] = byte(value & 0xFF)
		buffer[i*2+1] = byte((value >> 8) & 0xFF)
	}
	return buffer
}
