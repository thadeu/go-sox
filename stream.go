package sox

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// StreamConverter handles streaming audio conversion by keeping a SoX process alive
// This is ideal for accumulating audio data (like RTP packets) and converting incrementally
type StreamConverter struct {
	Input   AudioFormat
	Output  AudioFormat
	Options ConversionOptions

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	buffer     *bytes.Buffer
	bufferLock sync.Mutex

	readDone chan error
	started  bool
	closed   bool
}

// NewStreamConverter creates a new StreamConverter
func NewStreamConverter(input, output AudioFormat) *StreamConverter {
	return &StreamConverter{
		Input:   input,
		Output:  output,
		Options: DefaultOptions(),
		buffer:  &bytes.Buffer{},
	}
}

// WithOptions sets custom conversion options
func (s *StreamConverter) WithOptions(opts ConversionOptions) *StreamConverter {
	s.Options = opts
	return s
}

// Start initializes and starts the SoX process
func (s *StreamConverter) Start() error {
	if s.started {
		return fmt.Errorf("stream converter already started")
	}

	// Validate formats
	if err := s.Input.Validate(); err != nil {
		return fmt.Errorf("invalid input format: %w", err)
	}
	if err := s.Output.Validate(); err != nil {
		return fmt.Errorf("invalid output format: %w", err)
	}

	// Build SoX command
	args := s.buildCommandArgs()
	s.cmd = exec.Command(s.Options.SoxPath, args...)

	// Set up pipes
	var err error
	s.stdin, err = s.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	s.stdout, err = s.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	s.stderr, err = s.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start sox: %w", err)
	}

	// Track process
	GetMonitor().TrackProcess(s.cmd.Process.Pid)

	// Start goroutine to read stdout continuously
	s.readDone = make(chan error, 1)
	go s.readOutput()

	s.started = true
	return nil
}

// Write writes raw audio data to the SoX process
// The data will be converted according to the configured formats
func (s *StreamConverter) Write(data []byte) (int, error) {
	if !s.started {
		return 0, fmt.Errorf("stream converter not started")
	}
	if s.closed {
		return 0, fmt.Errorf("stream converter closed")
	}

	return s.stdin.Write(data)
}

// Read reads converted audio data from the buffer
func (s *StreamConverter) Read(p []byte) (int, error) {
	s.bufferLock.Lock()
	defer s.bufferLock.Unlock()

	return s.buffer.Read(p)
}

// Available returns the number of bytes available to read
func (s *StreamConverter) Available() int {
	s.bufferLock.Lock()
	defer s.bufferLock.Unlock()

	return s.buffer.Len()
}

// Flush closes the input, waits for conversion to complete, and returns all output
func (s *StreamConverter) Flush() ([]byte, error) {
	if !s.started {
		return nil, fmt.Errorf("stream converter not started")
	}
	if s.closed {
		return nil, fmt.Errorf("stream converter already closed")
	}

	// Close stdin to signal end of input
	if err := s.stdin.Close(); err != nil {
		return nil, fmt.Errorf("failed to close stdin: %w", err)
	}

	// Wait for reading to complete
	readErr := <-s.readDone

	// Wait for process to exit
	if err := s.cmd.Wait(); err != nil {
		stderrData, _ := io.ReadAll(s.stderr)
		GetMonitor().RecordFailure()
		return nil, fmt.Errorf("sox process failed: %w\nstderr: %s", err, string(stderrData))
	}

	// Untrack process
	if s.cmd.Process != nil {
		GetMonitor().UntrackProcess(s.cmd.Process.Pid)
	}

	if readErr != nil && readErr != io.EOF {
		return nil, fmt.Errorf("error reading output: %w", readErr)
	}

	s.closed = true

	// Return all buffered data
	s.bufferLock.Lock()
	defer s.bufferLock.Unlock()

	return s.buffer.Bytes(), nil
}

// Close closes the stream converter and terminates the SoX process
func (s *StreamConverter) Close() error {
	if !s.started {
		return nil
	}
	if s.closed {
		return nil
	}

	// Try to close stdin gracefully
	if s.stdin != nil {
		_ = s.stdin.Close()
	}

	// Kill the process if still running
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		GetMonitor().UntrackProcess(s.cmd.Process.Pid)
	}

	s.closed = true
	return nil
}

// readOutput continuously reads from stdout and buffers it
func (s *StreamConverter) readOutput() {
	buf := make([]byte, s.Options.BufferSize)
	for {
		n, err := s.stdout.Read(buf)
		if n > 0 {
			s.bufferLock.Lock()
			s.buffer.Write(buf[:n])
			s.bufferLock.Unlock()
		}
		if err != nil {
			s.readDone <- err
			return
		}
	}
}

// buildCommandArgs constructs the SoX command arguments for streaming
func (s *StreamConverter) buildCommandArgs() []string {
	args := []string{}

	// Global options
	args = append(args, s.Options.buildGlobalArgs()...)

	// Input format arguments
	args = append(args, s.Input.buildArgs(true)...)

	// Input file (stdin)
	args = append(args, "-")

	// Output format arguments
	args = append(args, s.Output.buildArgs(false)...)

	// Format-specific arguments for output
	args = append(args, s.Options.buildFormatArgs(&s.Output)...)

	// Output file (stdout)
	args = append(args, "-")

	// Effects
	if effects := s.Options.buildEffectArgs(); len(effects) > 0 {
		args = append(args, effects...)
	}

	return args
}
