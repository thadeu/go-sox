package main

import (
	"fmt"
	"log"
	"time"

	sox "github.com/thadeu/go-sox"
)

func main() {
	// Example: Ticker-based conversion that processes buffered audio at regular intervals
	// Useful for real-time audio processing where data arrives continuously
	fmt.Println("Starting ticker-based conversion (3 second intervals)")

	conv := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE).
		WithTicker(3 * time.Second)

	// Start the ticker conversion
	if err := conv.Start(); err != nil {
		log.Fatalf("Failed to start converter: %v", err)
	}

	// Simulate receiving audio packets over time
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	stopChan := make(chan struct{})
	go func() {
		time.Sleep(10 * time.Second)
		close(stopChan)
	}()

	packetCount := 0
	for {
		select {
		case <-ticker.C:
			// Simulate receiving a 100ms audio packet
			packet := generateAudioPacket(100)
			n, err := conv.Write(packet)
			if err != nil {
				log.Printf("Failed to write packet: %v", err)
				continue
			}
			packetCount++
			fmt.Printf("Wrote packet %d: %d bytes\n", packetCount, n)

		case <-stopChan:
			fmt.Println("Stopping converter and flushing remaining data...")
			if err := conv.Stop(); err != nil {
				log.Printf("Error stopping converter: %v", err)
			}
			fmt.Printf("Successfully processed %d packets\n", packetCount)
			return
		}
	}
}

// generateAudioPacket generates a small packet of PCM audio data
// Duration in milliseconds
func generateAudioPacket(durationMs int) []byte {
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
