package sox

import "fmt"

const (
	TYPE_RAW            = "raw"
	TYPE_FLAC           = "flac"
	TYPE_WAV            = "wav"
	TYPE_MP3            = "mp3"
	TYPE_OGG            = "ogg"
	TYPE_M4A            = "m4a"
	TYPE_AAC            = "aac"
	TYPE_AC3            = "ac3"
	TYPE_EAC3           = "eac3"
	TYPE_ALAW           = "alaw"
	TYPE_IMA_ADPCM      = "ima-adpcm"
	TYPE_MS_ADPCM       = "ms-adpcm"
	TYPE_GSM_FULL_RATE  = "gsm-full-rate"
	TYPE_GSM_HALF_RATE  = "gsm-half-rate"
	TYPE_GSM_EFR_RATE   = "gsm-efr-rate"
	TYPE_GSM_FR_RATE    = "gsm-fr-rate"
	TYPE_GSM_HR_RATE    = "gsm-hr-rate"
	TYPE_GSM_MR_RATE    = "gsm-mr-rate"
	TYPE_GSM_SUPER_RATE = "gsm-super-rate"
	SIGNED_INTEGER      = "signed-integer"
	UNSIGNED_INTEGER    = "unsigned-integer"
	FLOATING_POINT      = "floating-point"
	MU_LAW              = "mu-law"
	A_LAW               = "a-law"
	IMA_ADPCM           = "ima-adpcm"
	MS_ADPCM            = "ms-adpcm"
	GSM_FULL_RATE       = "gsm-full-rate"
	GSM_HALF_RATE       = "gsm-half-rate"
)

// AudioFormat defines the audio format parameters for input or output
type AudioFormat struct {
	Type       string // "raw", "flac", "wav", "mp3", "ogg", etc.
	Encoding   string // "signed-integer", "unsigned-integer", "floating-point", "mu-law", "a-law", "ima-adpcm", "ms-adpcm", "gsm-full-rate"
	SampleRate int    // Sample rate in Hz (e.g., 8000, 16000, 44100, 48000)
	Channels   int    // Number of channels: 1 = mono, 2 = stereo
	BitDepth   int    // Bits per sample: 8, 16, 24, 32

	// Extended format options - supports all SoX format parameters
	Volume         float64 // -v|--volume FACTOR - Input file volume adjustment factor
	IgnoreLength   bool    // --ignore-length - Ignore input file length given in header
	ReverseNibbles bool    // -N|--reverse-nibbles - Encoded nibble-order
	ReverseBits    bool    // -X|--reverse-bits - Encoded bit-order
	Endian         string  // --endian little|big|swap - Encoded byte-order
	Compression    float64 // -C|--compression FACTOR - Compression factor
	Comment        string  // --comment TEXT - Comment text for the output file
	AddComment     string  // --add-comment TEXT - Append output file comment
	CommentFile    string  // --comment-file FILENAME - File containing comment text
	NoGlob         bool    // --no-glob - Don't glob wildcard match

	Pipe bool // -|--pipe - Pipe input to output (default: false)

	// CustomArgs allows passing any additional SoX arguments not covered above
	// This provides full flexibility to use any SoX parameter
	// Example: []string{"--replay-gain", "track", "--norm"}
	CustomArgs []string
}

type Options = AudioFormat

// Common audio format presets for convenience

var (
	// PCM_RAW_8K_MONO - PCM Raw 8kHz mono 16-bit (common for telephony)
	PCM_RAW_8K_MONO = AudioFormat{
		Type:       TYPE_RAW,
		Encoding:   "signed-integer",
		SampleRate: 8000,
		Channels:   1,
		BitDepth:   16,
	}

	FLAC_16K_MONO_LE = AudioFormat{
		Type:       "flac",
		Encoding:   "unsigned",
		Endian:     "little",
		SampleRate: 16000,
		Channels:   1,
		BitDepth:   16,
	}

	WAV_8K_MONO_LE = AudioFormat{
		Type:       "wav",
		Encoding:   "signed",
		Endian:     "little",
		SampleRate: 8000,
		Channels:   1,
		BitDepth:   8,
	}

	WAV_16K_MONO = AudioFormat{
		Type:       "wav",
		Encoding:   "signed-integer",
		SampleRate: 16000,
		Channels:   1,
		BitDepth:   16,
	}

	WAV_16K_MONO_LE = AudioFormat{
		Type:       "wav",
		Encoding:   "signed",
		Endian:     "little",
		SampleRate: 16000,
		Channels:   1,
		BitDepth:   16,
	}

	// ULAW_8K_MONO - G.711 μ-law 8kHz mono (telephony standard)
	ULAW_8K_MONO = AudioFormat{
		Type:       "raw",
		Encoding:   "mu-law",
		SampleRate: 8000,
		Channels:   1,
		BitDepth:   8,
	}
)

// BuildArgs converts AudioFormat to SoX command-line arguments
// Supports all SoX format options without discriminating file types
// isInput: true for input format, false for output format
func (f *AudioFormat) BuildArgs() []string {
	var args []string

	// Volume adjustment (input only)
	if f.Volume != 0 {
		args = append(args, "-v", fmt.Sprintf("%f", f.Volume))
	}

	// Ignore length (input only)
	if f.IgnoreLength {
		args = append(args, "--ignore-length")
	}

	// Type argument
	if f.Type != "" {
		args = append(args, "-t", f.Type)
	}

	// Encoding
	if f.Encoding != "" {
		args = append(args, "-e", f.Encoding)
	}

	// Bit depth
	if f.BitDepth > 0 {
		args = append(args, "-b", fmt.Sprintf("%d", f.BitDepth))
	}

	// Reverse nibbles
	if f.ReverseNibbles {
		args = append(args, "-N")
	}

	// Reverse bits
	if f.ReverseBits {
		args = append(args, "-X")
	}

	// Endian
	if f.Endian != "" {
		args = append(args, "--endian", f.Endian)
	}

	// Channels
	if f.Channels > 0 {
		args = append(args, "-c", fmt.Sprintf("%d", f.Channels))
	}

	// Sample rate
	if f.SampleRate > 0 {
		args = append(args, "-r", fmt.Sprintf("%d", f.SampleRate))
	}

	// Compression (output only)
	if f.Compression != 0 {
		args = append(args, "-C", fmt.Sprintf("%f", f.Compression))
	}

	// Comment (output only)
	if f.Comment != "" {
		args = append(args, "--comment", f.Comment)
	}

	// Add comment (output only)
	if f.AddComment != "" {
		args = append(args, "--add-comment", f.AddComment)
	}

	// Comment file (output only)
	if f.CommentFile != "" {
		args = append(args, "--comment-file", f.CommentFile)
	}

	// No glob
	if f.NoGlob {
		args = append(args, "--no-glob")
	}

	// Custom arguments - allows user to add any SoX parameter
	if len(f.CustomArgs) > 0 {
		args = append(args, f.CustomArgs...)
	}

	// Pipe
	if f.Pipe {
		args = append(args, "-")
	}

	return args
}

// Validate checks if the AudioFormat has valid parameters
// More flexible validation that allows users to configure their own parameters
func (f *AudioFormat) Validate() error {
	// Validate endian values if specified
	if f.Endian != "" {
		if f.Endian != "little" && f.Endian != "big" && f.Endian != "swap" {
			return fmt.Errorf("endian must be 'little', 'big', or 'swap'")
		}
	}

	return nil
}
