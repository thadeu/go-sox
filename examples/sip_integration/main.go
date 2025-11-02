package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	sox "github.com/thadeu/go-sox"
)

// TranscriptionAPI simulates an API client for Whisper/DeepInfra
type TranscriptionAPI struct {
	endpoint string
}

func (t *TranscriptionAPI) Transcribe(audioData []byte) (string, error) {
	// In production: send audioData to Whisper/DeepInfra
	// resp, err := http.Post(t.endpoint, "audio/flac", bytes.NewReader(audioData))
	return fmt.Sprintf("[Transcription of %d bytes]", len(audioData)), nil
}

// RTPMediaHandler handles incoming RTP media and converts to FLAC for transcription
type RTPMediaHandler struct {
	stream           *sox.Task
	transcriptionCh  chan []byte
	maxBufferMs      int
	accumulatedMs    int
	packetDurationMs int
	mu               sync.Mutex
}

// NewRTPMediaHandler creates a handler for RTP → FLAC → Transcription pipeline
func NewRTPMediaHandler(maxBufferMs, packetDurationMs int) *RTPMediaHandler {
	return &RTPMediaHandler{
		maxBufferMs:      maxBufferMs,
		packetDurationMs: packetDurationMs,
		transcriptionCh:  make(chan []byte, 10),
	}
}

// Start initializes the RTP handler
func (h *RTPMediaHandler) Start() error {
	// Initialize SoX stream converter: PCM Raw 16kHz mono → FLAC
	h.stream = sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE)

	// Configure for optimal performance
	opts := sox.DefaultOptions()
	opts.CompressionLevel = 5 // Balance between size and speed
	opts.BufferSize = 64 * 1024
	h.stream.WithOptions(opts)
	h.stream.WithTicker(3 * time.Second)

	h.stream.Start()
	return nil
}

// HandleRTPPacket processes incoming RTP packet with PCM audio
func (h *RTPMediaHandler) HandleRTPPacket(pcmData []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Write PCM data to SoX stream
	_, err := h.stream.Write(pcmData)
	if err != nil {
		return fmt.Errorf("failed to write RTP packet: %w", err)
	}

	h.accumulatedMs += h.packetDurationMs

	// Check if we've accumulated enough audio for transcription
	if h.accumulatedMs >= h.maxBufferMs {
		// Flush and get FLAC output
		err := h.stream.Stop()
		if err != nil {
			return fmt.Errorf("failed to flush stream: %w", err)
		}

		// Send to transcription worker
		select {
		default:
			log.Println("Warning: transcription queue full, dropping packet")
		}
	}

	return nil
}

// TranscriptionWorker processes FLAC chunks and sends to transcription API
func (h *RTPMediaHandler) TranscriptionWorker(api *TranscriptionAPI) {
	for flacData := range h.transcriptionCh {
		text, err := api.Transcribe(flacData)
		if err != nil {
			log.Printf("Transcription error: %v", err)
			continue
		}

		fmt.Printf("Transcription: %s\n", text)
	}
}

// Stop gracefully stops the handler
func (h *RTPMediaHandler) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Flush any remaining data
	if h.accumulatedMs > 0 {
		err := h.stream.Stop()
		if err != nil {
			return fmt.Errorf("failed to flush stream: %w", err)
		}
	}

	close(h.transcriptionCh)
	return h.stream.Stop()
}

// simulateRTPStream simulates receiving RTP packets for demonstration
func simulateRTPStream(sampleRate, durationMs int) []byte {
	numSamples := (sampleRate * durationMs) / 1000
	buffer := make([]byte, numSamples*2)
	for i := 0; i < numSamples; i++ {
		value := int16((i % 1000) * 32)
		buffer[i*2] = byte(value & 0xFF)
		buffer[i*2+1] = byte((value >> 8) & 0xFF)
	}
	return buffer
}

func main() {
	// Check SoX installation
	if err := sox.CheckSoxInstalled(""); err != nil {
		log.Fatalf("SoX not installed: %v", err)
	}

	fmt.Println("SIP/RTP Integration Example")
	fmt.Println("============================\n")

	// Create transcription API client
	api := &TranscriptionAPI{
		endpoint: "https://api.deepinfra.com/v1/inference/openai/whisper-large-v3",
	}

	// Create RTP handler
	// Accumulate 3 seconds of audio before sending to transcription
	handler := NewRTPMediaHandler(3000, 20) // 3s max buffer, 20ms packets

	// Start handler
	if err := handler.Start(); err != nil {
		log.Fatalf("Failed to start handler: %v", err)
	}

	// Start transcription worker
	go handler.TranscriptionWorker(api)

	// Simulate receiving RTP packets (in real app, these come from SIP stack)
	fmt.Println("Simulating RTP stream reception...")
	totalPackets := 300 // 6 seconds of audio (300 * 20ms)

	for i := 0; i < totalPackets; i++ {
		// Simulate receiving RTP packet with PCM data
		rtpPacket := simulateRTPStream(16000, 20) // 20ms at 16kHz

		// Process packet
		if err := handler.HandleRTPPacket(rtpPacket); err != nil {
			log.Printf("Error handling packet: %v", err)
		}

		// In real app, packets arrive at real-time rate
		// time.Sleep(20 * time.Millisecond)
	}

	fmt.Printf("Processed %d RTP packets\n\n", totalPackets)

	// Stop handler and flush remaining data
	if err := handler.Stop(); err != nil {
		log.Printf("Error stopping handler: %v", err)
	}

	fmt.Println("SIP/RTP integration example complete!")
}
