// Package sox provides a high-performance Go wrapper for SoX (Sound eXchange),
// the Swiss Army knife of audio manipulation.
//
// This library enables fast audio format conversion using pipes and streams,
// avoiding file I/O overhead. It's particularly optimized for real-time audio
// processing scenarios like converting RTP media streams for transcription.
//
// # Performance
//
// Benchmarked on Apple M2 with 1 second audio (16kHz mono PCM to FLAC):
//   - SoX Converter: 4.87ms per conversion (4,870,383 ns/op)
//   - SoX Stream: 4.88ms per conversion (4,886,878 ns/op)
//   - FFmpeg (file I/O): 42ms per conversion (41,985,972 ns/op)
//
// Using pipes instead of file I/O provides 8.6x better performance compared to ffmpeg.
// Memory overhead is minimal: ~92-98KB allocated per conversion.
//
// # Basic Usage
//
// One-shot conversion:
//
//	converter := sox.NewConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO)
//	err := converter.Convert(inputReader, outputWriter)
//
// Streaming conversion (for RTP/real-time audio):
//
//	stream := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO)
//	stream.Start()
//
//	// Write audio data as it arrives
//	stream.Write(pcmChunk1)
//	stream.Write(pcmChunk2)
//
//	// Get converted output
//	flacData, err := stream.Flush()
//
// # Audio Formats
//
// The package includes common format presets:
//   - PCM_RAW_8K_MONO, PCM_RAW_16K_MONO, PCM_RAW_48K_MONO
//   - FLAC_16K_MONO, FLAC_44K_STEREO
//   - WAV_16K_MONO
//   - ULAW_8K_MONO
//
// Or define custom formats:
//
//	customFormat := sox.AudioFormat{
//	    Type:       "raw",
//	    Encoding:   "signed-integer",
//	    SampleRate: 8000,
//	    Channels:   1,
//	    BitDepth:   16,
//	}
//
// # Requirements
//
// SoX must be installed and accessible in PATH:
//   - macOS: brew install sox
//   - Ubuntu/Debian: apt-get install sox
//   - RHEL/CentOS: yum install sox
//
// Verify installation:
//
//	err := sox.CheckSoxInstalled("")
//
// # Use Cases
//
// This library is ideal for:
//   - Converting RTP/SIP media streams to formats suitable for transcription APIs
//   - Real-time audio format conversion without temporary files
//   - High-throughput audio processing pipelines
//   - Batch audio conversions with minimal latency
package sox
