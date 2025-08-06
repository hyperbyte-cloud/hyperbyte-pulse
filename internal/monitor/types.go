package monitor

import "time"

// ProcessInfo represents information about a single process
type ProcessInfo struct {
	PID           int32
	Name          string
	CPUPercent    float64
	MemoryMB      float64
	MemoryPerc    float32
	CreateTime    time.Time
	DiskReadKB    float64
	DiskWriteKB   float64
	DiskReadRate  float64 // KB/s
	DiskWriteRate float64 // KB/s
	DiskReadPerc  float64 // Percentage of system I/O
	DiskWritePerc float64 // Percentage of system I/O
	NetSentKB     float64
	NetRecvKB     float64
	NetSentRate   float64 // KB/s
	NetRecvRate   float64 // KB/s
}

// SystemMetrics represents overall system metrics
type SystemMetrics struct {
	CPUPercent    float64
	MemoryPercent float64
	TotalMemoryMB float64
	UsedMemoryMB  float64
	Timestamp     time.Time
}

// ProcessMetrics represents time-series metrics for a single process
type ProcessMetrics struct {
	Timestamps    []time.Time
	CPUPercent    []float64
	MemoryMB      []float64
	DiskReadRate  []float64 // KB/s
	DiskWriteRate []float64 // KB/s
	DiskReadPerc  []float64 // Percentage
	DiskWritePerc []float64 // Percentage
	NetSentRate   []float64 // KB/s
	NetRecvRate   []float64 // KB/s
}

// NewProcessMetrics creates a new ProcessMetrics with specified capacity
func NewProcessMetrics(capacity int) *ProcessMetrics {
	return &ProcessMetrics{
		Timestamps:    make([]time.Time, 0, capacity),
		CPUPercent:    make([]float64, 0, capacity),
		MemoryMB:      make([]float64, 0, capacity),
		DiskReadRate:  make([]float64, 0, capacity),
		DiskWriteRate: make([]float64, 0, capacity),
		DiskReadPerc:  make([]float64, 0, capacity),
		DiskWritePerc: make([]float64, 0, capacity),
		NetSentRate:   make([]float64, 0, capacity),
		NetRecvRate:   make([]float64, 0, capacity),
	}
}

// AddMetric adds a new data point to the metrics
func (pm *ProcessMetrics) AddMetric(timestamp time.Time, cpu, memory, diskReadRate, diskWriteRate, diskReadPerc, diskWritePerc, netSentRate, netRecvRate float64) {
	pm.Timestamps = append(pm.Timestamps, timestamp)
	pm.CPUPercent = append(pm.CPUPercent, cpu)
	pm.MemoryMB = append(pm.MemoryMB, memory)
	pm.DiskReadRate = append(pm.DiskReadRate, diskReadRate)
	pm.DiskWriteRate = append(pm.DiskWriteRate, diskWriteRate)
	pm.DiskReadPerc = append(pm.DiskReadPerc, diskReadPerc)
	pm.DiskWritePerc = append(pm.DiskWritePerc, diskWritePerc)
	pm.NetSentRate = append(pm.NetSentRate, netSentRate)
	pm.NetRecvRate = append(pm.NetRecvRate, netRecvRate)

	// Keep only the last N metrics (rolling window)
	capacity := cap(pm.Timestamps)
	if len(pm.Timestamps) > capacity {
		pm.Timestamps = pm.Timestamps[1:]
		pm.CPUPercent = pm.CPUPercent[1:]
		pm.MemoryMB = pm.MemoryMB[1:]
		pm.DiskReadRate = pm.DiskReadRate[1:]
		pm.DiskWriteRate = pm.DiskWriteRate[1:]
		pm.DiskReadPerc = pm.DiskReadPerc[1:]
		pm.DiskWritePerc = pm.DiskWritePerc[1:]
		pm.NetSentRate = pm.NetSentRate[1:]
		pm.NetRecvRate = pm.NetRecvRate[1:]
	}
}
