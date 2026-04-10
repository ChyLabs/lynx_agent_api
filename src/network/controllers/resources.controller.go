package controller

import (
	"fmt"
	"log"
	"lynx_agent_api/src/infra/db"
	schemas "lynx_agent_api/src/infra/schemas"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	golxc "github.com/lxc/go-lxc"
)

func getDiskUsage(containerName string) (int64, error) {
	diskPath := fmt.Sprintf("/var/lib/lxc/%s/disk.img", containerName)

	var stat syscall.Stat_t
	if err := syscall.Stat(diskPath, &stat); err != nil {
		return 0, fmt.Errorf("failed to stat disk image: %w", err)
	}

	return stat.Blocks * 512, nil
}

func getCpuUsage(containerName string) (int64, error) {
	cgroupPaths := []string{
		fmt.Sprintf("/sys/fs/cgroup/lxc.payload.%s/cpu.stat", containerName),
		fmt.Sprintf("/sys/fs/cgroup/lxc/%s/cpu.stat", containerName),
	}

	var lastErr error
	for _, path := range cgroupPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			lastErr = err
			continue
		}

		for _, line := range strings.Split(string(data), "\n") {
			if !strings.HasPrefix(line, "usage_usec ") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				break
			}
			usec, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				lastErr = fmt.Errorf("failed to parse usage_usec: %w", err)
				break
			}
			return usec * 1000, nil
		}
		lastErr = fmt.Errorf("usage_usec not found in %s", path)
	}

	return 0, fmt.Errorf("failed to read cpu from cgroup v2: %w", lastErr)
}

func getMemoryUsage(containerName string) (int64, error) {
	cgroupPaths := []string{
		fmt.Sprintf("/sys/fs/cgroup/lxc.payload.%s", containerName),
		fmt.Sprintf("/sys/fs/cgroup/lxc/%s", containerName),
	}

	var lastErr error
	for _, base := range cgroupPaths {
		currentData, err := os.ReadFile(base + "/memory.current")
		if err != nil {
			lastErr = err
			continue
		}

		memCurrent, err := strconv.ParseInt(strings.TrimSpace(string(currentData)), 10, 64)
		if err != nil {
			lastErr = fmt.Errorf("failed to parse memory.current: %w", err)
			continue
		}

		statData, err := os.ReadFile(base + "/memory.stat")
		if err != nil {
			return memCurrent, nil
		}

		var inactiveFile, slabReclaimable int64
		for _, line := range strings.Split(string(statData), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			val, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				continue
			}
			switch fields[0] {
			case "inactive_file":
				inactiveFile = val
			case "slab_reclaimable":
				slabReclaimable = val
			}
		}

		used := memCurrent - inactiveFile - slabReclaimable
		if used < 0 {
			used = 0
		}
		return used, nil
	}

	return 0, fmt.Errorf("failed to read memory from cgroup v2: %w", lastErr)
}

type cpuStats struct {
	lastCPUTime time.Duration
	lastMeasure time.Time
}

var (
	cpuStatsMap = make(map[string]*cpuStats)
	cpuStatsMu  sync.Mutex
	upgrader    = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func ServerResourcesWebSocket(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverUUID := vars["uuid"]

	if serverUUID == "" {
		http.Error(w, "Missing uuid path parameter", http.StatusBadRequest)
		return
	}

	var server schemas.Servers
	if err := db.DB.Where("uuid = ?", serverUUID).First(&server).Error; err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	c, err := golxc.NewContainer(server.SID)
	if err != nil {
		conn.WriteJSON(map[string]interface{}{
			"error": "Failed to initialize container handle",
		})
		return
	}
	defer c.Release()

	defer func() {
		cpuStatsMu.Lock()
		delete(cpuStatsMap, serverUUID)
		cpuStatsMu.Unlock()
	}()

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

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if !c.Running() {
				conn.WriteJSON(map[string]interface{}{
					"server_id":       serverUUID,
					"name":            server.Name,
					"running":         false,
					"cpu_percent":     0.0,
					"memory_usage":    0,
					"disk_usage":      0,
					"interface_stats": map[string]interface{}{},
					"timestamp":       time.Now().Unix(),
				})
				continue
			}

			var cpuPercent float64
			cpuTimeNs, cpuErr := getCpuUsage(server.SID)
			now := time.Now()

			if cpuErr == nil {
				cpuTime := time.Duration(cpuTimeNs)

				cpuStatsMu.Lock()
				stats, exists := cpuStatsMap[serverUUID]
				if exists {
					timeDelta := now.Sub(stats.lastMeasure).Seconds()
					cpuDelta := float64(cpuTime - stats.lastCPUTime)
					cpuCount := float64(server.Cpu)
					if cpuCount <= 0 {
						cpuCount = 1
					}
					if timeDelta > 0 {
						cpuPercent = (cpuDelta / 1e9 / timeDelta / cpuCount) * 100
						if cpuPercent < 0 {
							cpuPercent = 0
						} else if cpuPercent > 100 {
							cpuPercent = 100
						}
					}
					stats.lastCPUTime = cpuTime
					stats.lastMeasure = now
				} else {
					cpuStatsMap[serverUUID] = &cpuStats{
						lastCPUTime: cpuTime,
						lastMeasure: now,
					}
				}
				cpuStatsMu.Unlock()
			}

			memUsage, memErr := getMemoryUsage(server.SID)
			diskUsage, diskErr := getDiskUsage(server.SID)
			netStats, netErr := c.InterfaceStats()

			interfaceStats := make(map[string]map[string]int64)
			if netErr == nil {
				for iface, stats := range netStats {
					interfaceStats[iface] = make(map[string]int64)
					for key, val := range stats {
						interfaceStats[iface][key] = int64(val)
					}
				}
			}

			response := map[string]interface{}{
				"server_id": serverUUID,
				"name":      server.Name,
				"running":   true,
				"timestamp": time.Now().Unix(),
			}

			if cpuErr == nil {
				response["cpu_percent"] = cpuPercent
			} else {
				response["cpu_error"] = cpuErr.Error()
			}

			if memErr == nil {
				response["memory_usage"] = memUsage
			} else {
				response["memory_error"] = memErr.Error()
			}

			if diskErr == nil {
				response["disk_usage"] = diskUsage
			} else {
				response["disk_error"] = diskErr.Error()
			}

			if netErr == nil {
				response["interface_stats"] = interfaceStats
			} else {
				response["network_error"] = netErr.Error()
			}

			if err := conn.WriteJSON(response); err != nil {
				return
			}
		}
	}
}
