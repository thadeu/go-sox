package main

import (
	"context"
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

	fmt.Println("RTP Recorder Example - Simplified API with Auto-Flush")
	fmt.Println("=====================================================\n")

	// Example 1: Auto-flush with ticker (simplest!)
	fmt.Println("1. Auto-flush with ticker (simplest!):")
	autoFlushExample()

	// Example 2: Manual flush (full control)
	fmt.Println("\n2. Manual flush (full control):")
	manualFlushExample()

	// Example 3: Multiple concurrent calls with pool
	fmt.Println("\n3. Multiple concurrent calls with pool:")
	concurrentCallsWithPool()
}

func autoFlushExample() {
	// Auto-flush after 2 seconds - SUPER SIMPLE!
	converter := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
		WithOutputPath("/tmp/test_auto.flac").
		WithAutoFlush(2 * time.Second) // Flush automatically after 2s!

	converter.Start()

	fmt.Println("   Writing RTP packets...")
	// Simulate RTP packets
	for i := 0; i < 100; i++ {
		packet := simulateRTPPacket(16000, 20) // 20ms packets
		converter.Write(packet)
		time.Sleep(time.Millisecond * 10)
	}

	// Auto-flush will trigger after 2 seconds automatically!
	time.Sleep(2500 * time.Millisecond)

	fmt.Println("   File saved to /tmp/test_auto.flac (auto-flushed!)")
}

func manualFlushExample() {
	converter := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
		WithOutputPath("/tmp/test_manual.flac")

	converter.Start()

	// Simulate RTP packets
	for i := 0; i < 50; i++ {
		packet := simulateRTPPacket(16000, 20)
		converter.Write(packet)
		time.Sleep(time.Millisecond * 5)
	}

	// Manual flush when you want
	converter.Flush()
	fmt.Println("   File saved to /tmp/test_manual.flac")
}

func concurrentCallsWithPool() {
	// Create SHARED pool for concurrent conversions
	pool := sox.NewPoolWithLimit(2) // Limit to 2 concurrent conversions

	// Simulate multiple concurrent calls
	for callID := 1; callID <= 3; callID++ {
		go func(id int) {
			// Each call with auto-flush
			converter := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
				WithOutputPath(fmt.Sprintf("/tmp/call_%d.flac", id)).
				WithPool(pool).
				WithAutoFlush(1 * time.Second) // Auto-flush after 1s

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := converter.Start(ctx); err != nil {
				log.Printf("Call %d failed to start (pool full): %v", id, err)
				return
			}

			fmt.Printf("   Call %d started (pool slot acquired)\n", id)

			// Simulate RTP packets
			for i := 0; i < 50; i++ {
				packet := simulateRTPPacket(16000, 20)
				converter.Write(packet)
				time.Sleep(time.Millisecond * 15)
			}

			// Wait for auto-flush
			time.Sleep(1500 * time.Millisecond)

			fmt.Printf("   Call %d completed: /tmp/call_%d.flac saved (auto-flushed!)\n", id, id)
		}(callID)
	}

	// Wait for all calls to complete
	time.Sleep(time.Second * 4)
}
