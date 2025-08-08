package monitor

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// SortBy represents different sorting options
type SortBy int

const (
	SortByPID SortBy = iota
	SortByName
	SortByCPU
	SortByMemory
)

// ProcessIOCounters holds I/O counters for a process
type ProcessIOCounters struct {
	ReadBytes  uint64
	WriteBytes uint64
	Timestamp  time.Time
}

// Monitor handles system monitoring
type Monitor struct {
	mu               sync.RWMutex
	processes        []ProcessInfo
	systemMetrics    SystemMetrics
	processMetrics   map[int32]*ProcessMetrics
	sortBy           SortBy
	sortDesc         bool
	metricsCapacity  int
	lastNetStats     map[string]net.IOCountersStat
	lastProcessStats map[int32]*process.Process
	lastProcessIO    map[int32]*ProcessIOCounters
	systemIOTotal    float64 // Total system I/O rate for percentage calculation
}

// NewMonitor creates a new monitor instance
func NewMonitor() *Monitor {
	return &Monitor{
		processes:        make([]ProcessInfo, 0),
		processMetrics:   make(map[int32]*ProcessMetrics),
		sortBy:           SortByCPU,
		sortDesc:         true,
		metricsCapacity:  60, // Keep 60 seconds of data
		lastNetStats:     make(map[string]net.IOCountersStat),
		lastProcessStats: make(map[int32]*process.Process),
		lastProcessIO:    make(map[int32]*ProcessIOCounters),
		systemIOTotal:    0,
	}
}

// GetProcesses returns a copy of the current process list
func (m *Monitor) GetProcesses() []ProcessInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	processes := make([]ProcessInfo, len(m.processes))
	copy(processes, m.processes)
	return processes
}

// GetSystemMetrics returns the current system metrics
func (m *Monitor) GetSystemMetrics() SystemMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.systemMetrics
}

// GetProcessMetrics returns metrics for a specific process
func (m *Monitor) GetProcessMetrics(pid int32) *ProcessMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if metrics, exists := m.processMetrics[pid]; exists {
		// Return a deep copy to avoid race conditions with shared slice headers
		metricsCopy := &ProcessMetrics{
			Timestamps:    append([]time.Time(nil), metrics.Timestamps...),
			CPUPercent:    append([]float64(nil), metrics.CPUPercent...),
			MemoryMB:      append([]float64(nil), metrics.MemoryMB...),
			DiskReadRate:  append([]float64(nil), metrics.DiskReadRate...),
			DiskWriteRate: append([]float64(nil), metrics.DiskWriteRate...),
			DiskReadPerc:  append([]float64(nil), metrics.DiskReadPerc...),
			DiskWritePerc: append([]float64(nil), metrics.DiskWritePerc...),
			NetSentRate:   append([]float64(nil), metrics.NetSentRate...),
			NetRecvRate:   append([]float64(nil), metrics.NetRecvRate...),
		}
		return metricsCopy
	}
	return nil
}

// EnsureProcessMetrics ensures that a process has time series metrics tracking
// This is called when a user selects a process that might not be in the top 150
func (m *Monitor) EnsureProcessMetrics(pid int32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// If we don't have metrics for this process, create them
	if _, exists := m.processMetrics[pid]; !exists {
		m.processMetrics[pid] = NewProcessMetrics(m.metricsCapacity)
	}
}

// GetCurrentProcessData gets current data for a specific process and updates its time series
func (m *Monitor) GetCurrentProcessData(pid int32) (*ProcessInfo, error) {
	// Ensure we have metrics tracking for this process
	m.EnsureProcessMetrics(pid)

	// Try to get the process
	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil, err
	}

	// Get detailed process info
	processInfo, err := m.getDetailedProcessInfo(proc)
	if err != nil {
		return nil, err
	}

	// Update time series metrics for this process
	m.updateProcessTimeSeriesMetrics(processInfo)

	return &processInfo, nil
}

// SetSorting sets the sorting criteria
func (m *Monitor) SetSorting(sortBy SortBy, desc bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sortBy = sortBy
	m.sortDesc = desc
}

// UpdateMetrics updates all system and process metrics
func (m *Monitor) UpdateMetrics(ctx context.Context) error {
	// Update system metrics
	if err := m.updateSystemMetrics(); err != nil {
		return fmt.Errorf("failed to update system metrics: %w", err)
	}

	// Update process metrics
	if err := m.updateProcessMetrics(ctx); err != nil {
		return fmt.Errorf("failed to update process metrics: %w", err)
	}

	return nil
}

func (m *Monitor) updateSystemMetrics() error {
	// Get CPU usage
	cpuPercent, err := cpu.Percent(0, false)
	if err != nil {
		return err
	}
	var cpuValue float64
	if len(cpuPercent) > 0 {
		cpuValue = cpuPercent[0]
	}

	// Get memory usage
	memStat, err := mem.VirtualMemory()
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.systemMetrics = SystemMetrics{
		CPUPercent:    cpuValue,
		MemoryPercent: memStat.UsedPercent,
		TotalMemoryMB: float64(memStat.Total) / 1024 / 1024,
		UsedMemoryMB:  float64(memStat.Used) / 1024 / 1024,
		Timestamp:     time.Now(),
	}
	m.mu.Unlock()

	return nil
}

func (m *Monitor) updateProcessMetrics(ctx context.Context) error {
	pids, err := process.Pids()
	if err != nil {
		return err
	}

	// Performance optimization: Process in batches and prioritize interesting processes
	allProcesses := make([]ProcessInfo, 0, len(pids))
	newProcessStats := make(map[int32]*process.Process)

	// Calculate system I/O total for percentage calculations
	systemIOTotal := m.calculateSystemIOTotal()
	m.mu.Lock()
	m.systemIOTotal = systemIOTotal
	m.mu.Unlock()

	// First pass: Quick scan to get basic info for all processes
	for i, pid := range pids {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Process in batches to avoid blocking too long
		if i > 0 && i%50 == 0 {
			// Yield to other goroutines occasionally
			time.Sleep(1 * time.Millisecond)
		}

		// Reuse existing process object if available
		var proc *process.Process
		if existingProc, exists := m.lastProcessStats[pid]; exists {
			proc = existingProc
		} else {
			var err error
			proc, err = process.NewProcess(pid)
			if err != nil {
				continue // Process might have disappeared
			}
		}

		newProcessStats[pid] = proc

		// Get basic process info (lightweight operations only)
		processInfo, err := m.getBasicProcessInfo(proc)
		if err != nil {
			continue // Skip processes we can't read
		}

		allProcesses = append(allProcesses, processInfo)
	}

	// Sort by resource usage and keep top processes
	m.sortProcessesByResourceUsage(allProcesses)
	maxProcesses := 150 // Keep top 150 processes for detailed monitoring
	if len(allProcesses) > maxProcesses {
		allProcesses = allProcesses[:maxProcesses]
	}

	// Second pass: Detailed monitoring for top processes only
	processes := make([]ProcessInfo, 0, len(allProcesses))
	for _, basicInfo := range allProcesses {
		if proc, exists := newProcessStats[basicInfo.PID]; exists {
			// Get detailed info including I/O metrics
			processInfo, err := m.getDetailedProcessInfo(proc)
			if err != nil {
				continue
			}
			processes = append(processes, processInfo)

			// Update time-series metrics for this process
			m.updateProcessTimeSeriesMetrics(processInfo)
		}
	}

	m.mu.Lock()
	m.processes = processes
	m.lastProcessStats = newProcessStats
	m.sortProcesses()
	m.mu.Unlock()

	return nil
}

// calculateSystemIOTotal calculates total system I/O for percentage calculations
func (m *Monitor) calculateSystemIOTotal() float64 {
	// Get system-wide disk I/O stats
	// This is a simplified approach - in reality you'd want to get disk stats
	// For now, we'll use a reasonable baseline of 100 MB/s as "100%" I/O
	return 100 * 1024 // 100 MB/s in KB/s
}

// getBasicProcessInfo gets lightweight process info for initial sorting
func (m *Monitor) getBasicProcessInfo(proc *process.Process) (ProcessInfo, error) {
	info := ProcessInfo{PID: proc.Pid}

	// Get only the essential info for sorting
	if name, err := proc.Name(); err == nil {
		info.Name = name
	}

	if cpuPercent, err := proc.CPUPercent(); err == nil {
		info.CPUPercent = cpuPercent
	}

	if memPercent, err := proc.MemoryPercent(); err == nil {
		info.MemoryPerc = memPercent
	}

	if memInfo, err := proc.MemoryInfo(); err == nil {
		info.MemoryMB = float64(memInfo.RSS) / 1024 / 1024
	}

	return info, nil
}

// getDetailedProcessInfo gets comprehensive process info including I/O
func (m *Monitor) getDetailedProcessInfo(proc *process.Process) (ProcessInfo, error) {
	// Start with basic info
	info, err := m.getBasicProcessInfo(proc)
	if err != nil {
		return info, err
	}

	now := time.Now()

	// Read a consistent snapshot of system I/O total under lock
	m.mu.RLock()
	systemIOTotal := m.systemIOTotal
	m.mu.RUnlock()

	// Get process creation time
	if createTime, err := proc.CreateTime(); err == nil {
		info.CreateTime = time.Unix(createTime/1000, 0)
	}

	// Get I/O counters and calculate rates
	if ioCounters, err := proc.IOCounters(); err == nil {
		info.DiskReadKB = float64(ioCounters.ReadBytes) / 1024
		info.DiskWriteKB = float64(ioCounters.WriteBytes) / 1024

		// Calculate rates if we have previous data
		m.mu.RLock()
		lastIO, exists := m.lastProcessIO[proc.Pid]
		m.mu.RUnlock()
		if exists {
			timeDiff := now.Sub(lastIO.Timestamp).Seconds()
			if timeDiff > 0 {
				readDiff := float64(ioCounters.ReadBytes - lastIO.ReadBytes)
				writeDiff := float64(ioCounters.WriteBytes - lastIO.WriteBytes)

				info.DiskReadRate = (readDiff / 1024) / timeDiff   // KB/s
				info.DiskWriteRate = (writeDiff / 1024) / timeDiff // KB/s

				// Calculate percentage of system I/O
				if systemIOTotal > 0 {
					info.DiskReadPerc = (info.DiskReadRate / systemIOTotal) * 100
					info.DiskWritePerc = (info.DiskWriteRate / systemIOTotal) * 100
				} else {
					// Fallback: use rate relative to a reasonable baseline
					info.DiskReadPerc = math.Min(info.DiskReadRate/1024, 100)
					info.DiskWritePerc = math.Min(info.DiskWriteRate/1024, 100)
				}
			}
		}

		// Store current I/O counters for next calculation
		m.mu.Lock()
		m.lastProcessIO[proc.Pid] = &ProcessIOCounters{
			ReadBytes:  ioCounters.ReadBytes,
			WriteBytes: ioCounters.WriteBytes,
			Timestamp:  now,
		}
		m.mu.Unlock()
	}

	// Network I/O - simplified implementation (removed expensive connection enumeration)
	info.NetSentKB = 0
	info.NetRecvKB = 0
	info.NetSentRate = 0
	info.NetRecvRate = 0

	return info, nil
}

// sortProcessesByResourceUsage sorts processes by CPU and memory usage for prioritization
func (m *Monitor) sortProcessesByResourceUsage(processes []ProcessInfo) {
	sort.Slice(processes, func(i, j int) bool {
		// Sort by combined CPU and memory usage (descending)
		scoreI := processes[i].CPUPercent + float64(processes[i].MemoryPerc)
		scoreJ := processes[j].CPUPercent + float64(processes[j].MemoryPerc)
		return scoreI > scoreJ
	})
}

func (m *Monitor) getProcessInfo(proc *process.Process) (ProcessInfo, error) {
	info := ProcessInfo{PID: proc.Pid}
	now := time.Now()

	// Get process name
	if name, err := proc.Name(); err == nil {
		info.Name = name
	}

	// Get CPU percentage
	if cpuPercent, err := proc.CPUPercent(); err == nil {
		info.CPUPercent = cpuPercent
	}

	// Get memory info
	if memInfo, err := proc.MemoryInfo(); err == nil {
		info.MemoryMB = float64(memInfo.RSS) / 1024 / 1024
	}

	if memPercent, err := proc.MemoryPercent(); err == nil {
		info.MemoryPerc = memPercent
	}

	// Get process creation time
	if createTime, err := proc.CreateTime(); err == nil {
		info.CreateTime = time.Unix(createTime/1000, 0)
	}

	// Get I/O counters and calculate rates
	if ioCounters, err := proc.IOCounters(); err == nil {
		info.DiskReadKB = float64(ioCounters.ReadBytes) / 1024
		info.DiskWriteKB = float64(ioCounters.WriteBytes) / 1024

		// Calculate rates if we have previous data
		m.mu.RLock()
		lastIO, exists := m.lastProcessIO[proc.Pid]
		m.mu.RUnlock()
		if exists {
			timeDiff := now.Sub(lastIO.Timestamp).Seconds()
			if timeDiff > 0 {
				readDiff := float64(ioCounters.ReadBytes - lastIO.ReadBytes)
				writeDiff := float64(ioCounters.WriteBytes - lastIO.WriteBytes)

				info.DiskReadRate = (readDiff / 1024) / timeDiff   // KB/s
				info.DiskWriteRate = (writeDiff / 1024) / timeDiff // KB/s

				// Calculate percentage of system I/O (simplified)
				m.mu.RLock()
				systemIOTotal := m.systemIOTotal
				m.mu.RUnlock()
				if systemIOTotal > 0 {
					info.DiskReadPerc = (info.DiskReadRate / systemIOTotal) * 100
					info.DiskWritePerc = (info.DiskWriteRate / systemIOTotal) * 100
				} else {
					// If no system total, use absolute rate as percentage (cap at 100%)
					info.DiskReadPerc = math.Min(info.DiskReadRate/1024, 100) // MB/s as rough percentage
					info.DiskWritePerc = math.Min(info.DiskWriteRate/1024, 100)
				}
			}
		}

		// Store current I/O counters for next calculation
		m.mu.Lock()
		m.lastProcessIO[proc.Pid] = &ProcessIOCounters{
			ReadBytes:  ioCounters.ReadBytes,
			WriteBytes: ioCounters.WriteBytes,
			Timestamp:  now,
		}
		m.mu.Unlock()
	}

	// Network I/O - simplified implementation (removed expensive connection enumeration)
	// Per-process network stats are very difficult to get accurately and efficiently
	// For now, we'll provide placeholder values to avoid expensive operations
	info.NetSentKB = 0
	info.NetRecvKB = 0
	info.NetSentRate = 0
	info.NetRecvRate = 0

	// Note: Real per-process network monitoring would require:
	// 1. Parsing /proc/net/* files and correlating with process file descriptors
	// 2. Using eBPF or other kernel-level monitoring
	// 3. Or using tools like netstat/ss which are also expensive
	// For this demo, we'll leave network monitoring as a placeholder

	return info, nil
}

func (m *Monitor) updateProcessTimeSeriesMetrics(processInfo ProcessInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pid := processInfo.PID
	if _, exists := m.processMetrics[pid]; !exists {
		m.processMetrics[pid] = NewProcessMetrics(m.metricsCapacity)
	}

	m.processMetrics[pid].AddMetric(
		time.Now(),
		processInfo.CPUPercent,
		processInfo.MemoryMB,
		processInfo.DiskReadRate,
		processInfo.DiskWriteRate,
		processInfo.DiskReadPerc,
		processInfo.DiskWritePerc,
		processInfo.NetSentRate,
		processInfo.NetRecvRate,
	)
}

func (m *Monitor) sortProcesses() {
	sort.Slice(m.processes, func(i, j int) bool {
		var result bool
		switch m.sortBy {
		case SortByPID:
			result = m.processes[i].PID < m.processes[j].PID
		case SortByName:
			result = m.processes[i].Name < m.processes[j].Name
		case SortByCPU:
			result = m.processes[i].CPUPercent < m.processes[j].CPUPercent
		case SortByMemory:
			result = m.processes[i].MemoryMB < m.processes[j].MemoryMB
		default:
			result = m.processes[i].PID < m.processes[j].PID
		}

		if m.sortDesc {
			return !result
		}
		return result
	})
}

// CleanupOldMetrics removes metrics for processes that no longer exist
func (m *Monitor) CleanupOldMetrics() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get current PIDs
	currentPIDs := make(map[int32]bool)
	for _, proc := range m.processes {
		currentPIDs[proc.PID] = true
	}

	// Remove metrics for non-existent processes
	for pid := range m.processMetrics {
		if !currentPIDs[pid] {
			delete(m.processMetrics, pid)
		}
	}
}
