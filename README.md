<p align="center">
  <img src="docs/images/go-sox-1.png" alt="go-sox" width="100%"> 
</p>

# go-sox

![Build Status](https://github.com/thadeu/go-sox/actions/workflows/go-test.yml/badge.svg)
[![Go Reference](https://pkg.go.dev/badge/github.com/thadeu/go-sox.svg)](https://pkg.go.dev/github.com/thadeu/go-sox)

High-performance Go wrapper for SoX CLI using pipes and streams for audio format conversion.

## Table of Contents

- [Overview](#overview)
- [Why go-sox?](#why-go-sox)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [API Reference](#api-reference)
- [Performance](#performance)
- [Architecture & Design Decisions](#architecture--design-decisions)
- [Core Features](#core-features)
- [Usage Patterns](#usage-patterns)
- [Audio Formats](#audio-formats)
- [Options](#options)
- [Production Deployment](#production-deployment)
- [Troubleshooting](#troubleshooting)
- [Examples](#examples)
- [Contributing](#contributing)
- [License](#license)

## Quick Links

- [API Documentation](https://pkg.go.dev/github.com/thadeu/go-sox)
- [Production Guide](docs/PRODUCTION.md)
- [Advanced Options](docs/ADVANCED_OPTIONS.md)
- [Unified API](docs/UNIFIED_API.md)
- [Usage Guide](docs/USAGE.md)

## Overview

This library provides a Go interface to SoX (Sound eXchange) that eliminates file I/O overhead by using stdin/stdout pipes. It's designed for production environments where audio conversion performance matters.

**Primary use case**: Converting RTP media streams (PCM Raw) to compressed formats (FLAC) for transcription APIs with minimal latency.

## Why go-sox?

### Performance

Benchmarks on Apple M2 with 1 second audio (16kHz mono PCM to FLAC):

```
BenchmarkConverter_Convert-8         244      4870383 ns/op      91747 B/op    148 allocs/op
BenchmarkStreamConverter-8           243      4886878 ns/op      98629 B/op    129 allocs/op
BenchmarkFFmpegComparison-8           28     41985972 ns/op      23525 B/op     99 allocs/op
```

**SoX with pipes is 8.6x faster than ffmpeg** for typical conversions:
- SoX Converter: 4.87ms per conversion
- SoX Stream: 4.88ms per conversion  
- FFmpeg (file I/O): 42ms per conversion

### Performance Comparison

| Format     | Size   | go-sox | ffmpeg | Speedup |
|------------|--------|--------|--------|---------|
| PCM→FLAC   | 1s     | 4.87ms | 42ms   | 8.6x    |
| PCM→WAV    | 1s     | 3.21ms | 38ms   | 11.8x   |
| μ-law→FLAC | 1s     | 5.12ms | 45ms   | 8.8x    |

### Memory Usage

- Per conversion: ~150KB heap allocation
- Streaming mode: Constant ~32KB regardless of stream length
- Ticker mode: Buffers data between ticks, minimal overhead

### Production-Ready by Default

Every converter includes:
- **Circuit breaker** - prevents cascading failures
- **Automatic retry** - exponential backoff on transient errors
- **Timeout support** - prevents hung conversions
- **Context cancellation** - proper cleanup on cancellation

## Installation

```bash
# Install SoX
brew install sox  # macOS
apt-get install sox  # Debian/Ubuntu
# or
yum install sox  # CentOS/RHEL

# Install library
go get github.com/thadeu/go-sox
```

## Quick Start

### Simple Conversion

```go
import sox "github.com/thadeu/go-sox"

// New is production-ready by default (circuit breaker, retry, timeout)
task := sox.New(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO)

// Convert from reader to writer
input := bytes.NewReader(pcmData)
output := &bytes.Buffer{}
err := task.Convert(input, output)

// Or convert files (optimized path mode)
err := task.Convert("input.pcm", "output.flac")
```

### Streaming Mode

```go
// Real-time streaming with continuous writes
task := sox.New(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
    WithStream()

if err := task.Start(); err != nil {
    return err
}
defer task.Stop()

// Write audio chunks continuously
for packet := range rtpChannel {
    if _, err := task.Write(packet.Payload); err != nil {
        return err
    }
}
```

### Ticker Mode

```go
// Periodic batch processing (e.g., RTP recording)
task := sox.New(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
    WithOutputPath("/tmp/recording.flac").
    WithTicker(3 * time.Second)

if err := task.Start(); err != nil {
    return err
}
defer task.Stop()

// Write packets - conversion happens every 3 seconds
for packet := range rtpChannel {
    task.Write(packet.Payload)
}
```

## API Reference

### Task Modes

| Mode | Use Case | Process Lifetime | Memory | Output |
|------|----------|------------------|--------|--------|
| **Convert** | Batch, one-shot conversions | One conversion per process | Minimal | Entire result after conversion |
| **Stream** | Real-time streaming | Single process handles multiple writes | Constant | Continuous via Read() |
| **Ticker** | Periodic batch processing | Process per tick interval | Constant | Incremental writes to file |

See [Unified API Documentation](docs/UNIFIED_API.md) for complete API details.

## Performance

### Throughput

- **Small files (100ms)**: ~200 conversions/second
- **Medium files (1s)**: ~200 conversions/second  
- **Large files (5s)**: ~40 conversions/second

### Latency

- **PCM to FLAC (1s audio)**: ~5ms
- **PCM to WAV (1s audio)**: ~3ms
- **μ-law to FLAC (1s audio)**: ~5ms

### Scalability

- Each conversion uses ~150KB heap
- Streaming mode: ~32KB constant memory
- Supports hundreds of concurrent conversions (limited by system resources)

## Architecture & Design Decisions

### Why Pipes Over Files?

This library uses stdin/stdout pipes instead of temporary files because:

- **Performance**: Eliminates disk I/O overhead
- **Latency**: No file system round-trips
- **Memory**: Streams data without buffering entire files
- **Scalability**: Works with very large files without memory issues

### Conversion Flow

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   Reader    │────▶│   SoX Pipe   │────▶│   Writer    │
│  (stdin)    │     │   Process    │     │  (stdout)   │
└─────────────┘     └──────────────┘     └─────────────┘
                           │
                    ┌──────┴──────┐
                    │   Circuit   │
                    │   Breaker   │
                    └─────────────┘
```

### Path Mode Optimization

When both input and output are file paths, go-sox automatically uses direct file access (no piping):

```go
// Optimized: Direct file access
task.Convert("input.pcm", "output.flac")
// Executes: sox input.pcm output.flac

// Pipe mode: Uses stdin/stdout
task.Convert(inputReader, outputWriter)
// Executes: sox - input.pcm | sox - output.flac
```

### Circuit Breaker Pattern

The circuit breaker prevents cascading failures when SoX is unavailable:
- **Closed**: Normal operation, requests pass through
- **Open**: After 5 failures (configurable), rejects requests immediately
- **Half-Open**: After reset timeout (default 10s), allows limited requests to test recovery

### Retry Strategy

Automatic retry with exponential backoff:
- **Default**: 3 attempts with 100ms-5s backoff
- **Configurable**: Adjust attempts, initial backoff, max backoff, multiplier
- **Smart**: Doesn't retry on format errors or circuit breaker open

## Core Features

### Resiliency: Built In

All converters include three layers of protection:

#### 1. Automatic Retry with Exponential Backoff

```go
// Default: 3 attempts with 100ms-5s backoff
// Customizable:
retryConfig := sox.RetryConfig{
    MaxAttempts:     5,
    InitialBackoff:  50 * time.Millisecond,
    MaxBackoff:      10 * time.Second,
    BackoffMultiple: 2.0,
}
task := sox.New(input, output).
    WithRetryConfig(retryConfig)
```

#### 2. Circuit Breaker Pattern

```go
// Default: opens after 5 failures, resets after 10s
// Customizable:
breaker := sox.NewCircuitBreakerWithConfig(
    10,              // max failures
    15 * time.Second, // reset timeout
    5,               // half-open requests
)
task := sox.New(input, output).
    WithCircuitBreaker(breaker)
```

### Timeout Support

Prevent conversions from hanging indefinitely:

```go
// Using options
opts := sox.DefaultOptions()
opts.Timeout = 10 * time.Second
task := sox.New(input, output).WithOptions(opts)
task.Convert(input, output) // enforces 10s timeout

// Using context
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
task.ConvertWithContext(ctx, input, output) // cancellation propagates
```

## Usage Patterns

### Pattern 1: Batch Conversion

```go
func convertBatch(files []string) error {
    task := sox.New(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO)

    for _, file := range files {
        outputFile := strings.TrimSuffix(file, ".pcm") + ".flac"
        if err := task.Convert(file, outputFile); err != nil {
            return err
        }
    }
    return nil
}
```

### Pattern 2: API Endpoint

```go
func handleAudioConvert(w http.ResponseWriter, r *http.Request) {
    task := sox.New(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO)
    
    output := &bytes.Buffer{}
    if err := task.Convert(r.Body, output); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    w.Header().Set("Content-Type", "audio/flac")
    w.Write(output.Bytes())
}
```

### Pattern 3: RTP/Live Stream Recording

```go
func recordRTPStream(rtpChan chan []byte, outputFile string) error {
    task := sox.New(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
        WithOutputPath(outputFile).
        WithTicker(5 * time.Second) // flush every 5 seconds
    
    if err := task.Start(); err != nil {
        return err
    }
    defer task.Stop()
    
    for {
        select {
        case packet, ok := <-rtpChan:
            if !ok {
                return nil
            }
            if _, err := task.Write(packet); err != nil {
                return err
            }
        case <-time.After(30 * time.Second):
            return nil
        }
    }
}
```

### Pattern 4: Disable Resiliency (Not Recommended)

Only disable resiliency for testing or when you're implementing your own:

```go
task := sox.New(input, output).
    DisableResilience() // no retry, no circuit breaker, single attempt
```

## Audio Formats

### Format Presets

Commonly used formats are pre-configured:

```go
// PCM Raw formats (telephony/streaming)
sox.PCM_RAW_8K_MONO   // 8kHz mono (G.711 compatible)
sox.PCM_RAW_16K_MONO  // 16kHz mono (popular)

// FLAC (lossless)
sox.FLAC_16K_MONO_LE  // 16kHz mono, little-endian

// WAV (uncompressed)
sox.WAV_16K_MONO      // 16kHz mono
sox.WAV_8K_MONO_LE    // 8kHz mono, little-endian

// Telephony
sox.ULAW_8K_MONO      // G.711 μ-law 8kHz
```

### Custom Formats

Use any SoX-supported format:

```go
input := sox.AudioFormat{
    Type:         "raw",
    Encoding:     "signed-integer",
    SampleRate:   16000,
    Channels:     1,
    BitDepth:     16,
    IgnoreLength: true,  // useful for streaming
    Endian:       "little",
    Volume:       1.5,
}

output := sox.AudioFormat{
    Type:        "flac",
    SampleRate:  16000,
    Channels:    1,
    BitDepth:    16,
    Compression: 8.0,
    Comment:     "Auto-generated recording",
}

task := sox.New(input, output)
```

## Options

### Conversion Options

Control SoX behavior during conversions:

```go
opts := sox.DefaultOptions()

// I/O settings
opts.BufferSize = 64 * 1024  // 64KB I/O buffer
opts.Timeout = 30 * time.Second

// Quality/Compression
opts.CompressionLevel = 8    // FLAC compression (0-8)
opts.Quality = 5             // Lossy format quality (0-10)

// Effects
opts.Effects = []string{"norm", "-3"}  // normalize then compress 3dB
opts.Effects = []string{"highpass", "100"} // remove sub-100Hz

// Global SoX options
opts.NoDither = true         // disable dithering
opts.Guard = true            // guard against clipping
opts.Norm = true             // guard & normalize
opts.VerbosityLevel = 2      // debug output

// SoX binary location
opts.SoxPath = "/usr/local/bin/sox"

task := sox.New(input, output).WithOptions(opts)
```

See [ADVANCED_OPTIONS.md](docs/ADVANCED_OPTIONS.md) for complete documentation.

## Production Deployment

### Version Compatibility

| go-sox | Go Version | SoX Version |
|--------|------------|-------------|
| 1.x    | 1.21+      | 14.4+       |

### Docker/Kubernetes

```dockerfile
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y sox
```

For detailed production deployment instructions, resource limits, monitoring, and troubleshooting, see [PRODUCTION.md](docs/PRODUCTION.md).

### Requirements

- **SoX 14.4+** - installed and in `$PATH` or custom path via `ConversionOptions.SoxPath`
- **Go 1.21+** - for generics support and modern context handling

## Troubleshooting

### Common Issues

#### "sox: command not found"

```bash
# Install SoX
brew install sox  # macOS

# Or specify path explicitly
task := sox.New(input, output)
opts := sox.DefaultOptions()
opts.SoxPath = "/usr/local/bin/sox"
task.WithOptions(opts)
```

#### Conversion Timeout

Increase timeout or check system resources:

```go
opts := sox.DefaultOptions()
opts.Timeout = 60 * time.Second // increase from default
task.WithOptions(opts)
```

#### Resource Exhaustion

Manage conversion concurrency through goroutine workers or semaphores to control throughput.

See [PRODUCTION.md](docs/PRODUCTION.md) for detailed resource management guidance.

#### Memory Issues

Each SoX process uses ~2-5MB RAM. For 500 concurrent:
- Expected RAM: 1-2.5GB
- Recommended: 4GB+ available

#### Orphaned Processes

Monitor for zombie processes:

```bash
# Check for zombie SoX processes
ps aux | grep 'sox' | grep '<defunct>'

# Kill orphaned processes
pkill -9 sox
```

In code, always use defer:

```go
task := sox.New(input, output).WithStream()
if err := task.Start(); err != nil {
    return err
}
defer task.Stop() // Always cleanup
```

## Examples

See `examples/` directory:

- `simple_conversion/` - Basic conversion examples
- `rtp_to_flac/` - RTP conversion patterns
- `sip_integration/` - Production SIP/RTP handler
- `streaming_realtime/` - Real-time streaming with concurrent I/O
- `ticker_conversion/` - Periodic batch processing
- `advanced_options/` - Advanced configuration examples

## Testing

```bash
# Run all tests
make test

# Run benchmarks
make bench

# Run with coverage
make coverage

# Run quality checks
make check
```

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

- Tests pass: `make test`
- Benchmarks don't regress: `make bench`
- Code follows Go conventions: `make fmt` and `make lint`
- Follow [Conventional Commits](https://www.conventionalcommits.org/) for commit messages

## Security

Please see [SECURITY.md](SECURITY.md) for security policy and reporting vulnerabilities.

## License

MIT
