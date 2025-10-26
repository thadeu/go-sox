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
	"context"
	"fmt"
	"io"
	"os/exec"
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
}

// NewConverter creates a new Converter with resilient defaults (circuit breaker, retry, pool)
func NewConverter(input, output AudioFormat) *Converter {
	return &Converter{
		Input:          input,
		Output:         output,
		Options:        DefaultOptions(),
		circuitBreaker: NewCircuitBreaker(),
		retryConfig:    DefaultRetryConfig(),
		pool:           nil, // No pool by default, can be added with WithPool()
	}
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

// Convert performs a one-shot conversion from input reader to output writer
// This uses pipes to stream data through SoX without temporary files
// Includes automatic retry and circuit breaker protection
func (c *Converter) Convert(input io.Reader, output io.Writer) error {
	ctx := context.Background()
	if c.Options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Options.Timeout)
		defer cancel()
	}
	return c.ConvertWithContext(ctx, input, output)
}

// ConvertWithContext performs conversion with context for cancellation and timeout
// Includes automatic retry and circuit breaker protection
func (c *Converter) ConvertWithContext(ctx context.Context, input io.Reader, output io.Writer) error {
	// Acquire pool slot if pool is configured
	if c.pool != nil {
		if err := c.pool.Acquire(ctx); err != nil {
			return fmt.Errorf("failed to acquire worker slot: %w", err)
		}
		defer c.pool.Release()
	}

	// Convert input to ReadSeeker for retry support
	var seekableInput io.ReadSeeker
	if seeker, ok := input.(io.ReadSeeker); ok {
		seekableInput = seeker
	} else {
		// If not seekable, read all into memory (required for retry)
		data, err := io.ReadAll(input)
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		seekableInput = newBytesReader(data)
	}

	// Execute with retry and circuit breaker
	return c.executeWithRetry(ctx, seekableInput, output)
}

func (c *Converter) executeWithRetry(ctx context.Context, input io.ReadSeeker, output io.Writer) error {
	backoff := c.retryConfig.InitialBackoff
	var lastErr error

	for attempt := 0; attempt < c.retryConfig.MaxAttempts; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return fmt.Errorf("conversion cancelled: %w", ctx.Err())
		default:
		}

		// Execute with circuit breaker if enabled
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

		// Don't retry on circuit breaker open
		if c.circuitBreaker != nil && err == ErrCircuitOpen {
			return err
		}

		// Don't retry on validation errors
		if err == ErrInvalidFormat {
			return err
		}

		// Last attempt, don't wait
		if attempt == c.retryConfig.MaxAttempts-1 {
			break
		}

		// Wait with exponential backoff
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return fmt.Errorf("conversion cancelled during backoff: %w", ctx.Err())
		}

		// Increase backoff
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
func (c *Converter) convertInternal(ctx context.Context, input io.Reader, output io.Writer) error {
	// Validate formats
	if err := c.Input.Validate(); err != nil {
		return ErrInvalidFormat
	}
	if err := c.Output.Validate(); err != nil {
		return ErrInvalidFormat
	}

	// Build SoX command arguments
	args := c.buildCommandArgs()

	// Create command with context
	cmd := exec.CommandContext(ctx, c.Options.SoxPath, args...)

	// Set up pipes
	cmd.Stdin = input
	cmd.Stdout = output

	// Capture stderr for debugging
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start sox: %w", err)
	}

	// Track process
	GetMonitor().TrackProcess(cmd.Process.Pid)
	defer GetMonitor().UntrackProcess(cmd.Process.Pid)

	// Read stderr in background (to prevent blocking)
	stderrData := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(stderr)
		stderrData <- data
	}()

	// Wait for command to complete
	if err := cmd.Wait(); err != nil {
		errMsg := <-stderrData
		if ctx.Err() != nil {
			return fmt.Errorf("sox conversion timeout/cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("sox conversion failed: %w\nstderr: %s", err, string(errMsg))
	}

	return nil
}

// ConvertFile converts audio from an input file to an output file
// This is a convenience method for file-based conversions
// Includes automatic retry and circuit breaker protection
func (c *Converter) ConvertFile(inputPath, outputPath string) error {
	ctx := context.Background()
	if c.Options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Options.Timeout)
		defer cancel()
	}
	return c.ConvertFileWithContext(ctx, inputPath, outputPath)
}

// ConvertFileWithContext converts audio files with context for cancellation and timeout
func (c *Converter) ConvertFileWithContext(ctx context.Context, inputPath, outputPath string) error {
	// Acquire pool slot if pool is configured
	if c.pool != nil {
		if err := c.pool.Acquire(ctx); err != nil {
			return fmt.Errorf("failed to acquire worker slot: %w", err)
		}
		defer c.pool.Release()
	}

	// Execute with retry and circuit breaker
	return c.executeFileWithRetry(ctx, inputPath, outputPath)
}

func (c *Converter) executeFileWithRetry(ctx context.Context, inputPath, outputPath string) error {
	backoff := c.retryConfig.InitialBackoff
	var lastErr error

	for attempt := 0; attempt < c.retryConfig.MaxAttempts; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return fmt.Errorf("conversion cancelled: %w", ctx.Err())
		default:
		}

		// Execute with circuit breaker if enabled
		var err error
		if c.circuitBreaker != nil {
			err = c.circuitBreaker.Call(func() error {
				return c.convertFileInternal(ctx, inputPath, outputPath)
			})
		} else {
			err = c.convertFileInternal(ctx, inputPath, outputPath)
		}

		if err == nil {
			return nil
		}

		lastErr = err

		// Don't retry on circuit breaker open
		if c.circuitBreaker != nil && err == ErrCircuitOpen {
			return err
		}

		// Don't retry on validation errors
		if err == ErrInvalidFormat {
			return err
		}

		// Last attempt, don't wait
		if attempt == c.retryConfig.MaxAttempts-1 {
			break
		}

		// Wait with exponential backoff
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return fmt.Errorf("conversion cancelled during backoff: %w", ctx.Err())
		}

		// Increase backoff
		backoff = time.Duration(float64(backoff) * c.retryConfig.BackoffMultiple)
		if backoff > c.retryConfig.MaxBackoff {
			backoff = c.retryConfig.MaxBackoff
		}
	}

	return fmt.Errorf("file conversion failed after %d attempts: %w", c.retryConfig.MaxAttempts, lastErr)
}

func (c *Converter) convertFileInternal(ctx context.Context, inputPath, outputPath string) error {
	// Validate formats
	if err := c.Input.Validate(); err != nil {
		return ErrInvalidFormat
	}
	if err := c.Output.Validate(); err != nil {
		return ErrInvalidFormat
	}

	// Build SoX command arguments with file paths
	args := c.buildCommandArgs()

	// Replace stdin/stdout placeholders with actual file paths
	inputReplaced := false
	for i, arg := range args {
		if arg == "-" {
			if !inputReplaced {
				args[i] = inputPath
				inputReplaced = true
			} else {
				args[i] = outputPath
			}
		}
	}

	// Create command with context
	cmd := exec.CommandContext(ctx, c.Options.SoxPath, args...)

	// Track process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start sox: %w", err)
	}

	GetMonitor().TrackProcess(cmd.Process.Pid)
	defer GetMonitor().UntrackProcess(cmd.Process.Pid)

	// Wait for completion
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("sox conversion timeout/cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("sox conversion failed: %w", err)
	}

	return nil
}

// buildCommandArgs constructs the complete SoX command arguments
func (c *Converter) buildCommandArgs() []string {
	args := []string{}

	// Global options
	args = append(args, c.Options.buildGlobalArgs()...)

	// Input format arguments
	args = append(args, c.Input.buildArgs(true)...)

	// Input file (stdin)
	args = append(args, "-")

	// Output format arguments
	args = append(args, c.Output.buildArgs(false)...)

	// Format-specific arguments for output
	args = append(args, c.Options.buildFormatArgs(&c.Output)...)

	// Output file (stdout)
	args = append(args, "-")

	// Effects
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
