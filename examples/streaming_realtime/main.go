package main

import (
	"fmt"
	"log"
	"time"

	sox "github.com/thadeu/go-sox"
)

func main() {
	// Example: Real-time streaming conversion
	// Data flows through sox in real-time without waiting
	fmt.Println("Starting real-time streaming conversion")

	conv := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE).
		WithStream()

	// Start the streaming process (creates sox pipes)
	if err := conv.Start(); err != nil {
		log.Fatalf("Failed to start stream: %v", err)
	}

	// Channel to signal when we're done writing
	done := make(chan struct{})

	// Goroutine: continuously write audio packets to stdin
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		for i := 0; i < 20; i++ {
			<-ticker.C
			packet := generateAudioPacket(50) // 50ms of audio
			n, err := conv.Write(packet)
			if err != nil {
				log.Printf("Write error: %v", err)
				break
			}
			fmt.Printf("Wrote packet %d: %d bytes\n", i+1, n)
		}

		// Close stdin to signal end of input
		if err := conv.Stop(); err != nil {
			log.Printf("Error closing stream: %v", err)
		}
		close(done)
	}()

	// Goroutine: continuously read converted output
	go func() {
		buffer := make([]byte, 4096)
		totalRead := 0

		for {
			n, err := conv.Read(buffer)
			if err != nil {
				if err.Error() != "EOF" {
					log.Printf("Read error: %v", err)
				}
				break
			}
			if n > 0 {
				totalRead += n
				fmt.Printf("Read %d bytes (total: %d)\n", n, totalRead)
			}
		}
	}()

	// Wait for writer to finish
	<-done
	fmt.Println("Streaming conversion completed")
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
