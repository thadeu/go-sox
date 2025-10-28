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

	// Global SoX options (gopts)
	Buffer         int    // --buffer BYTES - Set the size of all processing buffers (default 8192)
	NoClobber      bool   // --no-clobber - Prompt to overwrite output file
	Clobber        bool   // --clobber - Don't prompt to overwrite output file (default)
	CombineMode    string // --combine concatenate|sequence|mix|mix-power|merge|multiply
	NoDither       bool   // -D|--no-dither - Don't dither automatically
	DftMin         int    // --dft-min NUM - Minimum size (log2) for DFT processing (default 10)
	EffectsFile    string // --effects-file FILENAME - File containing effects and options
	Guard          bool   // -G|--guard - Use temporary files to guard against clipping
	InputBuffer    int    // --input-buffer BYTES - Override the input buffer size
	Norm           bool   // --norm - Guard & normalise
	PlayRateArg    string // --play-rate-arg ARG - Default rate argument for auto-resample
	Plot           string // --plot gnuplot|octave - Generate script to plot response of filter effect
	ReplayGain     string // --replay-gain track|album|off - Default: off (sox, rec), track (play)
	RandomNumbers  bool   // -R - Use default random numbers (same on each run)
	SingleThreaded bool   // --single-threaded - Disable parallel effects channels processing
	TempDirectory  string // --temp DIRECTORY - Specify the directory for temporary files
	VerbosityLevel int    // -V[LEVEL] - Verbosity level (1-6)

	// CustomGlobalArgs allows passing any additional global SoX arguments
	// This provides full flexibility to use any SoX global parameter
	// Example: []string{"--help-effect", "reverb"}
	CustomGlobalArgs []string
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

// BuildGlobalArgs converts ConversionOptions to SoX global arguments
// Supports all SoX global options
func (o *ConversionOptions) BuildGlobalArgs() []string {
	var args []string

	// Buffer size
	if o.Buffer > 0 {
		args = append(args, "--buffer", fmt.Sprintf("%d", o.Buffer))
	}

	// Clobber/NoClobber
	if o.NoClobber {
		args = append(args, "--no-clobber")
	} else if o.Clobber {
		args = append(args, "--clobber")
	}

	// Combine mode
	if o.CombineMode != "" {
		args = append(args, "--combine", o.CombineMode)
	}

	// No dither
	if o.NoDither {
		args = append(args, "-D")
	}

	// DFT minimum
	if o.DftMin > 0 {
		args = append(args, "--dft-min", fmt.Sprintf("%d", o.DftMin))
	}

	// Effects file
	if o.EffectsFile != "" {
		args = append(args, "--effects-file", o.EffectsFile)
	}

	// Guard
	if o.Guard {
		args = append(args, "-G")
	}

	// Input buffer
	if o.InputBuffer > 0 {
		args = append(args, "--input-buffer", fmt.Sprintf("%d", o.InputBuffer))
	}

	// Norm
	if o.Norm {
		args = append(args, "--norm")
	}

	// Play rate arg
	if o.PlayRateArg != "" {
		args = append(args, "--play-rate-arg", o.PlayRateArg)
	}

	// Plot
	if o.Plot != "" {
		args = append(args, "--plot", o.Plot)
	}

	// Show progress (or quiet mode)
	if !o.ShowProgress {
		args = append(args, "-q") // quiet mode
	} else {
		args = append(args, "-S")
	}

	// Replay gain
	if o.ReplayGain != "" {
		args = append(args, "--replay-gain", o.ReplayGain)
	}

	// Random numbers
	if o.RandomNumbers {
		args = append(args, "-R")
	}

	// Single threaded
	if o.SingleThreaded {
		args = append(args, "--single-threaded")
	}

	// Temp directory
	if o.TempDirectory != "" {
		args = append(args, "--temp", o.TempDirectory)
	}

	// Verbosity level
	if o.VerbosityLevel > 0 {
		args = append(args, fmt.Sprintf("-V%d", o.VerbosityLevel))
	} else if o.Verbose {
		args = append(args, "-V")
	}

	// Custom global arguments
	if len(o.CustomGlobalArgs) > 0 {
		args = append(args, o.CustomGlobalArgs...)
	}

	if o.CompressionLevel >= 0 {
		args = append(args, "-C", fmt.Sprintf("%d", o.CompressionLevel))
	}

	if o.Quality >= 0 {
		args = append(args, "-q", fmt.Sprintf("%d", o.Quality))
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
