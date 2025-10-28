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

// Converter handles audio format conversion using SoX with built-in resiliency
type Converter struct {
	Input          AudioFormat
	Output         AudioFormat
	Options        ConversionOptions
	circuitBreaker *CircuitBreaker
	retryConfig    RetryConfig
	pool           *Pool

	// Streaming state
	streamMode    bool
	streamBuffer  *bytes.Buffer
	streamLock    sync.Mutex
	streamStarted bool
	streamClosed  bool
	streamCmd     *exec.Cmd
	streamStdin   io.WriteCloser
	streamStdout  io.ReadCloser

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

// New creates a new Converter with input and output formats
func New(input, output AudioFormat) *Converter {
	return &Converter{
		Input:          input,
		Output:         output,
		Options:        DefaultOptions(),
		circuitBreaker: NewCircuitBreaker(),
		retryConfig:    DefaultRetryConfig(),
		pool:           nil,
		streamBuffer:   &bytes.Buffer{},
		tickerBuffer:   &bytes.Buffer{},
		tickerStop:     make(chan struct{}),
	}
}

func NewTicker(input AudioFormat, output AudioFormat, interval time.Duration) *Converter {
	conv := New(input, output)
	conv.WithTicker(interval)

	return conv
}

func NewStream(input AudioFormat, output AudioFormat) *Converter {
	conv := New(input, output)
	conv.WithStream()

	return conv
}

// WithOptions sets custom conversion options
func (c *Converter) WithOptions(opts ConversionOptions) *Converter {
	c.Options = opts
	return c
}

// WithCircuitBreaker sets a custom circuit breaker
func (c *Converter) WithCircuitBreaker(cb *CircuitBreaker) *Converter {
	c.circuitBreaker = cb
	return c
}

// WithRetryConfig sets custom retry configuration
func (c *Converter) WithRetryConfig(config RetryConfig) *Converter {
	c.retryConfig = config
	return c
}

// WithPool adds pool-based concurrency control
// If pool is nil, creates a new default pool
func (c *Converter) WithPool(pool ...*Pool) *Converter {
	if len(pool) > 0 && pool[0] != nil {
		c.pool = pool[0]
	} else {
		c.pool = NewPool()
	}
	return c
}

// DisableResilience disables circuit breaker and retry (not recommended for production)
func (c *Converter) DisableResilience() *Converter {
	c.circuitBreaker = nil
	c.retryConfig.MaxAttempts = 1
	return c
}

// WithStream enables streaming mode for real-time data processing
// In streaming mode, use Write() to send data and Read() to receive data
func (c *Converter) WithStream() *Converter {
	c.streamMode = true
	return c
}

// WithTicker enables periodic conversion with specified interval
// Each interval, the buffered data will be converted and output
func (c *Converter) WithTicker(interval time.Duration) *Converter {
	c.tickerMode = true
	c.tickerDuration = interval
	return c
}

func (s *Converter) WithOutputPath(path string) *Converter {
	s.outputPath = path
	return s
}

func (s *Converter) WithStart() *Converter {
	s.Start()

	return s
}

// Convert performs conversion with flexible argument handling
// Arguments can be:
// - Convert(io.Reader, io.Writer) for streaming I/O
// - Convert(string, string) for file paths
// - Mixes of Reader/Writer and string paths are supported
func (c *Converter) Convert(args ...interface{}) error {
	ctx := context.Background()
	if c.Options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Options.Timeout)
		defer cancel()
	}
	return c.ConvertWithContext(ctx, args...)
}

// ConvertWithContext performs conversion with context and flexible arguments
func (c *Converter) ConvertWithContext(ctx context.Context, args ...interface{}) error {
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
			return c.executeWithRetryPath(ctx, inputPath, outputPath)
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
	return c.executeWithRetry(ctx, seekableInput, outputWriter)
}

// Write writes audio data to the converter
// Only valid when using WithStream() mode
func (c *Converter) Write(data []byte) (int, error) {
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

	return c.streamStdin.Write(data)
}

// Read reads converted audio data from the converter
// Only valid when using WithStream() mode
// Creates a copy of the current buffer to avoid data loss
func (c *Converter) Read(b []byte) (int, error) {
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

// Start initializes the streaming converter
// For streaming mode, starts the sox process
// For ticker mode, starts the periodic conversion loop
func (c *Converter) Start() error {
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

	// Track process
	GetMonitor().TrackProcess(cmd.Process.Pid)

	return nil
}

// runTicker initializes the ticker-based conversion
func (c *Converter) runTicker() error {
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
func (c *Converter) flushTickerBuffer() error {
	if c.tickerBuffer.Len() == 0 {
		return nil
	}

	// Create a copy of buffer data
	inputData := make([]byte, c.tickerBuffer.Len())
	copy(inputData, c.tickerBuffer.Bytes())

	// Reset buffer after copying to avoid duplicate processing
	c.tickerBuffer.Reset()

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

// Stop stops the converter and closes resources
// For streaming mode, closes the stdin pipe
// For ticker mode, stops the ticker and flushes remaining data
func (c *Converter) Stop() error {
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
		GetMonitor().UntrackProcess(c.streamCmd.Process.Pid)
	}

	return nil
}

// stopTicker stops the ticker and flushes remaining data
func (c *Converter) stopTicker() error {
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
func (c *Converter) Close() error {
	return c.Stop()
}

func (c *Converter) executeWithRetry(ctx context.Context, input io.ReadSeeker, output io.Writer) error {
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

		if _, err := input.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek input for retry: %w", err)
		}
	}

	return fmt.Errorf("conversion failed after %d attempts: %w", c.retryConfig.MaxAttempts, lastErr)
}

// executeWithRetryPath handles path-based conversion with direct file access (no piping)
func (c *Converter) executeWithRetryPath(ctx context.Context, inputPath, outputPath string) error {
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
				return c.convertInternalPath(ctx, inputPath, outputPath)
			})
		} else {
			err = c.convertInternalPath(ctx, inputPath, outputPath)
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

// convertInternal performs the actual SoX conversion without retry logic
func (c *Converter) convertInternal(ctx context.Context, input io.Reader, output io.Writer) error {
	if err := c.Input.Validate(); err != nil {
		return ErrInvalidFormat
	}
	if err := c.Output.Validate(); err != nil {
		return ErrInvalidFormat
	}

	// Acquire pool slot if configured
	if c.pool != nil {
		if err := c.pool.Acquire(ctx); err != nil {
			return fmt.Errorf("failed to acquire worker slot: %w", err)
		}
		defer c.pool.Release()
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

	GetMonitor().TrackProcess(cmd.Process.Pid)
	defer GetMonitor().UntrackProcess(cmd.Process.Pid)

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
func (c *Converter) convertInternalPath(ctx context.Context, inputPath, outputPath string) error {
	if err := c.Input.Validate(); err != nil {
		return ErrInvalidFormat
	}
	if err := c.Output.Validate(); err != nil {
		return ErrInvalidFormat
	}

	// Acquire pool slot if configured
	if c.pool != nil {
		if err := c.pool.Acquire(ctx); err != nil {
			return fmt.Errorf("failed to acquire worker slot: %w", err)
		}
		defer c.pool.Release()
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

	GetMonitor().TrackProcess(cmd.Process.Pid)
	defer GetMonitor().UntrackProcess(cmd.Process.Pid)

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
func (c *Converter) buildCommandArgs() []string {
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
