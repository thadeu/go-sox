package sox

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"
)

type Streamer struct {
	Input      AudioFormat
	Output     AudioFormat
	Options    ConversionOptions
	outputPath string
	started    bool
	closed     bool

	buffer     *bytes.Buffer
	bufferLock sync.Mutex

	ticker     *time.Ticker
	tickerStop chan struct{}
}

func NewStreamer(input, output AudioFormat) *Streamer {
	return &Streamer{
		Input:      input,
		Output:     output,
		Options:    DefaultOptions(),
		buffer:     &bytes.Buffer{},
		tickerStop: make(chan struct{}),
	}
}

func (s *Streamer) WithOutputPath(path string) *Streamer {
	s.outputPath = path
	return s
}

func (s *Streamer) WithOptions(options ConversionOptions) *Streamer {
	s.Options = options
	return s
}

// Write writes raw audio data to the buffer
func (s *Streamer) Write(data []byte) (int, error) {
	if !s.started {
		return 0, fmt.Errorf("stream converter not started")
	}

	if s.closed {
		return 0, fmt.Errorf("stream converter closed")
	}

	s.bufferLock.Lock()
	defer s.bufferLock.Unlock()

	return s.buffer.Write(data)
}

// Read reads data from the buffer
func (s *Streamer) Read(b []byte) (int, error) {
	s.bufferLock.Lock()
	defer s.bufferLock.Unlock()

	return s.buffer.Read(b)
}

// Start initializes the streamer with optional periodic flushing
// If interval > 0, starts a ticker that processes buffer at each interval
func (s *Streamer) Start(interval time.Duration) {
	if s.started {
		return
	}

	s.started = true
	s.closed = false

	if interval > 0 {
		s.ticker = time.NewTicker(interval)
		go s.runTicker()
	}
}

// runTicker processes the buffer whenever the ticker fires
func (s *Streamer) runTicker() {
	for {
		select {
		case <-s.ticker.C:
			s.bufferLock.Lock()
			if s.buffer.Len() > 0 {
				// Build command with current buffer content
				args := s.buildCommandArgs()
				args = append(args, s.outputPath)

				// Get SoX path
				soxPath := s.Options.SoxPath
				if soxPath == "" {
					soxPath = "sox"
				}

				// Copy buffer data
				inputData := make([]byte, s.buffer.Len())
				copy(inputData, s.buffer.Bytes())

				// Run command
				cmd := exec.Command(soxPath, args...)
				cmd.Stdin = bytes.NewReader(inputData)
				cmd.Stderr = os.Stderr

				if err := cmd.Run(); err != nil {
					log.Printf("Error converting packets: %v", err)
				}
			}
			s.bufferLock.Unlock()

		case <-s.tickerStop:
			return
		}
	}
}

// Stop stops the streamer and flushes remaining buffer
func (s *Streamer) Stop() error {
	if !s.started {
		return nil
	}

	if s.closed {
		return nil
	}

	s.closed = true
	s.started = false

	// Stop ticker
	if s.ticker != nil {
		s.ticker.Stop()
		close(s.tickerStop)
	}

	// Final flush
	return s.flush()
}

// End is alias for Stop
func (s *Streamer) End() error {
	return s.Stop()
}

// flush flushes the buffer to output file
func (s *Streamer) flush() error {
	s.bufferLock.Lock()
	defer s.bufferLock.Unlock()

	if s.buffer.Len() == 0 {
		return nil
	}

	// Determine output
	outputPath := s.outputPath
	if outputPath == "" {
		outputPath = "-"
	}

	// Build command
	args := s.buildCommandArgs()
	args = append(args, outputPath)

	// Get SoX path
	soxPath := s.Options.SoxPath
	if soxPath == "" {
		soxPath = "sox"
	}

	// Copy buffer data
	inputData := make([]byte, s.buffer.Len())
	copy(inputData, s.buffer.Bytes())

	// Run command
	cmd := exec.Command(soxPath, args...)
	cmd.Stdin = bytes.NewReader(inputData)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sox conversion failed: %w\nstderr: %s", err, stderr.String())
	}

	return nil
}

// buildCommandArgs constructs SoX command arguments
func (s *Streamer) buildCommandArgs() []string {
	args := []string{}

	// Global options
	args = append(args, s.Options.BuildGlobalArgs()...)

	// Input format arguments
	inputCopy := s.Input
	inputCopy.Pipe = false
	args = append(args, inputCopy.BuildArgs()...)

	// Input stdin
	args = append(args, "-")

	// Output format arguments
	outputCopy := s.Output
	outputCopy.Pipe = false
	args = append(args, outputCopy.BuildArgs()...)

	// Effects
	if effects := s.Options.buildEffectArgs(); len(effects) > 0 {
		args = append(args, effects...)
	}

	return args
}
