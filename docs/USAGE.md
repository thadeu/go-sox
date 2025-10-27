# go-sox Usage Guide

High-performance Go wrapper for SoX CLI with pipe/stream support.

## Installation

```bash
# Install SoX
brew install sox  # macOS
apt-get install sox  # Ubuntu/Debian

# Install go-sox
go get github.com/thadeu/go-sox
```

## Quick Start

### One-Shot Conversion

```go
import sox "github.com/thadeu/go-sox"

converter := sox.NewConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO)
err := converter.Convert(inputReader, outputWriter)
```

### Streaming Conversion (RTP Use Case)

```go
stream := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO)
stream.Start()

// Write chunks as they arrive
stream.Write(rtpPacket1)
stream.Write(rtpPacket2)

// Get converted output
flacData, err := stream.Flush()
```

### Streaming with Auto-Flush (Real-time Processing)

For scenarios where you need to read the output file while it's being written (e.g., real-time transcription):

```go
stream := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
    WithOutputPath("output.flac").
    WithAutoFlush(3 * time.Second)  // Flush every 3 seconds

stream.Start()

// Write RTP packets continuously
for {
    stream.Write(rtpPacket)
    // File grows incrementally - other processes can read it
}

// Final flush when done
stream.Flush()
```

**Key Benefits:**
- File grows incrementally every 3 seconds
- Other processes can read the file while it's being written
- SoX process stays alive for continuous streaming
- Perfect for real-time transcription pipelines

**Note:** `WithAutoFlush` requires `WithOutputPath` to be set.

## Format Presets

- `PCM_RAW_8K_MONO` - 8kHz mono PCM (telephony)
- `PCM_RAW_16K_MONO` - 16kHz mono PCM (speech recognition)
- `PCM_RAW_48K_MONO` - 48kHz mono PCM (high quality)
- `FLAC_16K_MONO` - 16kHz mono FLAC (transcription)
- `FLAC_44K_STEREO` - 44.1kHz stereo FLAC (CD quality)
- `WAV_16K_MONO` - 16kHz mono WAV
- `ULAW_8K_MONO` - G.711 μ-law (telephony)

## Custom Format

```go
customFormat := sox.AudioFormat{
    Type:       "raw",
    Encoding:   "signed-integer",
    SampleRate: 8000,
    Channels:   1,
    BitDepth:   16,
}
```

## Options

```go
opts := sox.DefaultOptions()
opts.CompressionLevel = 8      // FLAC compression (0-8)
opts.BufferSize = 64 * 1024    // I/O buffer size
opts.Effects = []string{"norm"} // Audio effects

converter.WithOptions(opts)
```

## Performance

Benchmarks (Apple M2, 1s audio):
- SoX: ~5ms per conversion
- FFmpeg: ~40ms per conversion
- **~8x faster than FFmpeg**

## RTP/SIP Integration

See `examples/sip_integration.go` for complete RTP → FLAC → Transcription pipeline.

Key pattern:
1. Receive RTP packets (PCM Raw)
2. Stream to SoX converter
3. Accumulate until threshold (e.g., 3 seconds)
4. Flush to get FLAC output
5. Send to transcription API (Whisper, DeepInfra)

## API Reference

### Converter

```go
type Converter struct {
    Input   AudioFormat
    Output  AudioFormat
    Options ConversionOptions
}

func NewConverter(input, output AudioFormat) *Converter
func (c *Converter) Convert(input io.Reader, output io.Writer) error
func (c *Converter) ConvertFile(inputPath, outputPath string) error
func (c *Converter) WithOptions(opts ConversionOptions) *Converter
```

### StreamConverter

```go
type StreamConverter struct { ... }

func NewStreamConverter(input, output AudioFormat) *StreamConverter
func (s *StreamConverter) Start() error
func (s *StreamConverter) Write(data []byte) (int, error)
func (s *StreamConverter) Read(p []byte) (int, error)
func (s *StreamConverter) Available() int
func (s *StreamConverter) Flush() ([]byte, error)
func (s *StreamConverter) Close() error
func (s *StreamConverter) WithOptions(opts ConversionOptions) *StreamConverter
```

### Utilities

```go
func CheckSoxInstalled(soxPath string) error
func DefaultOptions() ConversionOptions
```

## Examples

Run examples:

```bash
cd examples
go run rtp_to_flac.go        # Multiple conversion examples
go run sip_integration.go    # RTP handler pattern
```

## Testing

```bash
go test -v ./...              # Run tests
go test -bench=. ./...        # Run benchmarks
```

