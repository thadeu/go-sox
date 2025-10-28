package main

import (
	"bytes"
	"fmt"
	"log"
	"time"

	sox "github.com/thadeu/go-sox"
)

// simulateRTPPacket simulates receiving an RTP packet with PCM audio data
// In real usage, this would come from your SIP/RTP stack
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

// Example 1: Basic one-shot conversion
func exampleBasicConversion() {
	fmt.Println("=== Example 1: Basic One-Shot Conversion ===")

	// Generate some PCM audio (simulating received RTP)
	pcmData := simulateRTPPacket(8000, 1000) // 1 second at 16kHz
	fmt.Printf("Generated %d bytes of PCM audio\n", len(pcmData))

	// Create converter: PCM Raw 16kHz mono → FLAC 16kHz mono
	converter := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE)

	// Convert
	input := bytes.NewReader(pcmData)
	output := &bytes.Buffer{}

	start := time.Now()
	err := converter.Convert(input, output)
	elapsed := time.Since(start)

	if err != nil {
		log.Fatalf("Conversion failed: %v", err)
	}

	fmt.Printf("Converted in %v\n", elapsed)
	fmt.Printf("Output size: %d bytes (%.2f%% of input)\n\n",
		output.Len(),
		float64(output.Len())/float64(len(pcmData))*100)
}

// Example 2: Streaming conversion (RTP accumulation scenario)
func exampleStreamingConversion() {
	fmt.Println("=== Example 2: Streaming Conversion (RTP Accumulation) ===")

	// Create stream converter
	stream := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE).
		WithTicker(3 * time.Second).
		WithStart()

	// Start the conversion process
	fmt.Println("Stream started, simulating RTP packet reception...")

	// Simulate receiving RTP packets (20ms each at 16kHz = 320 bytes)
	packetDurationMs := 20
	numPackets := 50 // 1 second total

	start := time.Now()
	for i := 0; i < numPackets; i++ {
		// Simulate receiving an RTP packet
		rtpPacket := simulateRTPPacket(16000, packetDurationMs)

		// Write to stream converter
		_, err := stream.Write(rtpPacket)
		if err != nil {
			log.Fatalf("Failed to write packet %d: %v", i, err)
		}

		// Simulate real-time packet arrival
		time.Sleep(time.Millisecond * 5)
	}

	fmt.Printf("Received %d packets\n", numPackets)

	// Flush and get the complete FLAC output
	err := stream.Stop()
	if err != nil {
		log.Fatalf("Failed to flush stream: %v", err)
	}

	elapsed := time.Since(start)

	fmt.Printf("Stream conversion completed in %v\n", elapsed)

}

// Example 3: Real-world RTP-to-transcription worker pattern
func exampleRTPWorkerPattern() {
	fmt.Println("=== Example 3: RTP Worker Pattern for Transcription ===")

	const (
		maxAccumulationMs = 3000 // Send to transcription every 3 seconds
		packetDurationMs  = 20   // 20ms RTP packets
	)

	// Create stream converter
	stream := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE).
		WithTicker(3 * time.Second).
		WithStart()

	accumulatedMs := 0
	packetsProcessed := 0

	fmt.Println("Simulating continuous RTP stream with periodic transcription...")

	// Simulate continuous RTP reception
	for i := 0; i < 200; i++ { // Simulate 4 seconds of audio
		// Receive RTP packet
		rtpPacket := simulateRTPPacket(16000, packetDurationMs)

		// Write to converter
		_, err := stream.Write(rtpPacket)
		if err != nil {
			log.Fatalf("Write failed: %v", err)
		}

		packetsProcessed++
		accumulatedMs += packetDurationMs

		// Check if we've accumulated enough audio
		if accumulatedMs >= maxAccumulationMs {
			// Flush current stream and get FLAC data
			err := stream.Stop()
			if err != nil {
				log.Fatalf("End failed: %v", err)
			}

			// Reset for next batch
			stream = sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE).
				WithTicker(3 * time.Second).
				WithStart()

			accumulatedMs = 0
			packetsProcessed = 0
		}

		time.Sleep(time.Millisecond * 5)
	}

	fmt.Println("\nRTP worker pattern demonstration complete\n")
}

// Example 4: Using custom options
func exampleCustomOptions() {
	fmt.Println("=== Example 4: Custom Conversion Options ===")

	pcmData := simulateRTPPacket(8000, 1000)

	// Create converter with custom options
	converter := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE)

	// Configure options
	opts := sox.DefaultOptions()
	opts.CompressionLevel = 8       // Maximum FLAC compression
	opts.BufferSize = 64 * 1024     // 64KB buffer
	opts.Effects = []string{"norm"} // Normalize audio

	converter.WithOptions(opts)

	// Convert
	input := bytes.NewReader(pcmData)
	output := &bytes.Buffer{}

	start := time.Now()
	err := converter.Convert(input, output)
	elapsed := time.Since(start)

	if err != nil {
		log.Fatalf("Conversion failed: %v", err)
	}

	fmt.Printf("Converted with custom options in %v\n", elapsed)
	fmt.Printf("Output size: %d bytes (compression level 8)\n\n", output.Len())
}

// Example 5: Custom format (e.g., 8kHz telephony to FLAC)
func exampleCustomFormat() {
	fmt.Println("=== Example 5: Custom Audio Format (8kHz Telephony) ===")

	// Define custom format for 8kHz telephony audio
	pcmTelephony := sox.AudioFormat{
		Type:       "raw",
		Encoding:   "signed-integer",
		SampleRate: 8000,
		Channels:   1,
		BitDepth:   16,
	}

	wavTelephony := sox.AudioFormat{
		Type:       "wav",
		Encoding:   "little-endian",
		SampleRate: 8000,
		Channels:   1,
		BitDepth:   8,
	}

	// Generate 8kHz audio
	pcmData := simulateRTPPacket(8000, 1000)

	converter := sox.New(pcmTelephony, wavTelephony)

	input := bytes.NewReader(pcmData)
	output := &bytes.Buffer{}

	err := converter.Convert(input, output)
	if err != nil {
		log.Fatalf("Conversion failed: %v", err)
	}

	fmt.Printf("Converted 8kHz telephony audio: %d bytes → %d bytes\n\n",
		len(pcmData), output.Len())
}

func main() {
	// Check if SoX is installed
	if err := sox.CheckSoxInstalled(""); err != nil {
		log.Fatalf("SoX is not installed or not in PATH: %v\n"+
			"Please install SoX: https://sox.sourceforge.net/\n"+
			"  macOS: brew install sox\n"+
			"  Ubuntu/Debian: apt-get install sox\n", err)
	}

	fmt.Println("SoX Go Wrapper - RTP to FLAC Examples")
	fmt.Println("=====================================\n")

	// Run examples
	exampleBasicConversion()
	exampleStreamingConversion()
	exampleRTPWorkerPattern()
	exampleCustomOptions()
	exampleCustomFormat()

	fmt.Println("All examples completed successfully!")
}
