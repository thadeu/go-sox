# Unified Audio Conversion API

The go-sox package now provides a unified, flexible API for all audio conversion scenarios. A single entry point `sox.New(input, output)` supports three distinct conversion modes through builder methods.

## Quick Start

```go
// All conversions start with New()
conv := sox.New(inputFormat, outputFormat)
```

## Conversion Modes

### 1. Simple Conversion (Bytes-to-Bytes or File-to-File)

The simplest and most common use case. Processes all data at once and returns when complete.

```go
// Bytes to bytes conversion
inputData := generatePCMData()
inputReader := bytes.NewReader(inputData)
outputBuffer := &bytes.Buffer{}

conv := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE)

if err := conv.Convert(inputReader, outputBuffer); err != nil {
    log.Fatal(err)
}

if err := conv.Convert("input.pcm", "output.flac"); err != nil {
    log.Fatal(err)
}

// Mixed: io.Reader input, file path output
inputReader := os.Open("input.pcm")

if err := conv.Convert(inputReader, "output.flac"); err != nil {
    log.Fatal(err)
}
```

**Flexible Argument Handling:**
The `Convert()` method accepts flexible arguments that are automatically detected:
- `io.Reader` interfaces (e.g., `bytes.Buffer`, file pointer)
- String file paths
- Any combination of readers, writers, and paths

### 2. Ticker-Based Conversion (Periodic Processing)

Process buffered audio at regular intervals. Useful for real-time systems where data arrives continuously and needs periodic processing.

```go
// Setup: periodic conversion every 3 seconds
conv := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE).
    WithTicker(3 * time.Second)

// Start the ticker
if err := conv.Start(); err != nil {
    log.Fatal(err)
}

// Write incoming audio packets
for packet := range audioPackets {
    n, err := conv.Write(packet)
    
    if err != nil {
        log.Printf("Write error: %v", err)
    }
}

// Stop Ticker and flush remaining data
if err := conv.Stop(); err != nil {
    log.Fatal(err)
}
```

**How it works:**
1. Audio data is buffered as you call `Write()`
2. Every 3 seconds (or your configured interval), the buffer is automatically flushed and converted
3. After conversion, the buffer is cleared and ready for more data
4. When you call `Stop()`, any remaining buffered data is flushed

**Use cases:**
- VoIP systems that need periodic transcoding
- Real-time monitoring with batched processing
- Stream recording with timed format conversions

### 3. Real-Time Streaming (Continuous Data Flow)

Stream data through sox with no buffering. Useful for applications that need true real-time processing with minimal latency.

```go
// Setup: streaming mode
streamer := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE).
  WithStream()

// Start the streaming process
if err := streamer.Start(); err != nil {
    log.Fatal(err)
}

// Writer goroutine: send audio packets to sox
go func() {
    for packet := range audioBuffer {
        n, err := streamer.Write(packet.Payload)

        if err != nil {
            log.Printf("Write failed: %v", err)
        }
        
        fmt.Printf("Wrote %d bytes\n", n)
    }
}()

// Reader goroutine: receive converted audio
go func() {
    buffer := make([]byte, 4096)

    for {
        n, err := streamer.Read(buffer)

        if err != nil {
            break
        }

        if n > 0 {
            // Process converted audio
            processConvertedAudio(buffer[:n])
        }
    }
}()

// In the some moment, you can stop the streamer, close file and write headers (if you use wav or flac)
// Signal end of input
if err := streamer.Stop(); err != nil {
    log.Fatal(err)
}
```

**Key differences from ticker mode:**
- Data flows through sox in real-time without waiting for interval
- Minimal buffering for lowest possible latency
- Multiple concurrent readers and writers possible
- Useful for live audio streaming applications

## Builder Methods

All conversion modes support these builder methods:

```go
conv := sox.New(inputFormat, outputFormat).
    WithOptions(customOptions).
    WithPool(pool).
    WithRetryConfig(retryConfig).
    WithCircuitBreaker(circuitBreaker).
    DisableResilience()  // For non-critical conversions

// Then call one of:
conv.Convert(input, output)  // Simple mode
conv.WithTicker(interval).Start()  // Ticker mode
conv.WithStream().Start()  // Streaming mode
```

### Common Configuration

```go
// Custom options
options := sox.DefaultOptions()
options.ShowProgress = true
options.CompressionLevel = 8

// Resilience configuration
retryConfig := sox.RetryConfig{
    MaxAttempts: 3,
    InitialBackoff: 100 * time.Millisecond,
    MaxBackoff: 1 * time.Second,
    BackoffMultiple: 2.0,
}

// Pool for concurrency control
pool := sox.NewPoolWithLimit(10)

// Build converter
conv := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE).
    WithOptions(options).
    WithRetryConfig(retryConfig).
    WithPool(pool)
```

## Resilience Features

By default, all conversions include:

1. **Automatic Retry**: Exponential backoff retries on transient failures
2. **Circuit Breaker**: Prevents cascading failures when sox is unavailable
3. **Pool-Based Concurrency Control**: Prevents resource exhaustion

### Disabling Resilience

For non-critical conversions, disable resilience to reduce latency:

```go
conv := sox.New(inputFormat, outputFormat).
    DisableResilience()
```

## Error Handling

All methods return errors that should be handled:

```go
// Simple conversion
if err := conv.Convert(inputReader, outputBuffer); err != nil {
    // Handle: io errors, validation errors, format errors, etc.
}

// Streaming/Ticker setup
if err := conv.Start(); err != nil {
    // Handle: pipe creation, sox startup, etc.
}

// Writing data
if n, err := conv.Write(data); err != nil {
    // Handle: stream closed, not started, etc.
}

// Stopping
if err := conv.Stop(); err != nil {
    // Handle: process termination errors, etc.
}
```

## Backward Compatibility

The older `NewConverter()` function still works:

```go
// Old API still supported
conv := sox.NewConverter(inputFormat, outputFormat)

// But New() is the preferred way
conv := sox.New(inputFormat, outputFormat)
```

## Format Presets

Pre-configured common formats:

```go
sox.PCM_RAW_8K_MONO         // 8kHz mono 16-bit PCM (telephony)
sox.FLAC_16K_MONO_LE        // 16kHz mono FLAC little-endian
sox.WAV_16K_MONO            // 16kHz mono WAV
sox.WAV_8K_MONO_LE          // 8kHz mono WAV little-endian
sox.ULAW_8K_MONO            // 8kHz mono μ-law (G.711)
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

## Examples

See the `examples/` directory for complete working examples:
- `simple_conversion/`: Bytes-to-bytes and file-to-file conversion
- `ticker_conversion/`: Periodic batch processing
- `streaming_realtime/`: Real-time streaming with concurrent I/O

## Performance Considerations

### Simple Mode
- Best for batch processing
- Lowest memory overhead for small files
- Automatic retries on failure

### Ticker Mode
- Good for periodic processing
- Controlled memory usage with regular flushes
- Suitable for long-running services

### Streaming Mode
- Lowest latency
- Highest throughput
- Best for real-time applications
- Requires concurrent reader/writer goroutines

## Thread Safety

- **Simple Mode**: Safe to call from multiple goroutines (each gets its own converter)
- **Ticker Mode**: Thread-safe through internal locking
- **Streaming Mode**: Not thread-safe for concurrent writes/reads; use one writer and one reader goroutine
