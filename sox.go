// Copyright (c) 2025 Thadeu Esteves Jr
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
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

// Task handles audio format conversion using SoX with built-in resiliency.
// It provides a unified API for one-shot conversions, real-time streaming,
// and periodic batch processing.
//
// The Task includes production-ready features by default:
//   - Circuit breaker pattern to prevent cascading failures
//   - Automatic retry with exponential backoff
//   - Context support for cancellation and timeouts
//
// Example:
//
//	// Simple conversion
//	task := New(PCM_RAW_16K_MONO, FLAC_16K_MONO)
//	err := task.Convert(inputReader, outputWriter)
//
//	// With options
//	task := New(input, output).
//		WithOptions(opts).
//		WithRetryConfig(retryConfig)
//	err := task.Convert(inputPath, outputPath)
type Task struct {
	Input          AudioFormat
	Output         AudioFormat
	Options        ConversionOptions
	circuitBreaker *CircuitBreaker
	retryConfig    RetryConfig

	// Streaming state
	streamMode       bool
	streamBuffer     *bytes.Buffer
	streamLock       sync.Mutex
	streamStarted    bool
	streamClosed     bool
	streamCmd        *exec.Cmd
	streamStdin      io.WriteCloser
	streamStdout     io.ReadCloser
	streamOutput     *bytes.Buffer
	streamOutputDone chan error

	// Ticker state
	tickerMode     bool
	ticker         *time.Ticker
	tickerDuration time.Duration
	tickerStop     chan struct{}
	tickerBuffer   *bytes.Buffer
	tickerLock     sync.Mutex

	outputPath string

	// Path mode (direct file handling, no piping)
	pathMode  bool
	inputPath string
}

// New creates a new Task with input and output formats.
// The Task is configured with production-ready defaults:
//   - Circuit breaker (opens after 5 failures, resets after 10s)
//   - Retry (3 attempts with exponential backoff)
//   - Default SoX options
//
// Example:
//
//	task := New(PCM_RAW_16K_MONO, FLAC_16K_MONO)
//	err := task.Convert(inputReader, outputWriter)
//
//	task := New(input, output).
//		WithOptions(customOptions).
//		WithCircuitBreaker(circuitBreaker)
//	err := task.Convert(inputPath, outputPath)
func New(args ...interface{}) *Task {
	var input *AudioFormat
	var output *AudioFormat

	input = &PCM_RAW_8K_MONO
	output = &PCM_RAW_8K_MONO

	if len(args) == 1 {
		if ptr := toAudioFormatPtr(args[0]); ptr != nil {
			output = ptr
		}
	} else if len(args) == 2 {
		if ptr := toAudioFormatPtr(args[0]); ptr != nil {
			input = ptr
		}
		if ptr := toAudioFormatPtr(args[1]); ptr != nil {
			output = ptr
		}
	}

	return &Task{
		Input:          *input,
		Output:         *output,
		Options:        DefaultOptions(),
		circuitBreaker: NewCircuitBreaker(),
		retryConfig:    DefaultRetryConfig(),
		streamBuffer:   &bytes.Buffer{},
		tickerBuffer:   &bytes.Buffer{},
		tickerStop:     make(chan struct{}),
	}
}

// Convert performs a one-time audio conversion without needing to instantiate sox.New.
// It automatically detects the input format:
//   - wav, flac, and mp3: auto-detected by sox (no -t flag needed)
//   - Other formats: defaults to raw type (-t raw)
//
// The output format is specified via the options parameter.
//
// Example:
//
//	wavBuffer := &bytes.Buffer{}
//	err := sox.Convert(pcmPath, wavBuffer, sox.Options{
//		Type:       "wav",
//		Encoding:   "signed-integer",
//		Endian:     "little",
//		SampleRate: 16000,
//		BitDepth:   16,
//		Channels:   1,
//		Compression: 1.0,
//		IgnoreLength: false,
//		CustomArgs: []string{"--add-comment", "Custom metadata", "--norm"},
//	})
//
//	// Convert from wav file (auto-detected) to flac
//	err := sox.Convert("input.wav", "output.flac", sox.Options{
//		Type: "flac",
//	})
func Convert(input interface{}, output interface{}, options Options) error {
	// Create task with detected input format and provided output format
	task := New(toFormatType(input), &options)

	// Perform conversion
	return task.Convert(input, output)
}

// NewTicker creates a new Task configured for periodic batch processing.
// The ticker mode buffers input data and converts it at the specified interval.
//
// Example:
//
//	task := NewTicker(PCM_RAW_8K_MONO, FLAC_16K_MONO, 3*time.Second).
//		WithOutputPath("/tmp/output.flac")
//	task.Start()
//
//	for packet := range rtpChannel {
//		task.Write(packet.Payload)
//	}
//	task.Stop()
func NewTicker(input AudioFormat, output AudioFormat, interval time.Duration) *Task {
	conv := New(input, output)
	conv.WithTicker(interval)

	return conv
}

// NewStream creates a new Task configured for real-time streaming.
// In streaming mode, Write() sends data to the SoX process and Read() receives
// converted data. The SoX process remains alive during the stream.
//
// Example:
//
//	task := NewStream(PCM_RAW_8K_MONO, FLAC_16K_MONO).WithStream()
//	task.Start()
//
//	for packet := range rtpChannel {
//		task.Write(packet.Payload)
//		// Read converted data if needed
//		buf := make([]byte, 4096)
//		task.Read(buf)
//	}
//	task.Stop()
func NewStream(input AudioFormat, output AudioFormat) *Task {
	conv := New(input, output)
	conv.WithStream()

	return conv
}

// WithOptions sets custom conversion options for the Task.
// Options include timeout, buffer size, compression level, effects, and more.
//
// Example:
//
//	opts := DefaultOptions()
//	opts.Timeout = 30 * time.Second
//	opts.CompressionLevel = 8
//	task := New(input, output).WithOptions(opts)
func (c *Task) WithOptions(opts ConversionOptions) *Task {
	c.Options = opts
	return c
}

// WithCircuitBreaker sets a custom circuit breaker for the Task.
// By default, a circuit breaker is created with sensible defaults.
// Override this for custom failure thresholds and reset timeouts.
//
// Example:
//
//	breaker := NewCircuitBreakerWithConfig(
//		10,                 // maxFailures
//		15*time.Second,     // resetTimeout
//		5,                  // halfOpenRequests
//	)
//	task := New(input, output).WithCircuitBreaker(breaker)
func (c *Task) WithCircuitBreaker(cb *CircuitBreaker) *Task {
	c.circuitBreaker = cb
	return c
}

// WithRetryConfig sets custom retry configuration for the Task.
// By default, the Task uses DefaultRetryConfig() (3 attempts, exponential backoff).
//
// Example:
//
//	retryConfig := RetryConfig{
//		MaxAttempts:     5,
//		InitialBackoff:  200 * time.Millisecond,
//		MaxBackoff:      10 * time.Second,
//		BackoffMultiple: 2.0,
//	}
//	task := New(input, output).WithRetryConfig(retryConfig)
func (c *Task) WithRetryConfig(config RetryConfig) *Task {
	c.retryConfig = config
	return c
}

// DisableResilience disables circuit breaker and retry mechanisms.
// This reduces latency but removes protection against transient failures.
// Not recommended for production use unless you handle resiliency externally.
//
// Example:
//
//	// For testing or non-critical conversions
//	task := New(input, output).DisableResilience()
func (c *Task) DisableResilience() *Task {
	c.circuitBreaker = nil
	c.retryConfig.MaxAttempts = 1
	return c
}

// WithStream enables streaming mode for real-time data processing.
// In streaming mode, the SoX process remains alive and accepts continuous writes.
// Use Write() to send data and Read() to receive converted data.
//
// Example:
//
//	task := New(input, output).WithStream()
//	task.Start()
//	defer task.Stop()
//
//	for packet := range rtpChannel {
//		task.Write(packet.Payload)
//	}
func (c *Task) WithStream() *Task {
	c.streamMode = true
	return c
}

// WithTicker enables periodic conversion with the specified interval.
// Data written via Write() is buffered and converted at each tick.
// Useful for batch processing of continuous streams (e.g., RTP recording).
//
// Example:
//
//	task := New(input, output).
//		WithTicker(5*time.Second).
//		WithOutputPath("/tmp/output.flac")
//	task.Start()
//
//	for packet := range rtpChannel {
//		task.Write(packet.Payload)
//	}
//	task.Stop() // Final flush
func (c *Task) WithTicker(interval time.Duration) *Task {
	c.tickerMode = true
	c.tickerDuration = interval
	return c
}

// WithOutputPath sets the output file path for conversions.
// Used with ticker mode or stream mode to write directly to a file.
//
// Example:
//
//	task := New(input, output).
//		WithOutputPath("/tmp/recording.flac").
//		WithTicker(3*time.Second)
func (s *Task) WithOutputPath(path string) *Task {
	s.outputPath = path
	return s
}

// WithStart starts the Task immediately after configuration.
// Convenience method for chaining: New(...).WithStream().WithStart()
//
// Example:
//
//	task := New(input, output).
//		WithStream().
//		WithStart()
//	defer task.Stop()
func (s *Task) WithStart() *Task {
	s.Start()

	return s
}

// Convert performs audio format conversion with flexible argument handling.
// It automatically detects the input/output types and uses the most efficient method.
//
// Supported argument combinations:
//   - Convert(io.Reader, io.Writer) - uses pipes for streaming I/O
//   - Convert(string, string) - uses direct file paths (optimized, no pipes)
//   - Convert(io.Reader, string) - reads from stream, writes to file
//   - Convert(string, io.Writer) - reads from file, writes to stream
//
// The conversion respects Options.Timeout if set, otherwise uses context.Background().
//
// Example:
//
//	// Bytes to bytes
//	task := New(PCM_RAW_8K_MONO, FLAC_16K_MONO)
//	err := task.Convert(bytes.NewReader(pcmData), &outputBuffer)
//
//	// File to file (optimized path mode)
//	err := task.Convert("input.pcm", "output.flac")
//
//	// With timeout via options
//	task := New(input, output)
//	task.Options.Timeout = 10 * time.Second
//	err := task.Convert(inputReader, outputWriter)
func (c *Task) Convert(args ...interface{}) error {
	ctx := context.Background()
	if c.Options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Options.Timeout)
		defer cancel()
	}
	return c.ConvertWithContext(ctx, args...)
}

// ConvertWithContext performs conversion with a context for cancellation and timeout.
// The context is used for cancellation propagation and timeout enforcement.
// This is the preferred method when you need explicit control over cancellation.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
//	defer cancel()
//
//	task := New(input, output)
//	err := task.ConvertWithContext(ctx, inputReader, outputWriter)
//
//	// Cancellation example
//	ctx, cancel := context.WithCancel(context.Background())
//	go func() {
//		time.Sleep(5 * time.Second)
//		cancel() // Cancel conversion after 5 seconds
//	}()
//	err := task.ConvertWithContext(ctx, inputReader, outputWriter)
func (c *Task) ConvertWithContext(ctx context.Context, args ...interface{}) error {
	if len(args) < 2 {
		return fmt.Errorf("convert requires at least 2 arguments (input and output)")
	}

	input := args[0]
	output := args[1]

	// Check if this is path-based conversion (optimize by avoiding piping)
	if inputPath, ok := input.(string); ok {
		if outputPath, ok := output.(string); ok {
			// Both are paths - use direct file mode (no piping)
			c.pathMode = true
			c.inputPath = inputPath
			c.outputPath = outputPath
			return c.executeWithRetry(ctx, inputPath, outputPath)
		}
	}

	// Stream-based conversion (using readers/writers)
	// Detect input type
	var inputReader io.Reader
	switch v := input.(type) {
	case io.Reader:
		inputReader = v
	case string:
		file, err := os.Open(v)
		if err != nil {
			return fmt.Errorf("failed to open input file: %w", err)
		}
		defer file.Close()
		inputReader = file
	default:
		return fmt.Errorf("input must be io.Reader or string (file path), got %T", input)
	}

	// Detect output type
	var outputWriter io.Writer
	switch v := output.(type) {
	case io.Writer:
		outputWriter = v
	case string:
		file, err := os.Create(v)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer file.Close()
		outputWriter = file
	default:
		return fmt.Errorf("output must be io.Writer or string (file path), got %T", output)
	}

	// Convert input to ReadSeeker for retry support
	var seekableInput io.ReadSeeker
	if seeker, ok := inputReader.(io.ReadSeeker); ok {
		seekableInput = seeker
	} else {
		data, err := io.ReadAll(inputReader)
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		seekableInput = newBytesReader(data)
	}

	// Execute with retry and circuit breaker (stream-based)
	return c.executeWithRetryStream(ctx, seekableInput, outputWriter)
}

// Write writes audio data to the Task.
// Valid only when using WithStream() or WithTicker() mode.
// For stream mode, data is sent directly to the SoX process stdin.
// For ticker mode, data is buffered until the next tick interval.
//
// Example:
//
//	task := New(input, output).WithStream()
//	task.Start()
//	defer task.Stop()
//
//	for packet := range rtpChannel {
//		if _, err := task.Write(packet.Payload); err != nil {
//			return err
//		}
//	}
func (c *Task) Write(data []byte) (int, error) {
	if c.tickerMode {
		c.tickerLock.Lock()
		defer c.tickerLock.Unlock()

		return c.tickerBuffer.Write(data)
	}

	if !c.streamMode {
		return 0, fmt.Errorf("write only available in stream or ticker mode")
	}

	if !c.streamStarted {
		return 0, fmt.Errorf("stream not started, call Start() first")
	}

	if c.streamClosed {
		return 0, fmt.Errorf("stream is closed")
	}

	if c.streamStdin == nil {
		return 0, fmt.Errorf("stream stdin not initialized")
	}

	c.streamLock.Lock()
	defer c.streamLock.Unlock()
	c.streamBuffer.Write(data)

	return c.streamStdin.Write(data)
}

// Read reads converted audio data from the Task.
// Valid only when using WithStream() mode.
// Reads from the SoX process stdout pipe.
//
// Example:
//
//	task := New(input, output).WithStream()
//	task.Start()
//	defer task.Stop()
//
//	buf := make([]byte, 4096)
//	for {
//		n, err := task.Read(buf)
//		if err == io.EOF {
//			break
//		}
//		if err != nil {
//			return err
//		}
//		// Process buf[:n]
//	}
func (c *Task) Read(b []byte) (int, error) {
	if !c.streamMode {
		return 0, fmt.Errorf("read only available in stream mode")
	}

	if !c.streamStarted {
		return 0, fmt.Errorf("stream not started, call Start() first")
	}

	c.streamLock.Lock()
	defer c.streamLock.Unlock()

	return c.streamStdout.Read(b)
}

// Start initializes the Task for streaming or ticker mode.
// For streaming mode: starts the SoX process with stdin/stdout pipes.
// For ticker mode: starts the periodic conversion ticker.
//
// Must be called before Write() or Read().
//
// Example:
//
//	// Stream mode
//	task := New(input, output).WithStream()
//	if err := task.Start(); err != nil {
//		return err
//	}
//	defer task.Stop()
//
//	// Ticker mode
//	task := New(input, output).
//		WithTicker(3*time.Second).
//		WithOutputPath("/tmp/output.flac")
//	if err := task.Start(); err != nil {
//		return err
//	}
//	defer task.Stop()
func (c *Task) Start() error {
	if c.tickerMode {
		return c.runTicker()
	}

	if !c.streamMode {
		return fmt.Errorf("start only available in stream or ticker mode")
	}

	if c.streamStarted {
		return fmt.Errorf("stream already started")
	}

	c.streamStarted = true
	c.streamClosed = false
	c.streamOutput = &bytes.Buffer{}
	c.streamOutputDone = make(chan error, 1)

	// Build command arguments
	args := c.buildCommandArgs()

	// Create command
	cmd := exec.Command(c.Options.SoxPath, args...)

	// Set up pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	c.streamStdin = stdin

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	c.streamStdout = stdout

	// Capture stderr
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start sox: %w", err)
	}

	c.streamCmd = cmd

	// Start goroutine to continuously read stdout
	// For RAW format with outputPath in stream mode, write to file in append mode
	// Otherwise, buffer output in memory
	if c.outputPath != "" && c.Output.Type == TYPE_RAW {
		// Stream mode with outputPath and RAW format: read from stdout and append to file
		// RAW format doesn't have headers, so we can safely append chunks
		go func() {
			file, err := os.OpenFile(c.outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				c.streamOutputDone <- fmt.Errorf("failed to open output file: %w", err)
				return
			}
			defer file.Close()

			_, err = io.Copy(file, stdout)
			c.streamOutputDone <- err
		}()
	} else {
		// No outputPath or non-RAW format: buffer output in memory
		// Formats with headers (FLAC, WAV) are written directly by sox to the file
		go func() {
			_, err := io.Copy(c.streamOutput, stdout)
			c.streamOutputDone <- err
		}()
	}

	return nil
}

// runTicker initializes the ticker-based conversion
func (c *Task) runTicker() error {
	if c.tickerDuration <= 0 {
		return fmt.Errorf("ticker duration must be positive")
	}

	c.ticker = time.NewTicker(c.tickerDuration)

	go func() {
		for {
			select {
			case <-c.ticker.C:
				c.tickerLock.Lock()
				if c.tickerBuffer.Len() > 0 {
					_ = c.flushTickerBuffer()
				}
				c.tickerLock.Unlock()
			case <-c.tickerStop:
				return
			}
		}
	}()

	return nil
}

// flushTickerBuffer converts buffered data (assumes lock is held)
func (c *Task) flushTickerBuffer() error {
	if c.tickerBuffer.Len() == 0 {
		return nil
	}

	// Create a copy of buffer data
	inputData := make([]byte, c.tickerBuffer.Len())
	copy(inputData, c.tickerBuffer.Bytes())

	// Reset buffer after copying to avoid duplicate processing
	// c.tickerBuffer.Reset()

	// Run conversion on copied data
	ctx := context.Background()

	if c.Options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Options.Timeout)

		defer cancel()
	}

	inputReader := newBytesReader(inputData)
	outputBuffer := &bytes.Buffer{}

	return c.convertInternal(ctx, inputReader, outputBuffer)
}

// flushStreamBuffer writes the buffered stream data to the output path
func (c *Task) flushStreamBuffer() error {
	c.streamLock.Lock()
	defer c.streamLock.Unlock()

	if c.streamOutput == nil {
		return fmt.Errorf("stream output not initialized")
	}

	// Read all remaining data from stdout (already converted by sox)
	outputData, err := io.ReadAll(c.streamOutput)

	if err != nil {
		return fmt.Errorf("failed to read stream output: %w", err)
	}

	if len(outputData) == 0 {
		return nil
	}

	// Use Convert to ensure proper file format with headers
	// This guarantees sox will create the file correctly with proper headers (WAV, FLAC, etc)
	inputReader := bytes.NewReader(outputData)
	return c.Convert(inputReader, c.outputPath)
}

// Stop stops the Task and closes all resources.
// For streaming mode: closes stdin pipe and waits for SoX process to finish.
// For ticker mode: stops the ticker and performs final flush of buffered data.
//
// Always call Stop() to ensure proper cleanup, preferably with defer.
//
// Example:
//
//	task := New(input, output).WithStream()
//	if err := task.Start(); err != nil {
//		return err
//	}
//	defer task.Stop() // Ensures cleanup
//
//	// Use task...
func (c *Task) Stop() error {
	if c.tickerMode {
		return c.stopTicker()
	}

	if !c.streamMode {
		return nil
	}

	if !c.streamStarted || c.streamClosed {
		return nil
	}

	c.streamClosed = true

	// Close stdin to signal EOF
	if c.streamStdin != nil {
		if err := c.streamStdin.Close(); err != nil {
			return fmt.Errorf("failed to close stdin: %w", err)
		}
	}

	// Wait for process to complete
	if c.streamCmd != nil {
		if err := c.streamCmd.Wait(); err != nil {
			return fmt.Errorf("sox process failed: %w", err)
		}
	}

	// Wait for stdout reading to complete
	if c.streamOutputDone != nil {
		<-c.streamOutputDone
	}

	// Flush to output path if configured in stream mode
	// if c.outputPath != "" {
	// 	return c.flushStreamBuffer()
	// }

	return nil
}

// stopTicker stops the ticker and flushes remaining data
func (c *Task) stopTicker() error {
	if c.ticker != nil {
		c.ticker.Stop()
		close(c.tickerStop)
	}

	// Final flush
	c.tickerLock.Lock()
	defer c.tickerLock.Unlock()

	return c.flushTickerBuffer()
}

// Close is an alias for Stop(), provided for compatibility with io.Closer.
// Prefer using Stop() explicitly for clarity.
func (c *Task) Close() error {
	return c.Stop()
}

func (c *Task) executeWithRetry(ctx context.Context, inputPath, outputPath string) error {
	backoff := c.retryConfig.InitialBackoff
	var lastErr error

	for attempt := 0; attempt < c.retryConfig.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("conversion cancelled: %w", ctx.Err())
		default:
		}

		var err error
		if c.circuitBreaker != nil {
			err = c.circuitBreaker.Call(func() error {
				return c.convertInternalPath(ctx)
			})
		} else {
			err = c.convertInternalPath(ctx)
		}

		if err == nil {
			return nil
		}

		lastErr = err

		if c.circuitBreaker != nil && err == ErrCircuitOpen {
			return err
		}

		if err == ErrInvalidFormat {
			return err
		}

		if attempt == c.retryConfig.MaxAttempts-1 {
			break
		}

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return fmt.Errorf("conversion cancelled during backoff: %w", ctx.Err())
		}

		backoff = time.Duration(float64(backoff) * c.retryConfig.BackoffMultiple)
		if backoff > c.retryConfig.MaxBackoff {
			backoff = c.retryConfig.MaxBackoff
		}
	}

	return fmt.Errorf("conversion failed after %d attempts: %w", c.retryConfig.MaxAttempts, lastErr)
}

// executeWithRetryStream handles stream-based conversion with I/O piping
func (c *Task) executeWithRetryStream(ctx context.Context, input io.ReadSeeker, output io.Writer) error {
	backoff := c.retryConfig.InitialBackoff
	var lastErr error

	for attempt := 0; attempt < c.retryConfig.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("conversion cancelled: %w", ctx.Err())
		default:
		}

		var err error
		if c.circuitBreaker != nil {
			err = c.circuitBreaker.Call(func() error {
				return c.convertInternal(ctx, input, output)
			})
		} else {
			err = c.convertInternal(ctx, input, output)
		}

		if err == nil {
			return nil
		}

		lastErr = err

		if c.circuitBreaker != nil && err == ErrCircuitOpen {
			return err
		}

		if err == ErrInvalidFormat {
			return err
		}

		if attempt == c.retryConfig.MaxAttempts-1 {
			break
		}

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return fmt.Errorf("conversion cancelled during backoff: %w", ctx.Err())
		}

		backoff = time.Duration(float64(backoff) * c.retryConfig.BackoffMultiple)
		if backoff > c.retryConfig.MaxBackoff {
			backoff = c.retryConfig.MaxBackoff
		}

		// Reset input position for retry
		if _, err := input.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek input for retry: %w", err)
		}
	}

	return fmt.Errorf("conversion failed after %d attempts: %w", c.retryConfig.MaxAttempts, lastErr)
}

// convertInternal performs the actual SoX conversion without retry logic
func (c *Task) convertInternal(ctx context.Context, input io.Reader, output io.Writer) error {
	if err := c.Input.Validate(); err != nil {
		return ErrInvalidFormat
	}

	if err := c.Output.Validate(); err != nil {
		return ErrInvalidFormat
	}

	args := c.buildCommandArgs()
	cmd := exec.CommandContext(ctx, c.Options.SoxPath, args...)

	cmd.Stdin = input
	cmd.Stdout = output

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start sox: %w", err)
	}

	stderrData := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(stderr)
		stderrData <- data
	}()

	if err := cmd.Wait(); err != nil {
		errMsg := <-stderrData

		if ctx.Err() != nil {
			return fmt.Errorf("sox conversion timeout/cancelled: %w", ctx.Err())
		}

		return fmt.Errorf("sox conversion failed: %w\nstderr: %s", err, string(errMsg))
	}

	return nil
}

// convertInternalPath performs the actual SoX conversion for path-based mode
func (c *Task) convertInternalPath(ctx context.Context) error {
	if err := c.Input.Validate(); err != nil {
		return ErrInvalidFormat
	}

	if err := c.Output.Validate(); err != nil {
		return ErrInvalidFormat
	}

	args := c.buildCommandArgs()
	cmd := exec.CommandContext(ctx, c.Options.SoxPath, args...)

	cmd.Stdin = nil  // No stdin for path-based conversion
	cmd.Stdout = nil // No stdout for path-based conversion

	stderr, err := cmd.StderrPipe()

	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start sox: %w", err)
	}

	stderrData := make(chan []byte, 1)

	go func() {
		data, _ := io.ReadAll(stderr)
		stderrData <- data
	}()

	if err := cmd.Wait(); err != nil {
		errMsg := <-stderrData

		if ctx.Err() != nil {
			return fmt.Errorf("sox conversion timeout/cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("sox conversion failed: %w\nstderr: %s", err, string(errMsg))
	}

	return nil
}

// buildCommandArgs constructs the complete SoX command arguments
// For path mode: uses file paths directly (no pipes)
// For stream/ticker mode: uses stdin/stdout pipes (-)
func (c *Task) buildCommandArgs() []string {
	args := []string{}

	args = append(args, c.Options.BuildGlobalArgs()...)
	args = append(args, c.Input.BuildArgs()...)

	// Path mode: use file paths directly (no piping needed)
	if c.pathMode {
		args = append(args, c.inputPath)
		args = append(args, c.Output.BuildArgs()...)
		args = append(args, c.outputPath)
	} else {
		// Stream/ticker mode: use stdin/stdout pipes
		args = append(args, "-") // stdin

		args = append(args, c.Output.BuildArgs()...)

		// For stream mode with outputPath and RAW format, use stdout pipe for incremental append
		// For other formats (FLAC, WAV, etc.) with headers, sox writes directly to file
		// For ticker mode with outputPath, write directly to file
		if c.outputPath != "" && c.streamMode && c.Output.Type != TYPE_FLAC && c.Output.Type != TYPE_WAV {
			args = append(args, "-") // stdout - we'll handle file writing in Go with append
		} else if c.outputPath != "" {
			args = append(args, c.outputPath) // direct file output (required for formats with headers)
		} else {
			args = append(args, "-") // stdout
		}
	}

	if effects := c.Options.buildEffectArgs(); len(effects) > 0 {
		args = append(args, effects...)
	}

	return args
}

// CheckSoxInstalled verifies that SoX is installed and accessible.
// If soxPath is empty, checks for "sox" in PATH.
// Returns an error if SoX is not found or not executable.
//
// Example:
//
//	if err := CheckSoxInstalled(""); err != nil {
//		log.Fatal("SoX not installed:", err)
//	}
//
//	// Check custom path
//	if err := CheckSoxInstalled("/usr/local/bin/sox"); err != nil {
//		log.Fatal("SoX not found at custom path:", err)
//	}
func CheckSoxInstalled(soxPath string) error {
	if soxPath == "" {
		soxPath = "sox"
	}

	cmd := exec.Command(soxPath, "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sox not found or not executable: %w", err)
	}

	return nil
}
