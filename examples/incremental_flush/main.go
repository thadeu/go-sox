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

	stream := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
		WithOutputPath(outputPath).
		WithAutoFlush(3 * time.Second) // Flush every 3 seconds

	if err := stream.Start(); err != nil {
		log.Fatalf("Failed to start stream: %v", err)
	}

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

		// Check file size periodically
		if i%5 == 0 {
			if fileInfo, err := os.Stat(outputPath); err == nil {
				fmt.Printf("After %d packets: file size %d bytes\n", i+1, fileInfo.Size())
			}
		}

		time.Sleep(100 * time.Millisecond) // Simulate packet interval
	}

	// Final flush
	_, err := stream.Flush()
	if err != nil {
		log.Printf("Final flush error: %v", err)
	}

	// Check final file size
	if fileInfo, err := os.Stat(outputPath); err == nil {
		fmt.Printf("Final file size: %d bytes\n", fileInfo.Size())
	}

	fmt.Println("\n=== Example 2: Error Handling ===")

	// Example 2: WithAutoFlush without WithOutputPath should fail
	invalidStream := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
		WithAutoFlush(1 * time.Second) // No WithOutputPath

	err = invalidStream.Start()
	if err != nil {
		fmt.Println("✓ Correctly failed: WithAutoFlush requires WithOutputPath")
		fmt.Printf("  Error: %v\n", err)
	}

	fmt.Println("\n=== Summary ===")
	fmt.Println("✓ Auto-flush: File grows during streaming, other processes can read it")
	fmt.Println("✓ Requires WithOutputPath: Prevents confusion and ensures correct usage")
	fmt.Println("✓ Perfect for real-time transcription pipelines")
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
