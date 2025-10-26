package sox

import (
	"sync"
	"time"
)

// ResourceMonitor tracks active SoX processes and resource usage
type ResourceMonitor struct {
	mu                sync.RWMutex
	activeProcesses   map[int]time.Time // PID -> start time
	totalConversions  int64
	failedConversions int64
}

var (
	monitorInstance *ResourceMonitor
	monitorOnce     sync.Once
)

// GetMonitor returns the global resource monitor instance
func GetMonitor() *ResourceMonitor {
	monitorOnce.Do(func() {
		monitorInstance = &ResourceMonitor{
			activeProcesses: make(map[int]time.Time),
		}
	})
	return monitorInstance
}

// TrackProcess registers a new SoX process
func (m *ResourceMonitor) TrackProcess(pid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeProcesses[pid] = time.Now()
	m.totalConversions++
}

// UntrackProcess removes a completed SoX process
func (m *ResourceMonitor) UntrackProcess(pid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.activeProcesses, pid)
}

// RecordFailure increments the failure counter
func (m *ResourceMonitor) RecordFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failedConversions++
}

// ActiveProcesses returns the number of currently active SoX processes
func (m *ResourceMonitor) ActiveProcesses() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.activeProcesses)
}

// TotalConversions returns the total number of conversions attempted
func (m *ResourceMonitor) TotalConversions() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.totalConversions
}

// FailedConversions returns the number of failed conversions
func (m *ResourceMonitor) FailedConversions() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.failedConversions
}

// SuccessRate returns the success rate as a percentage
func (m *ResourceMonitor) SuccessRate() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.totalConversions == 0 {
		return 100.0
	}

	successful := m.totalConversions - m.failedConversions
	return float64(successful) / float64(m.totalConversions) * 100.0
}

// OldestProcess returns the age of the oldest active process
func (m *ResourceMonitor) OldestProcess() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.activeProcesses) == 0 {
		return 0
	}

	oldest := time.Now()
	for _, startTime := range m.activeProcesses {
		if startTime.Before(oldest) {
			oldest = startTime
		}
	}

	return time.Since(oldest)
}

// Stats returns current monitoring statistics
type MonitorStats struct {
	ActiveProcesses   int
	TotalConversions  int64
	FailedConversions int64
	SuccessRate       float64
	OldestProcessAge  time.Duration
}

// GetStats returns current resource monitoring statistics
func (m *ResourceMonitor) GetStats() MonitorStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := MonitorStats{
		ActiveProcesses:   len(m.activeProcesses),
		TotalConversions:  m.totalConversions,
		FailedConversions: m.failedConversions,
		OldestProcessAge:  0,
	}

	if m.totalConversions > 0 {
		successful := m.totalConversions - m.failedConversions
		stats.SuccessRate = float64(successful) / float64(m.totalConversions) * 100.0
	} else {
		stats.SuccessRate = 100.0
	}

	if len(m.activeProcesses) > 0 {
		oldest := time.Now()
		for _, startTime := range m.activeProcesses {
			if startTime.Before(oldest) {
				oldest = startTime
			}
		}
		stats.OldestProcessAge = time.Since(oldest)
	}

	return stats
}

// Reset clears all monitoring statistics
func (m *ResourceMonitor) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.activeProcesses = make(map[int]time.Time)
	m.totalConversions = 0
	m.failedConversions = 0
}
