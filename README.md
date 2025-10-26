# go-sox

High-performance Go wrapper for SoX CLI using pipes and streams for audio format conversion.

## Overview

This library provides a Go interface to SoX (Sound eXchange) that eliminates file I/O overhead by using stdin/stdout pipes. It's designed for production environments where audio conversion performance matters.

Primary use case: converting RTP media streams (PCM Raw) to compressed formats (FLAC) for transcription APIs with minimal latency.

## Performance

Benchmarks on Apple M2 with 1 second audio (16kHz mono PCM to FLAC):

```
BenchmarkConverter_Convert-8         244      4870383 ns/op      91747 B/op    148 allocs/op
BenchmarkStreamConverter-8           243      4886878 ns/op      98629 B/op    129 allocs/op
BenchmarkFFmpegComparison-8           28     41985972 ns/op      23525 B/op     99 allocs/op
```

Results:
- SoX Converter: 4.87ms per conversion
- SoX Stream: 4.88ms per conversion
- FFmpeg (file I/O): 42ms per conversion

**SoX with pipes is 8.6x faster than ffmpeg** for typical conversions. The streaming approach maintains the same performance while allowing incremental processing of RTP packets.

## Installation

```bash
# Install SoX
brew install sox  # macOS
apt-get install sox  # Debian/Ubuntu

# Install library
go get github.com/thadeu/go-sox
```

## Usage

### Basic Usage

```go
import sox "github.com/thadeu/go-sox"

// NewConverter is resilient by default (includes circuit breaker & retry)
converter := sox.NewConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO)
err := converter.Convert(inputReader, outputWriter)

// Add worker pool (creates default pool if no arg)
converter := sox.NewConverter(input, output).
    WithPool() // Creates pool with SOX_MAX_WORKERS (default: 500)

// Or use custom pool
pool := sox.NewPool()
converter := sox.NewConverter(input, output).
    WithPool(pool)
```

### Streaming Conversion

For scenarios where audio data arrives incrementally (RTP packets, live streams):

```go
stream := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO)
stream.Start()

// Write audio chunks as they arrive
for packet := range rtpPackets {
    stream.Write(packet.AudioData)
}

// Get converted output
flacData, err := stream.Flush()
```

### Production Example: RTP to Transcription

```go
func ProcessRTPBatch(pcmData []byte) error {
    // WithPool() creates default pool automatically
    converter := sox.NewConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
        WithPool()
    
    input := bytes.NewReader(pcmData)
    output := &bytes.Buffer{}
    
    // Automatic retry + circuit breaker + pool concurrency control
    if err := converter.Convert(input, output); err != nil {
        return err
    }
    
    return sendToWhisper(output.Bytes())
}
```

## Format Presets

Common formats included:

- `PCM_RAW_8K_MONO`, `PCM_RAW_16K_MONO`, `PCM_RAW_48K_MONO`
- `FLAC_16K_MONO`, `FLAC_44K_STEREO`
- `WAV_16K_MONO`
- `ULAW_8K_MONO`

Custom formats supported via `AudioFormat` struct.

## Options

```go
opts := sox.DefaultOptions()
opts.CompressionLevel = 8      // FLAC compression level
opts.BufferSize = 64 * 1024    // I/O buffer size
opts.Effects = []string{"norm"} // SoX effects chain

converter.WithOptions(opts)
```

## Production Features

**NewConverter is production-ready by default** - includes circuit breaker, retry, and timeout support.

### Worker Pool

Limit concurrent SoX processes to prevent resource exhaustion:

```go
// Option 1: Let converter create default pool
converter := sox.NewConverter(input, output).
    WithPool() // Creates pool with SOX_MAX_WORKERS (default: 500)

// Option 2: Share pool across multiple converters
pool := sox.NewPool() // or NewPoolWithLimit(100)
converter1 := sox.NewConverter(input1, output1).WithPool(pool)
converter2 := sox.NewConverter(input2, output2).WithPool(pool)
```

### Customizing Resiliency

Defaults are production-ready, but you can customize:

```go
converter := sox.NewConverter(input, output).
    WithRetryConfig(sox.RetryConfig{
        MaxAttempts:     3,
        InitialBackoff:  100 * time.Millisecond,
        MaxBackoff:      5 * time.Second,
        BackoffMultiple: 2.0,
    }).
    WithCircuitBreaker(sox.NewCircuitBreakerWithConfig(
        5,                  // maxFailures
        10 * time.Second,   // resetTimeout  
        3,                  // halfOpenRequests
    )).
    WithPool(pool)
```

### Disabling Resiliency (not recommended)

```go
converter := sox.NewConverter(input, output).
    DisableResilience()
```

### Resource Monitoring

Track active processes and conversion statistics:

```go
monitor := sox.GetMonitor()
stats := monitor.GetStats()

fmt.Printf("Active processes: %d\n", stats.ActiveProcesses)
fmt.Printf("Success rate: %.2f%%\n", stats.SuccessRate)
fmt.Printf("Total conversions: %d\n", stats.TotalConversions)
```

### Timeout Support

Prevent hung conversions:

```go
opts := sox.DefaultOptions()
opts.Timeout = 10 * time.Second
converter.WithOptions(opts)

// Or use context
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
converter.ConvertWithContext(ctx, input, output)
```

## Testing

```bash
go test -v              # Run tests
go test -bench=.        # Run benchmarks
go test -v -run TestPooled  # Test concurrent load
make test              # Using Makefile
```

## Production Deployment

For high-load scenarios (100+ parallel conversions), you must configure system limits:

```bash
# Set worker pool limit
export SOX_MAX_WORKERS=500

# Increase Linux ulimits (see PRODUCTION.md)
ulimit -n 65536  # File descriptors
ulimit -u 4096   # Processes
```

See [PRODUCTION.md](PRODUCTION.md) for detailed deployment instructions including:
- Linux ulimit persistence across reboots
- Docker/Kubernetes configuration
- Systemd service limits
- Monitoring and troubleshooting

## Requirements

- SoX installed and in PATH
- Go 1.21 or later

## Architecture

The library spawns SoX processes and communicates via pipes:

```
Input Reader → stdin → SoX Process → stdout → Output Writer
```

For streaming, the SoX process remains alive, accepting writes until explicitly flushed.

## Examples

See `examples/` directory:

- `rtp_to_flac.go` - Various conversion patterns
- `sip_integration.go` - Production RTP handler implementation

## License

MIT

