package sox

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/thadeu/go-sox"
)

// generatePCMData generates test PCM audio data
func generatePCMData(sampleRate, durationMs int) []byte {
	numSamples := (sampleRate * durationMs) / 1000
	buffer := make([]byte, numSamples*2) // mono, 16-bit
	for i := 0; i < numSamples; i++ {
		value := int16((i % 1000) * 32)
		buffer[i*2] = byte(value & 0xFF)
		buffer[i*2+1] = byte((value >> 8) & 0xFF)
	}
	return buffer
}

// TestStreamConverter_Basic tests basic streaming conversion
func TestStreamConverter_Basic(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skip("SoX not installed")
	}

	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
	if err := stream.Start(); err != nil {
		t.Fatalf("Failed to start stream: %v", err)
	}

	// Write multiple chunks
	for i := 0; i < 10; i++ {
		chunk := generatePCMData(16000, 100) // 100ms chunks
		if _, err := stream.Write(chunk); err != nil {
			t.Fatalf("Failed to write chunk %d: %v", i, err)
		}
	}

	// Flush and get output
	data, err := stream.Flush()
	if err != nil {
		t.Fatalf("Failed to flush: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("Expected non-empty output")
	}

	t.Logf("Converted %d chunks to %d bytes FLAC", 10, len(data))
}

// TestStreamConverter_WithOutputPath tests streaming with file output
func TestStreamConverter_WithOutputPath(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skip("SoX not installed")
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test_stream.flac")

	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO).
		WithOutputPath(outputPath)

	if err := stream.Start(); err != nil {
		t.Fatalf("Failed to start stream: %v", err)
	}

	// Write data
	for i := 0; i < 20; i++ {
		chunk := generatePCMData(16000, 50) // 50ms chunks
		if _, err := stream.Write(chunk); err != nil {
			t.Fatalf("Failed to write chunk %d: %v", i, err)
		}
	}

	// Flush to file
	_, err := stream.Flush()
	if err != nil {
		t.Fatalf("Failed to flush: %v", err)
	}

	// Verify file exists and has content
	fileInfo, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Output file not created: %v", err)
	}

	if fileInfo.Size() == 0 {
		t.Fatal("Output file is empty")
	}

	t.Logf("Created file %s with size %d bytes", outputPath, fileInfo.Size())
}

// TestStreamConverter_AutoFlush tests auto-flush functionality
func TestStreamConverter_AutoFlush(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skip("SoX not installed")
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test_autoflush.flac")

	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO).
		WithOutputPath(outputPath).
		WithAutoFlush(1 * time.Second) // Auto-flush after 1 second

	if err := stream.Start(); err != nil {
		t.Fatalf("Failed to start stream: %v", err)
	}

	// Write data for 500ms
	for i := 0; i < 10; i++ {
		chunk := generatePCMData(16000, 50) // 50ms chunks
		if _, err := stream.Write(chunk); err != nil {
			t.Fatalf("Failed to write chunk %d: %v", i, err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for auto-flush (1 second + buffer)
	time.Sleep(1500 * time.Millisecond)

	// Verify file exists
	fileInfo, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Auto-flush did not create file: %v", err)
	}

	if fileInfo.Size() == 0 {
		t.Fatal("Auto-flushed file is empty")
	}

	t.Logf("Auto-flush created file with size %d bytes", fileInfo.Size())
}

// TestStreamConverter_BufferAccumulation tests that buffer accumulates all data
func TestStreamConverter_BufferAccumulation(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skip("SoX not installed")
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test_accumulation.flac")

	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO).
		WithOutputPath(outputPath)

	if err := stream.Start(); err != nil {
		t.Fatalf("Failed to start stream: %v", err)
	}

	// Write data in multiple stages
	totalChunks := 30
	for i := 0; i < totalChunks; i++ {
		chunk := generatePCMData(16000, 100) // 100ms chunks
		if _, err := stream.Write(chunk); err != nil {
			t.Fatalf("Failed to write chunk %d: %v", i, err)
		}
	}

	// Flush all accumulated data
	_, err := stream.Flush()
	if err != nil {
		t.Fatalf("Failed to flush: %v", err)
	}

	// Verify file contains all data (should be ~3 seconds of audio)
	fileInfo, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Output file not created: %v", err)
	}

	// FLAC should compress 3 seconds of 16K mono PCM (96KB) to much less
	expectedMinSize := int64(1000) // At least 1KB
	if fileInfo.Size() < expectedMinSize {
		t.Fatalf("Output file too small: got %d bytes, expected at least %d", fileInfo.Size(), expectedMinSize)
	}

	t.Logf("Accumulated %d chunks (3s) into %d bytes FLAC", totalChunks, fileInfo.Size())
}

// TestStreamConverter_MultipleFormats tests different output formats
func TestStreamConverter_MultipleFormats(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skip("SoX not installed")
	}

	tmpDir := t.TempDir()

	formats := []struct {
		name   string
		format AudioFormat
		ext    string
	}{
		{"FLAC", FLAC_16K_MONO, ".flac"},
		{"WAV", WAV_16K_MONO, ".wav"},
		{"ULAW", ULAW_8K_MONO, ".ul"},
	}

	for _, tc := range formats {
		t.Run(tc.name, func(t *testing.T) {
			outputPath := filepath.Join(tmpDir, "test_"+tc.name+tc.ext)

			stream := NewStreamConverter(PCM_RAW_16K_MONO, tc.format).
				WithOutputPath(outputPath)

			if err := stream.Start(); err != nil {
				t.Fatalf("Failed to start stream for %s: %v", tc.name, err)
			}

			// Write data
			for i := 0; i < 10; i++ {
				chunk := generatePCMData(16000, 100)
				if _, err := stream.Write(chunk); err != nil {
					t.Fatalf("Failed to write chunk: %v", err)
				}
			}

			// Flush
			_, err := stream.Flush()
			if err != nil {
				t.Fatalf("Failed to flush %s: %v", tc.name, err)
			}

			// Verify file
			fileInfo, err := os.Stat(outputPath)
			if err != nil {
				t.Fatalf("File not created for %s: %v", tc.name, err)
			}

			if fileInfo.Size() == 0 {
				t.Fatalf("File is empty for %s", tc.name)
			}

			t.Logf("%s: %d bytes", tc.name, fileInfo.Size())
		})
	}
}

// TestStreamConverter_WithPool tests streaming with pool
func TestStreamConverter_WithPool(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skip("SoX not installed")
	}

	tmpDir := t.TempDir()
	pool := NewPoolWithLimit(2) // Limit to 2 concurrent

	// Start 3 streams (3rd should wait for slot)
	streams := make([]*StreamConverter, 3)
	for i := 0; i < 3; i++ {
		outputPath := filepath.Join(tmpDir, "test_pool_"+string(rune('A'+i))+".flac")
		streams[i] = NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO).
			WithOutputPath(outputPath).
			WithPool(pool)
	}

	// Start first 2 (should succeed immediately)
	for i := 0; i < 2; i++ {
		if err := streams[i].Start(); err != nil {
			t.Fatalf("Failed to start stream %d: %v", i, err)
		}
	}

	// Try to start 3rd (should block or fail)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := streams[2].Start(ctx)
	if err == nil {
		t.Fatal("Expected 3rd stream to fail/timeout due to pool limit")
	}

	// Flush first stream to free slot
	for i := 0; i < 5; i++ {
		chunk := generatePCMData(16000, 100)
		streams[0].Write(chunk)
	}
	streams[0].Flush()

	// Now 3rd stream should be able to start
	if err := streams[2].Start(); err != nil {
		t.Fatalf("Failed to start 3rd stream after slot freed: %v", err)
	}

	// Cleanup
	for i := 1; i < 3; i++ {
		chunk := generatePCMData(16000, 100)
		streams[i].Write(chunk)
		streams[i].Flush()
	}

	t.Log("Pool management working correctly")
}

// TestStreamConverter_Available tests Available() method
func TestStreamConverter_Available(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skip("SoX not installed")
	}

	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
	if err := stream.Start(); err != nil {
		t.Fatalf("Failed to start stream: %v", err)
	}

	// Initially should be 0
	if available := stream.Available(); available != 0 {
		t.Fatalf("Expected 0 available initially, got %d", available)
	}

	// Write some data
	chunk := generatePCMData(16000, 500) // 500ms
	stream.Write(chunk)

	// Give SoX time to process
	time.Sleep(200 * time.Millisecond)

	// Should have some data available
	available := stream.Available()
	if available == 0 {
		t.Log("Warning: No data available yet (might be buffering)")
	} else {
		t.Logf("Available: %d bytes", available)
	}

	// Flush
	data, err := stream.Flush()
	if err != nil {
		t.Fatalf("Failed to flush: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("Expected non-empty output")
	}

	t.Logf("Final output: %d bytes", len(data))
}

// TestStreamConverter_Close tests Close() method
func TestStreamConverter_Close(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skip("SoX not installed")
	}

	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
	if err := stream.Start(); err != nil {
		t.Fatalf("Failed to start stream: %v", err)
	}

	// Write some data
	chunk := generatePCMData(16000, 100)
	stream.Write(chunk)

	// Close without flush
	if err := stream.Close(); err != nil {
		t.Fatalf("Failed to close: %v", err)
	}

	// Try to write after close (should fail)
	_, err := stream.Write(chunk)
	if err == nil {
		t.Fatal("Expected write after close to fail")
	}

	t.Log("Close() working correctly")
}

// TestStreamConverter_StdoutMode tests streaming without output path
func TestStreamConverter_StdoutMode(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skip("SoX not installed")
	}

	stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
	// No WithOutputPath() - should use stdout mode

	if err := stream.Start(); err != nil {
		t.Fatalf("Failed to start stream: %v", err)
	}

	// Write data
	for i := 0; i < 10; i++ {
		chunk := generatePCMData(16000, 100)
		if _, err := stream.Write(chunk); err != nil {
			t.Fatalf("Failed to write: %v", err)
		}
	}

	// Flush and get data
	data, err := stream.Flush()
	if err != nil {
		t.Fatalf("Failed to flush: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("Expected non-empty output in stdout mode")
	}

	// Verify it's valid FLAC data (starts with "fLaC")
	if !bytes.HasPrefix(data, []byte("fLaC")) {
		t.Fatal("Output doesn't appear to be valid FLAC")
	}

	t.Logf("Stdout mode: %d bytes FLAC", len(data))
}

// TestStreamConverter_ErrorHandling tests error scenarios
func TestStreamConverter_ErrorHandling(t *testing.T) {
	if err := CheckSoxInstalled(""); err != nil {
		t.Skip("SoX not installed")
	}

	t.Run("WriteBeforeStart", func(t *testing.T) {
		stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
		chunk := generatePCMData(16000, 100)
		_, err := stream.Write(chunk)
		if err == nil {
			t.Fatal("Expected error when writing before start")
		}
	})

	t.Run("FlushBeforeStart", func(t *testing.T) {
		stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
		_, err := stream.Flush()
		if err == nil {
			t.Fatal("Expected error when flushing before start")
		}
	})

	t.Run("DoubleStart", func(t *testing.T) {
		stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO)
		stream.Start()
		err := stream.Start()
		if err == nil {
			t.Fatal("Expected error on double start")
		}
		stream.Close()
	})

	t.Run("InvalidOutputPath", func(t *testing.T) {
		stream := NewStreamConverter(PCM_RAW_16K_MONO, FLAC_16K_MONO).
			WithOutputPath("/invalid/path/that/does/not/exist/file.flac")

		if err := stream.Start(); err != nil {
			t.Fatalf("Start should succeed: %v", err)
		}

		chunk := generatePCMData(16000, 100)
		stream.Write(chunk)

		// Flush should fail when trying to write to invalid path
		_, err := stream.Flush()
		if err == nil {
			t.Fatal("Expected error when flushing to invalid path")
		}
	})
}
