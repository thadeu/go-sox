package sox

import "fmt"

// AudioFormat defines the audio format parameters for input or output
type AudioFormat struct {
	Type       string // "raw", "flac", "wav", "mp3", "ogg", etc.
	Encoding   string // "signed-integer", "unsigned-integer", "floating-point", "mu-law", "a-law", "ima-adpcm", "ms-adpcm", "gsm-full-rate", "little-endian", "big-endian"
	SampleRate int    // Sample rate in Hz (e.g., 8000, 16000, 44100, 48000)
	Channels   int    // Number of channels: 1 = mono, 2 = stereo
	BitDepth   int    // Bits per sample: 8, 16, 24, 32
}

// Common audio format presets for convenience

var (
	// PCM_RAW_8K_MONO - PCM Raw 8kHz mono 16-bit (common for telephony)
	PCM_RAW_8K_MONO = AudioFormat{
		Type:       "raw",
		Encoding:   "signed-integer",
		SampleRate: 8000,
		Channels:   1,
		BitDepth:   16,
	}

	// PCM_RAW_16K_MONO - PCM Raw 16kHz mono 16-bit (common for speech recognition)
	PCM_RAW_16K_MONO = AudioFormat{
		Type:       "raw",
		Encoding:   "signed-integer",
		SampleRate: 16000,
		Channels:   1,
		BitDepth:   16,
	}

	// PCM_RAW_48K_MONO - PCM Raw 48kHz mono 16-bit (high quality)
	PCM_RAW_48K_MONO = AudioFormat{
		Type:       "raw",
		Encoding:   "signed-integer",
		SampleRate: 48000,
		Channels:   1,
		BitDepth:   16,
	}

	// FLAC_16K_MONO - FLAC 16kHz mono (optimized for speech transcription)
	FLAC_16K_MONO = AudioFormat{
		Type:       "flac",
		Encoding:   "signed-integer",
		SampleRate: 16000,
		Channels:   1,
		BitDepth:   16,
	}

	// FLAC_44K_STEREO - FLAC 44.1kHz stereo (CD quality)
	FLAC_44K_STEREO = AudioFormat{
		Type:       "flac",
		Encoding:   "signed-integer",
		SampleRate: 44100,
		Channels:   2,
		BitDepth:   16,
	}

	// WAV_8K_MONO - WAV 8kHz mono 8-bit
	WAV_8K_MONO = AudioFormat{
		Type:       "wav",
		Encoding:   "little-endian",
		SampleRate: 8000,
		Channels:   1,
		BitDepth:   8,
	}

	WAV_16K_MONO = AudioFormat{
		Type:       "wav",
		Encoding:   "little-endian",
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

// buildArgs converts AudioFormat to SoX command-line arguments
func (f *AudioFormat) buildArgs(isInput bool) []string {
	var args []string

	// Type argument
	args = append(args, "-t", f.Type)

	// For raw formats, we need to specify encoding details
	if f.Type == "raw" {
		if f.Encoding != "" {
			args = append(args, "-e", f.Encoding)
		}
		if f.SampleRate > 0 {
			args = append(args, "-r", fmt.Sprintf("%d", f.SampleRate))
		}
		if f.Channels > 0 {
			args = append(args, "-c", fmt.Sprintf("%d", f.Channels))
		}
		if f.BitDepth > 0 {
			args = append(args, "-b", fmt.Sprintf("%d", f.BitDepth))
		}
	} else {
		// For other formats, add rate and channels if specified
		if f.SampleRate > 0 && !isInput {
			args = append(args, "-r", fmt.Sprintf("%d", f.SampleRate))
		}
		if f.Channels > 0 && !isInput {
			args = append(args, "-c", fmt.Sprintf("%d", f.Channels))
		}
	}

	return args
}

// Validate checks if the AudioFormat has valid parameters
func (f *AudioFormat) Validate() error {
	if f.Type == "" {
		return fmt.Errorf("audio format type is required")
	}

	if f.Type == "raw" {
		if f.Encoding == "" {
			return fmt.Errorf("encoding is required for raw format")
		}
		if f.SampleRate <= 0 {
			return fmt.Errorf("sample rate must be positive")
		}
		if f.Channels <= 0 {
			return fmt.Errorf("channels must be positive")
		}
		if f.BitDepth <= 0 {
			return fmt.Errorf("bit depth must be positive")
		}
	}

	return nil
}
