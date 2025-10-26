package sox

import (
	"fmt"
	"time"
)

// ConversionOptions provides additional options for audio conversion
type ConversionOptions struct {
	// SoxPath specifies the path to the sox binary (defaults to "sox")
	SoxPath string

	// BufferSize sets the buffer size for I/O operations (defaults to 32KB)
	BufferSize int

	// Effects contains additional SoX effects to apply during conversion
	// Example: []string{"norm", "-3"} for normalization
	Effects []string

	// Quality sets compression quality for lossy formats (0-10, higher is better)
	// Only applicable for certain formats like MP3, OGG
	Quality int

	// CompressionLevel sets compression level for lossless formats like FLAC (0-8)
	CompressionLevel int

	// ShowProgress enables progress output from SoX (written to stderr)
	ShowProgress bool

	// Verbose enables verbose output from SoX for debugging
	Verbose bool

	// Timeout sets maximum duration for conversion (0 = no timeout)
	Timeout time.Duration
}

// DefaultOptions returns ConversionOptions with sensible defaults
func DefaultOptions() ConversionOptions {
	return ConversionOptions{
		SoxPath:          "sox",
		BufferSize:       32 * 1024, // 32KB
		Quality:          -1,        // not set
		CompressionLevel: -1,        // not set
		ShowProgress:     false,
		Verbose:          false,
	}
}

// buildGlobalArgs converts ConversionOptions to SoX global arguments
func (o *ConversionOptions) buildGlobalArgs() []string {
	var args []string

	if !o.ShowProgress {
		args = append(args, "-q") // quiet mode
	}

	if o.Verbose {
		args = append(args, "-V")
	}

	return args
}

// buildEffectArgs converts effects to SoX effect arguments
func (o *ConversionOptions) buildEffectArgs() []string {
	if len(o.Effects) == 0 {
		return nil
	}
	return o.Effects
}

// buildFormatArgs adds format-specific compression/quality arguments
func (o *ConversionOptions) buildFormatArgs(format *AudioFormat) []string {
	var args []string

	// Compression level for FLAC
	if format.Type == "flac" && o.CompressionLevel >= 0 {
		args = append(args, "-C", fmt.Sprintf("%d", o.CompressionLevel))
	}

	// Quality for MP3/OGG
	if (format.Type == "mp3" || format.Type == "ogg") && o.Quality >= 0 {
		args = append(args, "-q", fmt.Sprintf("%d", o.Quality))
	}

	return args
}
