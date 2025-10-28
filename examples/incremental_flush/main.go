package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	sox "github.com/thadeu/go-sox"
)

func main() {
	// Check if SoX is installed
	if err := sox.CheckSoxInstalled(""); err != nil {
		log.Fatalf("SoX not installed: %v", err)
	}

	// Create output directory
	outputDir := "output"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Example 1: Incremental flush for real-time processing
	fmt.Println("=== Example 1: Incremental Flush (Real-time Processing) ===")

	outputPath := filepath.Join(outputDir, "realtime.flac")

	stream := sox.NewStreamer(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE).
		WithOutputPath(outputPath)

	// Start with auto-flush every 3 seconds
	stream.Start(3 * time.Second)

	// Simulate RTP packet streaming
	fmt.Println("Simulating RTP packet streaming...")
	for i := 0; i < 20; i++ {
		// Generate test PCM data (simulating RTP payload)
		chunk := generateTestPCMData(16000, 200) // 200ms chunks

		_, err := stream.Write(chunk)
		if err != nil {
			log.Printf("Error writing chunk %d: %v", i, err)
			continue
		}

		time.Sleep(100 * time.Millisecond) // Simulate packet interval
	}

	// Final flush - stop the stream and flush remaining data
	if err := stream.Stop(); err != nil {
		log.Printf("Final flush error: %v", err)
	}

	// Check for incremental files created by auto-flush
	fmt.Println("\n=== Checking incremental files ===")
	files, _ := filepath.Glob(filepath.Join(outputDir, "realtime.flac.*"))
	if len(files) > 0 {
		fmt.Printf("Found %d incremental flush files:\n", len(files))
		for _, file := range files {
			if fileInfo, err := os.Stat(file); err == nil {
				fmt.Printf("  - %s: %d bytes\n", filepath.Base(file), fileInfo.Size())
			}
		}
	}

	// Check final file
	if fileInfo, err := os.Stat(outputPath); err == nil {
		fmt.Printf("Final file %s: %d bytes\n", filepath.Base(outputPath), fileInfo.Size())
	}

	fmt.Println("\n=== Summary ===")
	fmt.Println("✓ Auto-flush: Creates incremental files during streaming")
	fmt.Println("✓ Periodic flush: Saves data every N seconds without blocking writes")
	fmt.Println("✓ Perfect for real-time processing pipelines")
}

// generateTestPCMData generates test PCM audio data
func generateTestPCMData(sampleRate, durationMs int) []byte {
	numSamples := (sampleRate * durationMs) / 1000
	buffer := make([]byte, numSamples*2) // mono, 16-bit
	for i := 0; i < numSamples; i++ {
		value := int16((i % 1000) * 32)
		buffer[i*2] = byte(value & 0xFF)
		buffer[i*2+1] = byte((value >> 8) & 0xFF)
	}
	return buffer
}
