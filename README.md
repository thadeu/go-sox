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

### One-Shot Conversion

```go
import sox "github.com/thadeu/go-sox"

converter := sox.NewConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO)
err := converter.Convert(inputReader, outputWriter)
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
type AudioHandler struct {
    stream *sox.StreamConverter
}

func (h *AudioHandler) ProcessRTPPacket(pcmData []byte) error {
    _, err := h.stream.Write(pcmData)
    if h.accumulated >= threshold {
        flacData, _ := h.stream.Flush()
        go sendToWhisper(flacData)
        
        // Reset for next batch
        h.stream = sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO)
        h.stream.Start()
    }
    return err
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

## Testing

```bash
go test -v              # Run tests
go test -bench=.        # Run benchmarks
make test              # Using Makefile
```

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

