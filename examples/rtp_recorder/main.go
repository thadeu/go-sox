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

	fmt.Println("RTP Recorder Example - Simplified API with Auto-Flush")
	fmt.Println("=====================================================\n")

	// Example 1: Auto-flush with ticker (simplest!)
	fmt.Println("1. Auto-flush with ticker (simplest!):")
	autoFlushExample()

	// Example 2: Manual flush (full control)
	fmt.Println("\n2. Manual flush (full control):")
	manualFlushExample()

}

func autoFlushExample() {
	// Auto-flush after 2 seconds - SUPER SIMPLE!
	converter := sox.NewStreamer(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE).
		WithOutputPath("/tmp/test_auto.flac").
		WithAutoStart(2 * time.Second) // Flush automatically after 2s!

	converter.Start(2 * time.Second)

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
	converter := sox.NewStreamer(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE).
		WithOutputPath("/tmp/test_manual.flac")

	converter.Start(2 * time.Second)

	// Simulate RTP packets
	for i := 0; i < 50; i++ {
		packet := simulateRTPPacket(16000, 20)
		converter.Write(packet)
		time.Sleep(time.Millisecond * 5)
	}

	// Manual flush when you want
	converter.End()
	fmt.Println("   File saved to /tmp/test_manual.flac")
}
