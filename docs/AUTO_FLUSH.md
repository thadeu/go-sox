# Auto-Flush Feature

## Overview

The `StreamConverter` now supports **automatic flushing** with a configurable interval. This allows you to set a timer that automatically flushes and saves the audio file after a specified duration.

## When to Use

- **RTP/SIP calls**: Automatically save recording after call duration
- **Time-limited recordings**: Set maximum recording duration
- **Memory management**: Automatically flush after certain time to free resources
- **Safety**: Ensure data is saved even if manual flush is forgotten

## API

### Basic Usage

```go
converter := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
    WithOutputPath("/app/recordings/call.flac").
    WithAutoFlush(30 * time.Second) // Auto-flush after 30 seconds

converter.Start()

// or with pool and auto-start

pool := sox.NewPoolWithLimit(10)

converter := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
    WithPool(pool).
    WithAutoFlush(3 * time.Second). // Auto-flush after 3 seconds
    WithOutputPath("/app/recordings/call.flac").
    WithAutoStart()

// Write RTP packets
for packet := range rtpChannel {
    converter.Write(packet.AudioData)
}

// Converter will automatically flush after 30 seconds!
// No need to call Flush() manually
```

### With Pool (Multiple Concurrent Calls)

```go
pool := sox.NewPoolWithLimit(10)

func handleRTPCall(callID string) {
    converter := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
        WithOutputPath(fmt.Sprintf("/app/recordings/%s.flac", callID)).
        WithPool(pool).
        WithAutoFlush(5 * time.Minute) // Auto-flush after 5 minutes
    
    converter.Start()
    
    // Write RTP packets
    for packet := range rtpChannel {
        converter.Write(packet.AudioData)
    }
    
    // File automatically saved after 5 minutes!
}
```

### Manual Flush (Full Control)

If you need full control, don't use `WithAutoFlush()`:

```go
converter := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
    WithOutputPath("/app/recordings/call.flac")

converter.Start()

// Write RTP packets
for packet := range rtpChannel {
    converter.Write(packet.AudioData)
}

// Manual flush when you want
converter.Flush()
```

### Combining Auto-Flush with Manual Control

You can use auto-flush as a safety mechanism and still manually flush:

```go
converter := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
    WithOutputPath("/app/recordings/call.flac").
    WithAutoFlush(10 * time.Minute) // Safety: max 10 minutes

converter.Start()

// Write RTP packets
for {
    select {
    case packet := <-rtpChannel:
        converter.Write(packet.AudioData)
        
    case <-callEnded:
        // Manual flush when call ends (before auto-flush)
        converter.Flush()
        return
    }
}

// If call never ends, auto-flush saves after 10 minutes
```

## How It Works

1. **Start()** - Starts SoX process and timer
2. **Write()** - Data is written to SoX stdin
3. **SoX converts** - Output is accumulated in internal buffer
4. **Timer expires** - Automatically calls `Flush()`
5. **Flush()** - Writes **entire accumulated buffer** to file
6. **Resources freed** - Pool slot released, process terminated

### Important: Buffer Accumulation

The converter **accumulates ALL data** from the start of the call:
- SoX output goes to stdout → internal buffer
- Buffer keeps growing with each Write()
- Flush() writes **complete buffer** to file
- **No data loss** - entire recording from start to finish

## Benefits

1. **Simple API** - Just set interval, forget about it
2. **Safety** - Ensures data is saved even if manual flush is forgotten
3. **Flexible** - Can still manually flush before timer expires
4. **Memory efficient** - Automatically frees resources after flush
5. **Pool friendly** - Works seamlessly with pool management

## Examples

### Example 1: RTP Recorder with Auto-Flush

```go
func recordRTPCall(callID string, duration time.Duration) {
    converter := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
        WithOutputPath(fmt.Sprintf("/app/recordings/%s.flac", callID)).
        WithAutoFlush(duration)
    
    converter.Start()
    
    // Write RTP packets
    for packet := range rtpChannel {
        converter.Write(packet.AudioData)
    }
    
    // File automatically saved after duration!
}

// Usage
recordRTPCall("call-123", 3*time.Minute) // Auto-flush after 3 minutes
```

### Example 2: Multiple Concurrent Calls

```go
pool := sox.NewPoolWithLimit(20)

func handleCall(callID string) {
    converter := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
        WithOutputPath(fmt.Sprintf("/app/recordings/%s.flac", callID)).
        WithPool(pool).
        WithAutoFlush(5 * time.Minute)
    
    if err := converter.Start(); err != nil {
        log.Printf("Failed to start: %v", err)
        return
    }
    
    // Process RTP packets...
}

// Start multiple calls
for i := 0; i < 50; i++ {
    go handleCall(fmt.Sprintf("call-%d", i))
}
```

### Example 3: With Timeout Context

```go
func recordWithTimeout(callID string) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    converter := sox.NewStreamConverter(sox.PCM_RAW_16K_MONO, sox.FLAC_16K_MONO).
        WithOutputPath(fmt.Sprintf("/app/recordings/%s.flac", callID)).
        WithAutoFlush(5 * time.Minute)
    
    if err := converter.Start(ctx); err != nil {
        log.Printf("Failed to start: %v", err)
        return
    }
    
    // Write packets...
}
```

## Configuration Recommendations

### Short Recordings (< 1 minute)
```go
WithAutoFlush(30 * time.Second)
```

### Medium Recordings (1-5 minutes)
```go
WithAutoFlush(3 * time.Minute)
```

### Long Recordings (> 5 minutes)
```go
WithAutoFlush(10 * time.Minute)
```

### Safety Timeout (with manual flush)
```go
WithAutoFlush(1 * time.Hour) // Safety: max 1 hour
```

## Important Notes

1. **Auto-flush terminates the process** - After auto-flush, the converter is closed
2. **Manual flush stops timer** - If you manually flush before timer expires, timer is stopped
3. **Close stops timer** - Calling `Close()` stops the auto-flush timer
4. **One-time flush** - Auto-flush only happens once, then converter is closed
5. **Complete recording saved** - Flush writes entire accumulated buffer (from start to finish)

## Buffer Behavior

### How Buffer Accumulation Works

```
Time 0s: Start() → SoX process starts, buffer empty
Time 1s: Write(packet1) → SoX converts → buffer: [packet1_flac]
Time 2s: Write(packet2) → SoX converts → buffer: [packet1_flac, packet2_flac]
Time 3s: Write(packet3) → SoX converts → buffer: [packet1_flac, packet2_flac, packet3_flac]
Time 3s: AutoFlush() → Write buffer to file → file contains ALL 3 packets!
```

### Example: 10 Second Call with 3 Second Auto-Flush

```go
converter := sox.NewStreamConverter(input, output).
    WithOutputPath("/app/recordings/call.flac").
    WithAutoFlush(3 * time.Second)

converter.Start()

// Time 0-3s: Write packets
for i := 0; i < 150; i++ { // 150 packets = 3 seconds
    converter.Write(rtpPacket)
}

// Time 3s: Auto-flush triggers
// File saved with ALL 3 seconds of audio!
// Process terminated, converter closed
```

**Result**: File contains complete 3-second recording, not just the last packet!

## Comparison: Before vs After

### Before (Manual Management)

```go
converter := sox.NewStreamConverter(input, output).
    WithOutputPath(outputFile)

converter.Start()

// Need to track time manually
timeout := time.After(5 * time.Minute)

for {
    select {
    case packet := <-rtpChannel:
        converter.Write(packet)
        
    case <-timeout:
        // Manual timeout handling
        converter.Flush()
        return
    }
}
```

### After (Auto-Flush)

```go
converter := sox.NewStreamConverter(input, output).
    WithOutputPath(outputFile).
    WithAutoFlush(5 * time.Minute) // That's it!

converter.Start()

// Just write packets
for packet := range rtpChannel {
    converter.Write(packet)
}

// Automatically flushed after 5 minutes!
```

## Complete Example

See `examples/rtp_recorder/main.go` for complete working examples.

## FAQ

**Q: What happens if I manually flush before auto-flush timer?**  
A: The timer is automatically stopped, and manual flush takes precedence.

**Q: Can I change the interval after Start()?**  
A: No, the interval is set before Start() and cannot be changed.

**Q: What if I don't want auto-flush?**  
A: Simply don't call `WithAutoFlush()`. The converter works normally without it.

**Q: Does auto-flush work with stdout mode?**  
A: Auto-flush is designed for file output. For stdout mode, use manual flush.

**Q: Can I use auto-flush with pool?**  
A: Yes! Auto-flush works seamlessly with pool management.

