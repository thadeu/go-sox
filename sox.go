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

// sox.Task
// Task handles audio format conversion using SoX with built-in resiliency
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

// New creates a new Task with input and output formats
func New(input, output AudioFormat) *Task {
	return &Task{
		Input:          input,
		Output:         output,
		Options:        DefaultOptions(),
		circuitBreaker: NewCircuitBreaker(),
		retryConfig:    DefaultRetryConfig(),
		streamBuffer:   &bytes.Buffer{},
		tickerBuffer:   &bytes.Buffer{},
		tickerStop:     make(chan struct{}),
	}
}

func NewTicker(input AudioFormat, output AudioFormat, interval time.Duration) *Task {
	conv := New(input, output)
	conv.WithTicker(interval)

	return conv
}

func NewStream(input AudioFormat, output AudioFormat) *Task {
	conv := New(input, output)
	conv.WithStream()

	return conv
}

// WithOptions sets custom conversion options
func (c *Task) WithOptions(opts ConversionOptions) *Task {
	c.Options = opts
	return c
}

// WithCircuitBreaker sets a custom circuit breaker
func (c *Task) WithCircuitBreaker(cb *CircuitBreaker) *Task {
	c.circuitBreaker = cb
	return c
}

// WithRetryConfig sets custom retry configuration
func (c *Task) WithRetryConfig(config RetryConfig) *Task {
	c.retryConfig = config
	return c
}

// DisableResilience disables circuit breaker and retry (not recommended for production)
func (c *Task) DisableResilience() *Task {
	c.circuitBreaker = nil
	c.retryConfig.MaxAttempts = 1
	return c
}

// WithStream enables streaming mode for real-time data processing
// In streaming mode, use Write() to send data and Read() to receive data
func (c *Task) WithStream() *Task {
	c.streamMode = true
	return c
}

// WithTicker enables periodic conversion with specified interval
// Each interval, the buffered data will be converted and output
func (c *Task) WithTicker(interval time.Duration) *Task {
	c.tickerMode = true
	c.tickerDuration = interval
	return c
}

func (s *Task) WithOutputPath(path string) *Task {
	s.outputPath = path
	return s
}

func (s *Task) WithStart() *Task {
	s.Start()

	return s
}

// Convert performs conversion with flexible argument handling
// Arguments can be:
// - Convert(io.Reader, io.Writer) for streaming I/O
// - Convert(string, string) for file paths
// - Mixes of Reader/Writer and string paths are supported
func (c *Task) Convert(args ...interface{}) error {
	ctx := context.Background()
	if c.Options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Options.Timeout)
		defer cancel()
	}
	return c.ConvertWithContext(ctx, args...)
}

// ConvertWithContext performs conversion with context and flexible arguments
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

// Write writes audio data to the Task
// Only valid when using WithStream() mode
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

// Read reads converted audio data from the Task
// Only valid when using WithStream() mode
// Creates a copy of the current buffer to avoid data loss
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

// Start initializes the streaming Task
// For streaming mode, starts the sox process
// For ticker mode, starts the periodic conversion loop
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
	// This prevents the sox process from blocking when stdout buffer is full
	go func() {
		_, err := io.Copy(c.streamOutput, stdout)
		c.streamOutputDone <- err
	}()

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

// Stop stops the Task and closes resources
// For streaming mode, closes the stdin pipe
// For ticker mode, stops the ticker and flushes remaining data
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
	if c.outputPath != "" {
		return c.flushStreamBuffer()
	}

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

// Close is an alias for Stop for compatibility
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

		// Output destination for ticker mode with file output
		if c.tickerMode && c.outputPath != "" {
			args = append(args, c.outputPath)
		} else {
			args = append(args, "-") // stdout
		}
	}

	if effects := c.Options.buildEffectArgs(); len(effects) > 0 {
		args = append(args, effects...)
	}

	return args
}

// CheckSoxInstalled verifies that SoX is installed and accessible
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
