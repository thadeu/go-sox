package main

import (
	"fmt"
	"log"
	"time"

	sox "github.com/thadeu/go-sox"
)

// simulateRTPPacket simulates receiving an RTP packet with PCM audio data
func simulateRTPPacket(sampleRate, durationMs int) []byte {
	numSamples := (sampleRate * durationMs) / 1000
	buffer := make([]byte, numSamples*2) // mono, 16-bit

	// Generate simple test audio
	for i := 0; i < numSamples; i++ {
		value := int16((i % 1000) * 32) // Simple ramp pattern
		buffer[i*2] = byte(value & 0xFF)
		buffer[i*2+1] = byte((value >> 8) & 0xFF)
	}

	return buffer
}

// Example: RTP Recorder with simplified API
func main() {
	if err := sox.CheckSoxInstalled(""); err != nil {
		log.Fatalf("SoX not installed: %v", err)
	}

	fmt.Println("RTP Recorder Example - Simplified API")
	fmt.Println("====================================\n")

	// Example 1: Basic streaming without pool (for single calls)
	fmt.Println("1. Basic streaming (single call):")
	basicStreaming()

	// Example 2: Multiple concurrent calls with pool
	fmt.Println("\n2. Multiple concurrent calls with pool:")
	concurrentCalls()

	// Example 3: Direct file output (no need to write manually)
	fmt.Println("\n3. Direct file output:")
	directFileOutput()
}

func basicStreaming() {
	// Simple streaming without pool
	converter := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO)

	if err := converter.Start(); err != nil {
		log.Fatalf("Failed to start: %v", err)
	}

	// Simulate RTP packets
	for i := 0; i < 10; i++ {
		packet := simulateRTPPacket(16000, 20) // 20ms packets
		converter.Write(packet)
		time.Sleep(time.Millisecond * 5)
	}

	// Get converted data
	data, err := converter.Flush()
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	fmt.Printf("   Converted %d bytes of FLAC data\n", len(data))
}

func concurrentCalls() {
	// Create SHARED pool for concurrent conversions
	pool := sox.NewPoolWithLimit(2) // Limit to 2 concurrent conversions

	// Simulate multiple concurrent calls
	for callID := 1; callID <= 5; callID++ {
		go func(id int) {
			// Each call gets its own converter with SHARED pool
			converter := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
				WithPool(pool) // Pool compartilhado!

			if err := converter.Start(); err != nil {
				log.Printf("Call %d failed to start (pool full): %v", id, err)
				return
			}

			fmt.Printf("   Call %d started (pool slot acquired)\n", id)

			// Simulate RTP packets for this call
			for i := 0; i < 20; i++ {
				packet := simulateRTPPacket(16000, 20)
				converter.Write(packet)
				time.Sleep(time.Millisecond * 10)
			}

			// Get converted data
			data, err := converter.Flush()
			if err != nil {
				log.Printf("Call %d conversion failed: %v", id, err)
				return
			}

			fmt.Printf("   Call %d completed: %d bytes FLAC (pool slot released)\n", id, len(data))
		}(callID)
	}

	// Wait for all calls to complete
	time.Sleep(time.Second * 3)
}

func directFileOutput() {
	// Direct file output - no need to write manually!
	pool := sox.NewPoolWithLimit(1) // Single conversion
	converter := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
		WithOutputPath("/tmp/test_call.flac").
		WithPool(pool) // Pool compartilhado

	if err := converter.Start(); err != nil {
		log.Fatalf("Failed to start: %v", err)
	}

	// Simulate RTP packets
	for i := 0; i < 50; i++ {
		packet := simulateRTPPacket(16000, 20)
		converter.Write(packet)
		time.Sleep(time.Millisecond * 5)
	}

	// Flush - file is automatically written!
	_, err := converter.Flush()
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	fmt.Println("   File written to /tmp/test_call.flac")
	fmt.Println("   No manual file writing needed!")
}
