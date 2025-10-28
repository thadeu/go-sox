package sox

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"
)

type Streamer struct {
	Input   AudioFormat
	Output  AudioFormat
	Options ConversionOptions

	pool       *Pool
	outputPath string
	started    bool
	closed     bool
	cmd        *exec.Cmd

	ticker     *time.Ticker
	tickerStop chan bool

	buffer     *bytes.Buffer
	bufferLock sync.Mutex
}

func NewStreamer(input, output AudioFormat) *Streamer {
	return &Streamer{
		Input:  input,
		Output: output,
	}
}

func (s *Streamer) WithAutoStart(interval time.Duration) *Streamer {
	s.Start(interval)
	return s
}

func (s *Streamer) WithOptions(options ConversionOptions) *Streamer {
	s.Options = options
	return s
}

func (s *Streamer) WithOutputPath(path string) *Streamer {
	s.outputPath = path
	return s
}

// Write writes raw audio data to the SoX process
// The data will be converted according to the configured formats
func (s *Streamer) Write(data []byte) (int, error) {
	if !s.started {
		return 0, fmt.Errorf("stream converter not started")
	}

	if s.closed {
		return 0, fmt.Errorf("stream converter closed")
	}

	s.bufferLock.Lock()
	defer s.bufferLock.Unlock()

	return s.buffer.Write(data)
}

// Read reads converted audio data from the buffer
func (s *Streamer) Read(b []byte) (int, error) {
	s.bufferLock.Lock()
	defer s.bufferLock.Unlock()

	return s.buffer.Read(b)
}

// Simple Logic to create buffer stream to auto-save
// cmd := exec.Command(
//
//	"sox",
//	"-t", "raw",
//	"-r", strconv.Itoa(session.OriginalSampleRate),
//	"-b", strconv.Itoa(session.OriginalSampleRate/1000),
//	"-c", "1",
//	"--encoding", GetCodecType(session.OriginalCodec),
//	"--ignore-length",
//	"-",
//	"-t", "flac",
//	"-r", "16000",
//	"-b", "16",
//	"-c", "1",
//	"-C", "0",
//	"--add-comment", "PAPI rtp-recorder",
//	finalPath,
//
// )
//
// cmd.Stdin = bytes.NewReader(session.Buffer)
// cmd.Stderr = os.Stderr
//
//	if err := cmd.Run(); err != nil {
//		log.Println("Erro na conversão dos pacotes:", err)
//		continue
//	}
func (s *Streamer) Start(interval time.Duration) {
	s.ticker = time.NewTicker(interval)

	for {
		select {
		case <-s.ticker.C:
			args := s.buildCommandArgs()
			s.cmd = exec.Command(s.Options.SoxPath, args...)

			s.cmd.Stdin = s.buffer
			s.cmd.Stderr = os.Stderr

			if err := s.cmd.Run(); err != nil {
				log.Println("Error converting packets:", err)
				continue
			}
		case <-s.tickerStop:
			s.ticker.Stop()
			return
		}
	}
}

func (s *Streamer) Stop() error {
	if !s.started {
		return nil
	}

	if s.closed {
		return nil
	}

	s.tickerStop <- true
	s.closed = true
	s.started = false

	if s.cmd != nil && s.cmd.Process != nil {
		// Try to wait for graceful exit first
		done := make(chan error, 1)

		go func() {
			done <- s.cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited gracefully
		case <-time.After(5 * time.Second):
			// Timeout - force kill
			s.cmd.Process.Kill()

			<-done // Wait for Wait() to return after Kill
		}
	}

	return nil
}

func (s *Streamer) End() error {
	return s.Stop()
}

// buildCommandArgs constructs the SoX command arguments for streaming
func (s *Streamer) buildCommandArgs() []string {
	args := []string{}

	// Global options
	args = append(args, s.Options.BuildGlobalArgs()...)

	// Input format arguments
	if !s.Input.Pipe {
		s.Input.Pipe = true // Always use stdin
	}

	args = append(args, s.Input.BuildArgs()...)

	// Input file (stdin)
	args = append(args, "-")

	// Output format arguments
	args = append(args, s.Output.BuildArgs()...)

	// Effects
	if effects := s.Options.buildEffectArgs(); len(effects) > 0 {
		args = append(args, effects...)
	}

	return args
}
