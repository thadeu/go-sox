# go-sox

![Build Status](https://github.com/thadeu/go-sox/actions/workflows/go-test.yml/badge.svg)
[![Go Reference](https://pkg.go.dev/badge/github.com/thadeu/go-sox.svg)](https://pkg.go.dev/github.com/thadeu/go-sox)

High-performance Go wrapper for SoX CLI using pipes and streams for audio format conversion.

## Overview

This library provides a Go interface to SoX (Sound eXchange) that eliminates file I/O overhead by using stdin/stdout pipes. It's designed for production environments where audio conversion performance matters.

Primary use case: converting RTP media streams (PCM Raw) to compressed formats (FLAC) for transcription APIs with minimal latency.

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

### Production-Ready by Default

Every converter includes:
- **Circuit breaker** - prevents cascading failures
- **Automatic retry** - exponential backoff on transient errors
- **Worker pool** - limits concurrent SoX processes
- **Timeout support** - prevents hung conversions
- **Process monitoring** - track active processes and statistics

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

### Converter: One-Shot Conversions

Perfect for batch processing or individual conversions:

```go
import sox "github.com/thadeu/go-sox"

// NewConverter is production-ready by default (circuit breaker, retry, timeout)
converter := sox.NewConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO)

// Convert from reader to writer
input := bytes.NewReader(pcmData)
output := &bytes.Buffer{}
err := converter.Convert(input, output)

// Or convert files
err := converter.ConvertFile("input.pcm", "output.flac")
```

### Streamer: Real-Time Streaming

Perfect for processing continuous streams (RTP, WebRTC, live audio):

```go
// Auto-start with flush every 3 seconds
stream := sox.NewStreamer(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
    WithOutputPath("/tmp/stream.flac").
    WithAutoStart(3 * time.Second)

// Write audio chunks continuously
for packet := range rtpChannel {
    stream.Write(packet.Payload)
}

// Final flush and close
stream.End()
```

**Key Streamer Benefits:**
- File grows incrementally at each flush interval
- Other processes can read the file while it's being written
- Minimal memory footprint for long-running streams
- Perfect for real-time transcription pipelines

## Converter vs Streamer

| Feature | Converter | Streamer |
|---------|-----------|----------|
| Use Case | Batch, one-shot conversions | Continuous, real-time streaming |
| Process Lifetime | One conversion per process | Single process handles multiple writes |
| Memory | Minimal for single conversions | Constant regardless of stream length |
| Output | Entire result after conversion | Incremental writes to file |
| Ideal For | API endpoints, batch jobs | Live recording, RTP processing |

## Core Features

### Resiliency: Built In

All converters include three layers of protection:

#### 1. Automatic Retry with Exponential Backoff

```go
// Default: 3 attempts with 100ms-5s backoff
// Customizable:
converter := sox.NewConverter(input, output).
    WithRetryConfig(sox.RetryConfig{
        MaxAttempts:     5,
        InitialBackoff:  50 * time.Millisecond,
        MaxBackoff:      10 * time.Second,
        BackoffMultiple: 2.0,
    })
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
converter := sox.NewConverter(input, output).
    WithCircuitBreaker(breaker)
```

### Timeout Support

Prevent conversions from hanging indefinitely:

```go
// Using options
opts := sox.DefaultOptions()
opts.Timeout = 10 * time.Second
converter := sox.NewConverter(input, output).WithOptions(opts)
converter.Convert(input, output) // enforces 10s timeout

// Using context
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
converter.ConvertWithContext(ctx, input, output) // cancellation propagates
```

## Usage Patterns

### Pattern 1: Batch Conversion

```go
func convertBatch(files []string) error {
    converter := sox.NewConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO)

    for _, file := range files {
        outputFile := strings.TrimSuffix(file, ".pcm") + ".flac"
        if err := converter.ConvertFile(file, outputFile); err != nil {
            return err
        }
    }
    return nil
}
```

### Pattern 2: API Endpoint

```go
func handleAudioConvert(w http.ResponseWriter, r *http.Request) {
    converter := sox.NewConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO)
    
    output := &bytes.Buffer{}
    if err := converter.Convert(r.Body, output); err != nil {
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
    streamer := sox.NewStreamer(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
        WithOutputPath(outputFile).
        WithAutoStart(5 * time.Second) // flush every 5 seconds
    
    for {
        select {
        case packet, ok := <-rtpChan:
            if !ok {
                return streamer.End()
            }
            if _, err := streamer.Write(packet); err != nil {
                return err
            }
        case <-time.After(30 * time.Second):
            return streamer.End()
        }
    }
}
```

### Pattern 4: Disable Resiliency (Not Recommended)

Only disable resiliency for testing or when you're implementing your own:

```go
converter := sox.NewConverter(input, output).
    DisableResilience() // no retry, no circuit breaker, single attempt
```

## Audio Formats

### Format Presets

Commonly used formats are pre-configured:

```go
// PCM Raw formats (telephony/streaming)
sox.PCM_RAW_8K_MONO   // 8kHz mono (G.711 compatible)
sox.PCM_RAW_16K_MONO  // 16kHz mono (popular)
sox.PCM_RAW_48K_MONO  // 48kHz mono (professional)

// FLAC (lossless)
sox.FLAC_16K_MONO     // 16kHz mono
sox.FLAC_44K_STEREO   // 44.1kHz stereo

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
    Type:       "raw",
    Encoding:   "signed-integer",
    SampleRate: 16000,
    Channels:   1,
    BitDepth:   16,
    IgnoreLength: true,  // useful for streaming
    Endian:     "little",
    Volume:     1.5,
}

output := sox.AudioFormat{
    Type:        "flac",
    SampleRate:  16000,
    Channels:    1,
    BitDepth:    16,
    Compression: 8.0,
    Comment:     "Auto-generated recording",
}

converter := sox.NewConverter(input, output)
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

converter := sox.NewConverter(input, output).WithOptions(opts)
```

### Advanced Format Options

Full flexibility with extended format parameters:

```go
format := sox.AudioFormat{
    Type:           "raw",
    Encoding:       "signed-integer",
    SampleRate:     16000,
    Channels:       1,
    BitDepth:       16,
    Endian:         "little",
    Volume:         1.2,
    ReverseNibbles: false,
    ReverseBits:    false,
    IgnoreLength:   true,
    NoGlob:         true,
    CustomArgs:     []string{"--no-dither"},
}
```

See [ADVANCED_OPTIONS.md](docs/ADVANCED_OPTIONS.md) for complete documentation.

## Testing

```bash
go test -v              # Run all tests
go test -bench=.        # Run benchmarks
make test              # Using Makefile
```

## Production Deployment

For high-load scenarios, manage conversion throughput with concurrency control:

```bash
# Control throughput using goroutine workers or semaphores
# The go-sox library handles retries and circuit breaker automatically
```

### Docker/Kubernetes

```dockerfile
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y sox
```

For detailed production deployment instructions, monitoring, and troubleshooting, see [PRODUCTION.md](docs/PRODUCTION.md).

## Requirements

- **SoX 14.4+** - installed and in `$PATH` or custom path via `ConversionOptions.SoxPath`
- **Go 1.21+** - for generics support and modern context handling

## Architecture

### Converter Flow

```
Reader → stdin → SoX Process → stdout → Writer
```

Each conversion spawns a new SoX process, communicates via pipes, and cleans up after completion.

### Streamer Flow

```
Write() → Buffer → [Tick] → SoX Process → Output File (incremental)
                       ↓
                   Continues...
                       ↓
End() → Final Flush → Close Process
```

The SoX process remains alive during streaming, accepting incremental writes.

## Examples

See `examples/` directory:

- `rtp_to_flac.go` - Various RTP conversion patterns
- `sip_integration.go` - Production SIP/RTP handler
- `streaming_realtime.go` - Real-time streaming with concurrent I/O
- `circuit_breaker.go` - Resiliency patterns

## Troubleshooting

### "sox: command not found"

```bash
# Install SoX
brew install sox  # macOS

# Or specify path explicitly
converter := sox.NewConverter(input, output)
opts := sox.DefaultOptions()
opts.SoxPath = "/usr/local/bin/sox"
converter.WithOptions(opts)
```

### Conversion Timeout

Increase timeout or check system resources:

```go
opts := sox.DefaultOptions()
opts.Timeout = 60 * time.Second // increase from default
converter.WithOptions(opts)
```

### Resource Exhaustion

Manage conversion concurrency through goroutine workers or semaphores to control throughput.

## Contributing

Contributions are welcome! Please ensure:
- Tests pass: `go test -v`
- Benchmarks don't regress: `go test -bench=.`
- Code follows Go conventions

## License

MIT

