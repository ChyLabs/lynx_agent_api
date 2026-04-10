package controller

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type CPUStat struct {
	User    uint64
	Nice    uint64
	System  uint64
	Idle    uint64
	IOWait  uint64
	IRQ     uint64
	SoftIRQ uint64
	Steal   uint64
}

type NodeResourceStats struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryUsed    uint64  `json:"memory_used"`
	MemoryTotal   uint64  `json:"memory_total"`
	MemoryPercent float64 `json:"memory_percent"`
	DiskUsed      uint64  `json:"disk_used"`
	DiskTotal     uint64  `json:"disk_total"`
	DiskPercent   float64 `json:"disk_percent"`
}

func parseCPUStat() (*CPUStat, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return nil, fmt.Errorf("failed to read /proc/stat")
	}

	line := scanner.Text()
	if !strings.HasPrefix(line, "cpu ") {
		return nil, fmt.Errorf("unexpected /proc/stat format")
	}

	fields := strings.Fields(line)
	if len(fields) < 8 {
		return nil, fmt.Errorf("insufficient CPU fields in /proc/stat")
	}

	stat := &CPUStat{}
	stat.User, _ = strconv.ParseUint(fields[1], 10, 64)
	stat.Nice, _ = strconv.ParseUint(fields[2], 10, 64)
	stat.System, _ = strconv.ParseUint(fields[3], 10, 64)
	stat.Idle, _ = strconv.ParseUint(fields[4], 10, 64)
	stat.IOWait, _ = strconv.ParseUint(fields[5], 10, 64)
	stat.IRQ, _ = strconv.ParseUint(fields[6], 10, 64)
	stat.SoftIRQ, _ = strconv.ParseUint(fields[7], 10, 64)
	if len(fields) > 8 {
		stat.Steal, _ = strconv.ParseUint(fields[8], 10, 64)
	}

	return stat, nil
}

func calculateCPUPercent(prev, curr *CPUStat) float64 {
	prevIdle := prev.Idle + prev.IOWait
	currIdle := curr.Idle + curr.IOWait

	prevNonIdle := prev.User + prev.Nice + prev.System + prev.IRQ + prev.SoftIRQ + prev.Steal
	currNonIdle := curr.User + curr.Nice + curr.System + curr.IRQ + curr.SoftIRQ + curr.Steal

	prevTotal := prevIdle + prevNonIdle
	currTotal := currIdle + currNonIdle

	totalDiff := currTotal - prevTotal
	idleDiff := currIdle - prevIdle

	if totalDiff == 0 {
		return 0.0
	}

	return float64(totalDiff-idleDiff) / float64(totalDiff) * 100.0
}

func getMemoryStats() (used, total uint64, percent float64, err error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0, err
	}
	defer file.Close()

	var memTotal, memFree, buffers, cached uint64

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		key := strings.TrimSuffix(fields[0], ":")
		value, _ := strconv.ParseUint(fields[1], 10, 64)

		switch key {
		case "MemTotal":
			memTotal = value * 1024
		case "MemFree":
			memFree = value * 1024
		case "Buffers":
			buffers = value * 1024
		case "Cached":
			cached = value * 1024
		}
	}

	used = memTotal - memFree - buffers - cached
	percent = 0.0
	if memTotal > 0 {
		percent = float64(used) / float64(memTotal) * 100.0
	}

	return used, memTotal, percent, nil
}

func getDiskStats() (used, total uint64, percent float64, err error) {
	cmd := exec.Command("df", "-B1", "/")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to execute df command: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return 0, 0, 0, fmt.Errorf("unexpected df output format")
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 5 {
		return 0, 0, 0, fmt.Errorf("insufficient fields in df output")
	}

	total, _ = strconv.ParseUint(fields[1], 10, 64)
	used, _ = strconv.ParseUint(fields[2], 10, 64)

	if total > 0 {
		percent = float64(used) / float64(total) * 100.0
	}

	return used, total, percent, nil
}

func getNodeResourceStats(prevCPU *CPUStat) (NodeResourceStats, *CPUStat, error) {
	stats := NodeResourceStats{}

	currCPU, err := parseCPUStat()
	if err != nil {
		return stats, prevCPU, fmt.Errorf("failed to get CPU stats: %w", err)
	}

	if prevCPU != nil {
		stats.CPUPercent = calculateCPUPercent(prevCPU, currCPU)
	}

	memUsed, memTotal, memPercent, err := getMemoryStats()
	if err != nil {
		return stats, currCPU, fmt.Errorf("failed to get memory stats: %w", err)
	}
	stats.MemoryUsed = memUsed
	stats.MemoryTotal = memTotal
	stats.MemoryPercent = memPercent

	diskUsed, diskTotal, diskPercent, err := getDiskStats()
	if err != nil {
		return stats, currCPU, fmt.Errorf("failed to get disk stats: %w", err)
	}
	stats.DiskUsed = diskUsed
	stats.DiskTotal = diskTotal
	stats.DiskPercent = diskPercent

	return stats, currCPU, nil
}

func NodeResourcesWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	done := make(chan struct{})

	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				close(done)
				return
			}
		}
	}()

	var prevCPU *CPUStat

	for {
		select {
		case <-done:
			log.Printf("WebSocket connection closed for node resources")
			return
		case <-ticker.C:
			stats, currCPU, err := getNodeResourceStats(prevCPU)
			prevCPU = currCPU

			response := map[string]interface{}{
				"timestamp": time.Now().Unix(),
			}

			if err != nil {
				response["error"] = err.Error()
			} else {
				response["cpu_percent"] = fmt.Sprintf("%.2f", stats.CPUPercent)
				response["memory_used"] = stats.MemoryUsed
				response["memory_total"] = stats.MemoryTotal
				response["memory_percent"] = fmt.Sprintf("%.2f", stats.MemoryPercent)
				response["disk_used"] = stats.DiskUsed
				response["disk_total"] = stats.DiskTotal
				response["disk_percent"] = fmt.Sprintf("%.2f", stats.DiskPercent)
			}

			if err := conn.WriteJSON(response); err != nil {
				log.Printf("WebSocket write error: %v", err)
				return
			}
		}
	}
}
