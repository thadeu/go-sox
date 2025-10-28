# Unified Audio Conversion API - Complete Summary

## What Changed

The go-sox package has been refactored to provide a unified, flexible API for all audio conversion scenarios. The new design uses a single entry point `sox.New(input, output)` with three distinct conversion modes accessible through builder methods.

## Three Conversion Modes

### 1. Simple Conversion (Default)

**Best for:** Batch processing, one-shot conversions, simple use cases

```go
conv := sox.New(inputFormat, outputFormat)
err := conv.Convert(input, output)
```

**Supported argument types:**
- `io.Reader` → `io.Writer` (bytes to bytes)
- `string` → `string` (file path to file path)
- Mixed (reader/writer and paths)

```go
// Bytes to bytes
err := conv.Convert(bytes.NewReader(data), &buffer)

// File to file
err := conv.Convert("input.pcm", "output.flac")

// Mixed
err := conv.Convert(os.Open("input.pcm"), "output.flac")
```

### 2. Ticker-Based Conversion

**Best for:** Periodic batch processing, real-time systems with buffering, VoIP transcoding

```go
conv := sox.New(inputFormat, outputFormat).WithTicker(interval)
conv.Start()
for data := range inputStream {
    conv.Write(data)
}
conv.Stop()
```

**Key characteristics:**
- Automatically buffers incoming data
- Processes buffer every `interval` duration
- Thread-safe through internal locking
- Flushes remaining data on Stop()

### 3. Real-Time Streaming

**Best for:** Low-latency applications, live audio streaming, true real-time I/O

```go
conv := sox.New(inputFormat, outputFormat).WithStream()
conv.Start()

go func() {
    for data := range input {
        conv.Write(data)
    }
    conv.Stop()
}()

go func() {
    buf := make([]byte, 4096)
    for {
        n, err := conv.Read(buf)
        if err != nil { break }
        processOutput(buf[:n])
    }
}()
```

**Key characteristics:**
- Minimal buffering for lowest latency
- Continuous data flow through sox
- Separate reader and writer goroutines
- Best throughput for streaming applications

## Complete API Reference

### Constructor

```go
func New(input, output AudioFormat) *Converter
```

Entry point for all conversions. Use builder methods to configure behavior.

### Conversion Methods

```go
func (c *Converter) Convert(args ...interface{}) error
func (c *Converter) ConvertWithContext(ctx context.Context, args ...interface{}) error
```

Execute simple one-shot conversion. Arguments are auto-detected:
- io.Reader, io.Writer, or string file paths
- Any combination of the above

### Builder Methods

```go
func (c *Converter) WithOptions(opts ConversionOptions) *Converter
func (c *Converter) WithPool(pool ...*Pool) *Converter
func (c *Converter) WithRetryConfig(config RetryConfig) *Converter
func (c *Converter) WithCircuitBreaker(cb *CircuitBreaker) *Converter
func (c *Converter) DisableResilience() *Converter
func (c *Converter) WithTicker(interval time.Duration) *Converter
func (c *Converter) WithStream() *Converter
```

All return `*Converter` for method chaining.

### Lifecycle Methods

```go
func (c *Converter) Start() error
func (c *Converter) Stop() error
func (c *Converter) Close() error  // Alias for Stop()
```

`Start()` required for ticker and stream modes. Initialize before calling Write/Read.

### I/O Methods

```go
func (c *Converter) Write(data []byte) (int, error)
func (c *Converter) Read(b []byte) (int, error)
```

Available in ticker and stream modes. Write adds data, Read retrieves output.

## Resilience Features

All conversions include by default:

1. **Retry with Exponential Backoff**
   - Configurable maximum attempts (default: 3)
   - Exponential backoff with configurable multiplier (default: 2.0)
   - Bounded maximum backoff time (default: 1 second)

2. **Circuit Breaker**
   - Prevents cascading failures
   - Opens when failure rate exceeds threshold
   - Automatically recovers

3. **Pool-Based Concurrency Control**
   - Prevents resource exhaustion
   - Configurable worker slots (default: 500)
   - Transparent to users

Disable for performance-critical, non-critical operations:

```go
conv := sox.New(input, output).DisableResilience()
```

## Thread Safety

| Mode | Thread Safety | Usage Pattern |
|------|---------------|---------------|
| Simple | Safe (independent instances) | Each goroutine gets own converter |
| Ticker | Thread-safe (internal locking) | Multiple readers can Write() |
| Stream | Not safe for concurrent ops | One writer, one reader goroutine |

## Format Support

Pre-defined presets:
```go
sox.PCM_RAW_8K_MONO              // PCM 8kHz mono
sox.FLAC_16K_MONO_LE             // FLAC 16kHz mono
sox.WAV_16K_MONO                 // WAV 16kHz mono
sox.WAV_8K_MONO_LE               // WAV 8kHz mono little-endian
sox.ULAW_8K_MONO                 // μ-law 8kHz mono (G.711)
```

Custom formats:
```go
custom := sox.AudioFormat{
    Type:       "wav",
    Encoding:   "signed-integer",
    SampleRate: 16000,
    Channels:   1,
    BitDepth:   16,
}
```

## Error Handling

Standard Go error handling:

```go
// Simple conversion
if err := conv.Convert(input, output); err != nil {
    // Handle conversion errors
}

// Stream operations
if err := conv.Start(); err != nil {
    // Handle startup errors
}

if n, err := conv.Write(data); err != nil {
    // Handle write errors (stream/ticker mode)
}

if err := conv.Stop(); err != nil {
    // Handle shutdown errors
}
```

## Configuration Examples

### Basic Usage

```go
conv := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE)
err := conv.Convert(inputReader, outputBuffer)
```

### With Options

```go
opts := sox.DefaultOptions()
opts.ShowProgress = true
opts.CompressionLevel = 8

conv := sox.New(input, output).WithOptions(opts)
err := conv.Convert("input.pcm", "output.flac")
```

### With Resilience Configuration

```go
conv := sox.New(input, output).WithRetryConfig(sox.RetryConfig{
    MaxAttempts:    5,
    InitialBackoff: 50 * time.Millisecond,
    MaxBackoff:     2 * time.Second,
    BackoffMultiple: 2.0,
})
```

### With Concurrency Control

```go
pool := sox.NewPoolWithLimit(20)

conv := sox.New(input, output).WithPool(pool)
// Multiple conversions now share pool constraints
```

### Complete Example

```go
conv := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE).
    WithOptions(customOptions).
    WithPool(sox.NewPoolWithLimit(10)).
    WithRetryConfig(sox.RetryConfig{
        MaxAttempts:    3,
        InitialBackoff: 100 * time.Millisecond,
        MaxBackoff:     1 * time.Second,
        BackoffMultiple: 2.0,
    }).
    WithCircuitBreaker(customCB)

err := conv.Convert("input.pcm", "output.flac")
```

## Backward Compatibility

The old `NewConverter()` function still works as a wrapper:

```go
// Both equivalent
old := sox.NewConverter(input, output)
new := sox.New(input, output)
```

Old methods `ConvertFile()` and `ConvertFileWithContext()` are now handled by unified `Convert()`.

## Performance Characteristics

| Mode | Latency | Memory | Throughput | Use Case |
|------|---------|--------|-----------|----------|
| Simple | Medium | Low-Medium | Medium | Batch processing |
| Ticker | Low-Medium | Medium | Medium | Periodic batching |
| Stream | Low | Low | High | Real-time I/O |

## Files Changed

- `sox.go`: Unified Converter type with all three modes
- `docs/UNIFIED_API.md`: Comprehensive API documentation
- `MIGRATION.md`: Migration guide from old API
- `examples/simple_conversion/`: Simple mode example
- `examples/ticker_conversion/`: Ticker mode example
- `examples/streaming_realtime/`: Stream mode example

## Testing

All existing tests pass:
```bash
go test -v -race  # All tests pass with race detection
```

## Deprecation Policy

- `NewConverter()`: Still works, not deprecated
- `Streamer` type: Still available for backward compatibility
- `ConvertFile()` family: Merged into `Convert()`

## Getting Started

1. **Simple use case:**
   ```go
   conv := sox.New(input, output)
   err := conv.Convert(inputData, outputBuffer)
   ```

2. **Periodic processing:**
   ```go
   conv := sox.New(input, output).WithTicker(interval)
   conv.Start()
   // Write data, gets converted at intervals
   conv.Stop()
   ```

3. **Real-time streaming:**
   ```go
   conv := sox.New(input, output).WithStream()
   conv.Start()
   // Concurrent reader/writer for low-latency I/O
   ```

See `docs/UNIFIED_API.md` for detailed documentation and examples.
