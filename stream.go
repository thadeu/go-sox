package sox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// StreamConverter handles streaming audio conversion by keeping a SoX process alive
// This is ideal for accumulating audio data (like RTP packets) and converting incrementally
type StreamConverter struct {
	Input   AudioFormat
	Output  AudioFormat
	Options ConversionOptions

	// Optional pool for concurrency control
	pool *Pool

	// Optional output path (if empty, uses stdout)
	outputPath string

	// Auto-flush configuration
	autoFlush         bool
	flushInterval     time.Duration
	flushTicker       *time.Ticker
	flushStopChan     chan bool
	flushWaitGroup    sync.WaitGroup // wait for autoFlushLoop to finish
	incrementalFlush  bool           // enable incremental flush mode
	outputFile        *os.File       // file handle for incremental writes
	outputFileLock    sync.Mutex     // protect file writes
	lastFlushPosition int            // track how many bytes already flushed incrementally

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	buffer     *bytes.Buffer
	bufferLock sync.Mutex

	readDone chan error
	started  bool
	closed   bool
	acquired bool
	mu       sync.Mutex
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

// WithPool enables pool-based concurrency control
func (s *StreamConverter) WithPool(pool ...*Pool) *StreamConverter {
	if len(pool) > 0 {
		s.pool = pool[0]
	} else {
		s.pool = NewPool() // Default pool
	}
	return s
}

// WithOutputPath sets the output file path (if empty, uses stdout)
func (s *StreamConverter) WithOutputPath(path string) *StreamConverter {
	s.outputPath = path
	return s
}

func (s *StreamConverter) WithAutoStart(ctx ...context.Context) *StreamConverter {
	s.Start(ctx...)

	return s
}

// WithAutoFlush enables automatic incremental flushing
// Requires WithOutputPath to be set - writes accumulated data to file every interval
// while keeping the SoX process running for continuous streaming
func (s *StreamConverter) WithAutoFlush(interval time.Duration) *StreamConverter {
	s.autoFlush = true
	s.flushInterval = interval

	return s
}

// WithOptions sets custom conversion options
func (s *StreamConverter) WithOptions(opts ConversionOptions) *StreamConverter {
	s.Options = opts
	return s
}

// releasePool releases the pool slot if acquired
func (s *StreamConverter) releasePool() {
	s.mu.Lock()
	if s.acquired && s.pool != nil {
		s.pool.Release()
		s.acquired = false
	}
	s.mu.Unlock()
}

// Start initializes and starts the SoX process
func (s *StreamConverter) Start(ctx ...context.Context) error {
	if s.started {
		return fmt.Errorf("stream converter already started")
	}

	// Acquire pool slot if using pool
	if s.pool != nil {
		var streamCtx context.Context

		if len(ctx) > 0 {
			streamCtx = ctx[0]
		} else {
			streamCtx = context.Background()
		}

		if err := s.pool.Acquire(streamCtx); err != nil {
			return fmt.Errorf("failed to acquire worker slot: %w", err)
		}

		s.mu.Lock()
		s.acquired = true
		s.mu.Unlock()
	}

	// Validate formats
	if err := s.Input.Validate(); err != nil {
		s.releasePool()
		return fmt.Errorf("invalid input format: %w", err)
	}

	if err := s.Output.Validate(); err != nil {
		s.releasePool()
		return fmt.Errorf("invalid output format: %w", err)
	}

	// Build SoX command
	args := s.buildCommandArgs()
	s.cmd = exec.Command(s.Options.SoxPath, args...)

	// Set up pipes
	var err error
	s.stdin, err = s.cmd.StdinPipe()
	if err != nil {
		s.releasePool()
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	// ALWAYS use stdout pipe to accumulate data in buffer
	s.stdout, err = s.cmd.StdoutPipe()
	if err != nil {
		s.releasePool()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	s.stderr, err = s.cmd.StderrPipe()
	if err != nil {
		s.releasePool()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// If auto-flush is enabled, use incremental mode (requires output path)
	if s.autoFlush {
		if s.outputPath == "" {
			s.releasePool()
			return fmt.Errorf("WithAutoFlush requires WithOutputPath to be set")
		}

		s.incrementalFlush = true

		// Open file for incremental writing
		var err error
		s.outputFile, err = os.OpenFile(s.outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			s.releasePool()
			return fmt.Errorf("failed to open output file: %w", err)
		}
	}

	// Start the command
	if err := s.cmd.Start(); err != nil {
		s.releasePool()
		return fmt.Errorf("failed to start sox: %w", err)
	}

	// Track process
	GetMonitor().TrackProcess(s.cmd.Process.Pid)

	// ALWAYS read stdout to accumulate in buffer
	s.readDone = make(chan error, 1)
	go s.readOutput()

	// Start auto-flush ticker if enabled
	if s.autoFlush && s.flushInterval > 0 {
		s.flushStopChan = make(chan bool)
		s.flushTicker = time.NewTicker(s.flushInterval)
		s.flushWaitGroup.Add(1)
		go s.autoFlushLoop()
	}

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

// autoFlushLoop runs in a goroutine and performs incremental flush periodically
func (s *StreamConverter) autoFlushLoop() {
	defer s.flushWaitGroup.Done() // Signal completion when goroutine exits

	for {
		select {
		case <-s.flushTicker.C:
			// Incremental flush: write available data without closing stream
			written, err := s.writeAvailableData()
			if err != nil {
				// Log error but continue streaming
				fmt.Fprintf(os.Stderr, "Incremental flush error: %v\n", err)
			} else if written > 0 {
				// Optional: track metrics or log
				_ = written
			}
			// Continue loop - don't return

		case <-s.flushStopChan:
			s.flushTicker.Stop()
			return
		}
	}
}

// stopAutoFlush stops the auto-flush ticker and waits for completion
func (s *StreamConverter) stopAutoFlush() {
	if s.autoFlush && s.flushStopChan != nil {
		// Stop the ticker first to prevent new ticks
		if s.flushTicker != nil {
			s.flushTicker.Stop()
		}
		// Signal the goroutine to stop
		close(s.flushStopChan)
		s.flushStopChan = nil

		// Wait for the goroutine to finish completely
		s.flushWaitGroup.Wait()
	}
}

// writeAvailableData writes all available buffered data to output file
// without closing the SoX process. Returns bytes written.
func (s *StreamConverter) writeAvailableData() (int, error) {
	// Check if stream is closed or output file is not available
	s.mu.Lock()
	if s.closed || s.outputFile == nil {
		s.mu.Unlock()
		return 0, nil // Don't write if closed or no file
	}
	s.mu.Unlock()

	s.bufferLock.Lock()
	bufferData := s.buffer.Bytes()
	currentSize := len(bufferData)

	// Only write new data since last flush
	if currentSize <= s.lastFlushPosition {
		s.bufferLock.Unlock()
		return 0, nil
	}

	// Get new data since last flush
	newData := bufferData[s.lastFlushPosition:]
	s.bufferLock.Unlock()

	if len(newData) == 0 {
		return 0, nil
	}

	s.outputFileLock.Lock()
	// Double-check if file is still available after acquiring lock
	if s.outputFile == nil {
		s.outputFileLock.Unlock()
		return 0, nil
	}

	written, err := s.outputFile.Write(newData)
	if err != nil {
		s.outputFileLock.Unlock()
		return 0, fmt.Errorf("failed to write to output file: %w", err)
	}

	// Sync to ensure data is flushed to disk
	if err := s.outputFile.Sync(); err != nil {
		s.outputFileLock.Unlock()
		return written, fmt.Errorf("failed to sync output file: %w", err)
	}
	s.outputFileLock.Unlock()

	// Update checkpoint
	s.lastFlushPosition += written

	return written, nil
}

// Flush closes the input, waits for conversion to complete, and returns all output
func (s *StreamConverter) Flush() ([]byte, error) {
	if !s.started {
		return nil, fmt.Errorf("stream converter not started")
	}
	if s.closed {
		return nil, fmt.Errorf("stream converter already closed")
	}

	// Stop auto-flush ticker if running
	s.stopAutoFlush()

	// In incremental mode, write any remaining data first
	if s.incrementalFlush && s.outputFile != nil {
		s.writeAvailableData()
	}

	// Close stdin to signal end of input
	if err := s.stdin.Close(); err != nil {
		s.releasePool()
		return nil, fmt.Errorf("failed to close stdin: %w", err)
	}

	// Wait for reading to complete
	readErr := <-s.readDone

	// Wait for process to exit
	if err := s.cmd.Wait(); err != nil {
		stderrData, _ := io.ReadAll(s.stderr)
		GetMonitor().RecordFailure()
		s.releasePool()
		return nil, fmt.Errorf("sox process failed: %w\nstderr: %s", err, string(stderrData))
	}

	// Untrack process
	if s.cmd.Process != nil {
		GetMonitor().UntrackProcess(s.cmd.Process.Pid)
	}

	if readErr != nil && readErr != io.EOF {
		s.releasePool()
		return nil, fmt.Errorf("error reading output: %w", readErr)
	}

	s.closed = true
	s.releasePool()

	// Get all buffered data
	s.bufferLock.Lock()
	data := s.buffer.Bytes()
	s.bufferLock.Unlock()

	// If in incremental mode, write final data and close file
	if s.incrementalFlush && s.outputFile != nil {
		s.outputFileLock.Lock()
		// Write any remaining data since last incremental flush
		if len(data) > s.lastFlushPosition {
			finalData := data[s.lastFlushPosition:]
			s.outputFile.Write(finalData)
		}
		s.outputFile.Sync()
		s.outputFile.Close()
		s.outputFileLock.Unlock()
		return nil, nil
	}

	// If output path is set, write accumulated buffer to file
	if s.outputPath != "" {
		if err := os.WriteFile(s.outputPath, data, 0644); err != nil {
			return nil, fmt.Errorf("failed to write output file: %w", err)
		}
		return nil, nil // No data to return when writing to file
	}

	// Return all buffered data
	return data, nil
}

// Close closes the stream converter and terminates the SoX process
func (s *StreamConverter) Close() error {
	if !s.started {
		return nil
	}
	if s.closed {
		return nil
	}

	// Stop auto-flush ticker if running
	s.stopAutoFlush()

	// If using incremental flush, write any remaining data before closing stdin
	if s.incrementalFlush && s.outputFile != nil {
		s.writeAvailableData()
	}

	// Close stdin to signal end of input
	if s.stdin != nil {
		_ = s.stdin.Close()
	}

	// Wait for reading to complete
	if s.readDone != nil {
		<-s.readDone
	}

	// Wait for process to exit gracefully
	if s.cmd != nil && s.cmd.Process != nil {
		// Try to wait for graceful exit first
		done := make(chan error, 1)
		go func() {
			done <- s.cmd.Wait()
		}()

		// Wait up to 5 seconds for graceful exit
		select {
		case <-done:
			// Process exited gracefully
		case <-time.After(5 * time.Second):
			// Timeout - force kill
			s.cmd.Process.Kill()
			<-done // Wait for Wait() to return after Kill
		}

		GetMonitor().UntrackProcess(s.cmd.Process.Pid)
	}

	// NOW close the output file after process is completely done
	if s.outputFile != nil {
		s.outputFileLock.Lock()
		// Write any remaining data from buffer if in incremental mode
		if s.incrementalFlush {
			s.bufferLock.Lock()
			data := s.buffer.Bytes()
			if len(data) > s.lastFlushPosition {
				finalData := data[s.lastFlushPosition:]
				s.outputFile.Write(finalData)
			}
			s.bufferLock.Unlock()
		}
		s.outputFile.Sync()
		s.outputFile.Close()
		s.outputFile = nil // Clear the file handle
		s.outputFileLock.Unlock()

		// Fix WAV headers if using WAV format
		if s.Output.Type == "wav" {
			err := s.fixWAVHeaders(s.outputPath)
			if err != nil {
				// Log error but don't fail the close operation
				fmt.Fprintf(os.Stderr, "Warning: failed to fix WAV headers: %v\n", err)
			}
		}
	}

	s.closed = true
	s.releasePool()
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
	args = append(args, s.Options.BuildGlobalArgs()...)

	// Input format arguments
	args = append(args, s.Input.BuildArgs(true)...)

	// Input file (stdin)
	args = append(args, "-")

	// Output format arguments
	args = append(args, s.Output.BuildArgs(false)...)

	// Format-specific arguments for output
	args = append(args, s.Options.buildFormatArgs(&s.Output)...)

	// ALWAYS use stdout to accumulate in buffer
	// File writing happens in Flush() method
	args = append(args, "-")

	// Effects
	if effects := s.Options.buildEffectArgs(); len(effects) > 0 {
		args = append(args, effects...)
	}

	return args
}

// fixWAVHeaders corrects WAV file headers using SoX to ensure proper duration
func (s *StreamConverter) fixWAVHeaders(filePath string) error {
	// Create temporary file for corrected WAV
	tempPath := filePath + ".tmp"

	// Use SoX to rewrite the file with correct headers
	converter := NewConverter(s.Input, s.Output)
	err := converter.ConvertFile(filePath, tempPath)
	if err != nil {
		// Clean up temp file if conversion failed
		os.Remove(tempPath)
		return fmt.Errorf("failed to fix WAV headers: %w", err)
	}

	// Replace original file with corrected one
	err = os.Rename(tempPath, filePath)
	if err != nil {
		// Clean up temp file if rename failed
		os.Remove(tempPath)
		return fmt.Errorf("failed to replace WAV file: %w", err)
	}

	return nil
}
