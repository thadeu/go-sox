# API Migration Guide

This document shows how the go-sox API has evolved to provide a unified, flexible interface while maintaining backward compatibility.

## Overview

The refactored API consolidates all audio conversion scenarios under a single `New()` entry point with three conversion modes:

1. **Simple Mode**: `Convert(input, output)` - one-shot batch processing
2. **Ticker Mode**: `WithTicker()` + `Start()` + `Write()` - periodic processing
3. **Stream Mode**: `WithStream()` + `Start()` + `Write()/Read()` - real-time streaming

## Migration Examples

### Before: Simple Conversion

```go
// Old API: NewConverter() with only io.Reader/io.Writer support
input := bytes.NewReader(pcmData)
output := &bytes.Buffer{}

converter := sox.NewConverter(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE)
err := converter.Convert(input, output)

// For file conversion, required separate method:
converter := sox.NewConverter(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE)
err := converter.ConvertFile("input.pcm", "output.flac")
```

### After: Simple Conversion

```go
// New API: New() with flexible arguments
// Bytes to bytes
input := bytes.NewReader(pcmData)
output := &bytes.Buffer{}

conv := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE)
err := conv.Convert(input, output)

// File to file - same method!
conv := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE)
err := conv.Convert("input.pcm", "output.flac")

// Mixed: reader input, file output
input := os.Open("input.pcm")
conv := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE)
err := conv.Convert(input, "output.flac")
```

**Benefits:**
- One method handles all cases
- Automatic type detection
- No need for separate `ConvertFile()` method
- More intuitive and flexible

### Before: Streaming Conversion

```go
// Old API: Required separate Streamer type
stream := sox.NewStreamer(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE)
stream.Start(3 * time.Second)

// Write packets
for packet := range packets {
    stream.Write(packet)
}

// Stop and flush
stream.Stop()
```

### After: Streaming Conversion (Ticker Mode)

```go
// New API: Unified under Converter with WithTicker()
conv := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE).
    WithTicker(3 * time.Second)

if err := conv.Start(); err != nil {
    log.Fatal(err)
}

// Write packets
for packet := range packets {
    conv.Write(packet)
}

// Stop and flush
if err := conv.Stop(); err != nil {
    log.Fatal(err)
}
```

**Benefits:**
- Single unified API for all conversion modes
- Consistent builder pattern
- Unified error handling
- Ticker mode integrated into main Converter type

### New: Real-Time Streaming Mode

```go
// Completely new mode: WithStream() for true real-time I/O
conv := sox.New(sox.PCM_RAW_8K_MONO, sox.FLAC_16K_MONO_LE).
    WithStream()

if err := conv.Start(); err != nil {
    log.Fatal(err)
}

// Writer goroutine
go func() {
    for packet := range packets {
        conv.Write(packet)
    }
    conv.Stop()
}()

// Reader goroutine
go func() {
    buffer := make([]byte, 4096)
    for {
        n, err := conv.Read(buffer)
        if err != nil {
            break
        }
        processOutput(buffer[:n])
    }
}()
```

## Configuration Pattern

### Before: Configuration

```go
converter := sox.NewConverter(input, output)
converter.WithOptions(options)
converter.WithRetryConfig(retryConfig)
```

### After: Configuration (Fluent Builder)

```go
conv := sox.New(input, output).
    WithOptions(options).
    WithRetryConfig(retryConfig).
    WithCircuitBreaker(cb).
    DisableResilience()
```

**Benefits:**
- Fluent builder pattern
- Method chaining for readability
- Consistent with Go conventions

## Backward Compatibility

The old `NewConverter()` function still works but is now a wrapper:

```go
// Both work identically
old := sox.NewConverter(input, output)
new := sox.New(input, output)

// Internally: NewConverter just calls New()
func NewConverter(input, output AudioFormat) *Converter {
    return New(input, output)
}
```

Existing code using `NewConverter()` continues to work without modification.

## File-Based Operations

### Before: Separate ConvertFile Method

```go
converter := sox.NewConverter(input, output)
err := converter.ConvertFile("input.pcm", "output.flac")

// or with context:
err := converter.ConvertFileWithContext(ctx, "input.pcm", "output.flac")
```

### After: Unified Convert Method

```go
conv := sox.New(input, output)
err := conv.Convert("input.pcm", "output.flac")

// with context:
err := conv.ConvertWithContext(ctx, "input.pcm", "output.flac")
```

**Benefits:**
- One method for all I/O patterns
- Cleaner, more discoverable API
- Type-safe argument detection

## Internal Structure Changes

### Before: Multiple Types

```go
// Three separate concepts
type Converter struct { ... }
type Streamer struct { ... }
// NewConverter() and NewStreamer() were independent
```

### After: Unified Type

```go
// Single unified type with multiple modes
type Converter struct {
    // ... base fields ...
    streamMode     bool
    tickerMode     bool
    // ... mode-specific fields ...
}

// Single entry point
func New(input, output AudioFormat) *Converter
```

**Benefits:**
- Consistent behavior across modes
- Shared resilience features
- Single configuration point
- Easier maintenance

## Error Handling

Both APIs use error returns in the same way:

```go
// Before and After
if err := converter.Convert(...); err != nil {
    if err == ErrCircuitOpen {
        // Circuit breaker opened
    } else if err == ErrInvalidFormat {
        // Format validation failed
    } else {
        // Other error
    }
}
```

## Testing Compatibility

All existing tests continue to pass with the new API:

```bash
go test -v
# All tests pass without modification
```

## Migration Checklist

- [ ] Update any `NewConverter()` calls to `New()` (optional, both work)
- [ ] Replace `ConvertFile()` calls with unified `Convert()`
- [ ] If using `NewStreamer()`:
  - [ ] For ticker mode: use `WithTicker()` instead
  - [ ] For streaming: use `WithStream()` instead
- [ ] Update configuration to use fluent builder pattern
- [ ] Test application works with new API
- [ ] (Optional) Remove `NewStreamer` usage if migrated

## Performance

The new unified API has identical performance characteristics:

- **Simple Mode**: Same performance as old `NewConverter()`
- **Ticker Mode**: Same performance as old `NewStreamer()` with `WithAutoStart()`
- **Stream Mode**: New capability, optimized for real-time I/O
