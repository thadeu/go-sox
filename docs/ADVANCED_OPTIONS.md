# Advanced SoX Options

This document describes the advanced options available in go-sox that provide full access to all SoX parameters.

## Overview

go-sox now supports all SoX format options (fopts) and global options (gopts) without discriminating file types. This gives you complete flexibility to use any SoX parameter available in the underlying SoX library.

## Extended Format Options

The `AudioFormat` struct now includes extended options that map directly to SoX format parameters:

```go
type AudioFormat struct {
    // Basic format parameters
    Type       string
    Encoding   string
    SampleRate int
    Channels   int
    BitDepth   int

    // Extended format options
    Volume          float64  // -v|--volume FACTOR
    IgnoreLength    bool     // --ignore-length
    ReverseNibbles  bool     // -N|--reverse-nibbles
    ReverseBits     bool     // -X|--reverse-bits
    Endian          string   // --endian little|big|swap
    Compression     float64  // -C|--compression FACTOR
    Comment         string   // --comment TEXT
    AddComment      string   // --add-comment TEXT
    CommentFile     string   // --comment-file FILENAME
    NoGlob          bool     // --no-glob
    
    // Full flexibility
    CustomArgs      []string // Any additional SoX arguments
}
```

### Examples

#### Volume Adjustment

```go
input := sox.AudioFormat{
    Type:       "raw",
    Encoding:   "signed-integer",
    SampleRate: 16000,
    Channels:   1,
    BitDepth:   16,
    Volume:     1.5, // Increase volume by 50%
}
```

#### Compression

```go
output := sox.AudioFormat{
    Type:        "flac",
    SampleRate:  16000,
    Channels:    1,
    BitDepth:    16,
    Compression: 8.0, // Maximum compression for FLAC
}
```

#### Endian

```go
format := sox.AudioFormat{
    Type:       "raw",
    Encoding:   "signed-integer",
    SampleRate: 16000,
    Channels:   1,
    BitDepth:   16,
    Endian:     "little", // little, big, or swap
}
```

#### Custom Arguments

```go
format := sox.AudioFormat{
    Type:       "flac",
    SampleRate: 16000,
    Channels:   1,
    BitDepth:   16,
    // Pass any SoX parameter not explicitly defined
    CustomArgs: []string{"--add-comment", "Custom metadata"},
}
```

## Global Options

The `ConversionOptions` struct now includes all SoX global options:

```go
type ConversionOptions struct {
    // Basic options
    SoxPath          string
    BufferSize       int
    Effects          []string
    ShowProgress     bool
    Verbose          bool
    Timeout          time.Duration

    // Global SoX options
    Buffer           int      // --buffer BYTES
    NoClobber        bool     // --no-clobber
    Clobber          bool     // --clobber
    CombineMode      string   // --combine concatenate|sequence|mix|mix-power|merge|multiply
    NoDither         bool     // -D|--no-dither
    DftMin           int      // --dft-min NUM
    EffectsFile      string   // --effects-file FILENAME
    Guard            bool     // -G|--guard
    InputBuffer      int      // --input-buffer BYTES
    Norm             bool     // --norm
    PlayRateArg      string   // --play-rate-arg ARG
    Plot             string   // --plot gnuplot|octave
    ReplayGain       string   // --replay-gain track|album|off
    RandomNumbers    bool     // -R
    SingleThreaded   bool     // --single-threaded
    TempDirectory    string   // --temp DIRECTORY
    VerbosityLevel   int      // -V[LEVEL]
    
    // Full flexibility
    CustomGlobalArgs []string // Any additional global SoX arguments
}
```

### Examples

#### Buffer Configuration

```go
options := sox.ConversionOptions{
    Buffer:      16384, // Set buffer size
    InputBuffer: 32768, // Override input buffer size
}
```

#### Dithering and Guard

```go
options := sox.ConversionOptions{
    NoDither: true,  // Disable dithering
    Guard:    true,  // Guard against clipping
}
```

#### Normalization

```go
options := sox.ConversionOptions{
    Norm: true, // Guard and normalize
}
```

#### Verbosity

```go
options := sox.ConversionOptions{
    VerbosityLevel: 3, // 1-6, higher = more verbose
}
```

## Complete Example

Here's a complete example using all advanced features:

```go
package main

import (
    "bytes"
    "log"
    sox "github.com/thadeu/go-sox"
)

func main() {
    // Input with extended options
    input := sox.AudioFormat{
        Type:         "raw",
        Encoding:     "signed-integer",
        SampleRate:   8000,
        Channels:     1,
        BitDepth:     16,
        Volume:       2.0,     // Double the volume
        IgnoreLength: true,    // Ignore length in header
        Endian:       "little", // Little-endian byte order
    }

    // Output with extended options
    output := sox.AudioFormat{
        Type:        "flac",
        SampleRate:  16000, // Upsample
        Channels:    2,     // Convert to stereo
        BitDepth:    24,    // Increase bit depth
        Compression: 8.0,   // Maximum FLAC compression
        Comment:     "Processed with go-sox",
    }

    // Global options
    options := sox.ConversionOptions{
        SoxPath:      "sox",
        ShowProgress: false,
        Buffer:       32768,
        NoDither:     false,
        Guard:        true,
        Effects: []string{
            "channels", "2", // Convert mono to stereo
            "gain", "-n",    // Normalize
        },
    }

    // Create converter
    converter := sox.NewConverter(input, output).
        WithOptions(options).
        WithRetryConfig(sox.RetryConfig{
            MaxAttempts: 3,
        })

    // Convert
    pcmData := []byte{/* your PCM data */}
    inputReader := bytes.NewReader(pcmData)
    outputBuffer := &bytes.Buffer{}

    if err := converter.Convert(inputReader, outputBuffer); err != nil {
        log.Fatal(err)
    }
}
```

## Backward Compatibility

All existing code continues to work without any changes. The new options are optional and have sensible defaults:

- Volume defaults to 0 (no adjustment)
- IgnoreLength defaults to false
- Endian defaults to empty (SoX default)
- CustomArgs defaults to empty slice
- All global options default to their SoX defaults

## Flexibility with CustomArgs

If you need to use a SoX parameter that's not explicitly defined, you can always use `CustomArgs` in `AudioFormat` or `CustomGlobalArgs` in `ConversionOptions`:

```go
format := sox.AudioFormat{
    Type:       "ogg",
    CustomArgs: []string{"--replay-gain", "track"},
}

options := sox.ConversionOptions{
    CustomGlobalArgs: []string{"--single-threaded"},
}
```

This ensures that go-sox supports any current or future SoX parameter without requiring code changes.

## SoX Documentation Reference

For complete documentation on all SoX parameters, run:

```bash
sox --help
sox --help-format all
sox --help-effect all
```

Or visit the SoX documentation: http://sox.sourceforge.net/

## See Also

- [Usage Guide](USAGE.md)
- [Production Deployment](PRODUCTION.md)
- [Auto Flush](AUTO_FLUSH.md)

