package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"lynx_agent_api/src/config"
	"lynx_agent_api/src/infra/db"
	schemas "lynx_agent_api/src/infra/schemas"
	"lynx_agent_api/src/lib"
	"lynx_agent_api/src/utils"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	golxc "github.com/lxc/go-lxc"
)

type CreateServerRequest struct {
	Name         string `json:"name"`
	Release      string `json:"release"`
	Archi        string `json:"archi"`
	Distro       string `json:"distro"`
	Memory       int    `json:"memory"`
	Cpu          int    `json:"cpu"`
	Disk         int    `json:"disk"`
	NodeId       string `json:"node_id"`
	UserId       string `json:"user_id"`
	Password     string `json:"password"`
	AllocationIP string `json:"allocation_ip,omitempty"`
	AllocationID string `json:"allocation_id,omitempty"`
}

type UpdateServerRequest struct {
	Memory *int `json:"memory,omitempty"`
	Cpu    *int `json:"cpu,omitempty"`
}

type DeleteServerRequest struct {
	Name string `json:"name"`
}

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func GetAllServers(w http.ResponseWriter, r *http.Request) {
	var servers []schemas.Servers
	if err := db.DB.Order("created_at DESC").Find(&servers).Error; err != nil {
		lib.HandleError(w, lib.BadRequest("Failed to retrieve servers", err))
		return
	}

	lib.Success(w, "Servers retrieved successfully", map[string]interface{}{
		"servers": servers,
	})
}

func GetAllServersByNodeId(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeUUID := vars["node_uuid"]

	if nodeUUID == "" {
		lib.HandleError(w, lib.BadRequest("Missing node_uuid path parameter", nil))
		return
	}

	var servers []schemas.Servers

	if err := db.DB.Where("node_id = ?", nodeUUID).Order("created_at DESC").Find(&servers).Error; err != nil {
		lib.HandleError(w, lib.BadRequest("Failed to retrieve servers", err))
		return
	}

	lib.Success(w, "Servers retrieved successfully", map[string]interface{}{
		"servers": servers,
	})
}

func GetAllServersByUserId(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userUUID := vars["user_uuid"]

	if userUUID == "" {
		lib.HandleError(w, lib.BadRequest("Missing user_uuid path parameter", nil))
		return
	}

	var servers []schemas.Servers

	if err := db.DB.Where("user_id = ?", userUUID).Order("created_at DESC").Find(&servers).Error; err != nil {
		lib.HandleError(w, lib.BadRequest("Failed to retrieve servers", err))
		return
	}

	lib.Success(w, "Servers retrieved successfully", map[string]interface{}{
		"servers": servers,
	})
}

func GetServerByAllocationID(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	allocationUUID := vars["allocation_uuid"]

	if allocationUUID == "" {
		lib.HandleError(w, lib.BadRequest("Missing allocation_uuid path parameter", nil))
		return
	}

	var server schemas.Servers
	if err := db.DB.Where("allocation_id = ?", allocationUUID).First(&server).Error; err != nil {
		lib.HandleError(w, lib.NotFound("Server not found"))
		return
	}

	lib.Success(w, "Server retrieved successfully", map[string]interface{}{
		"server": server,
	})
}

func GetServerByID(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uuid := vars["uuid"]

	if uuid == "" {
		lib.HandleError(w, lib.BadRequest("Missing uuid path parameter", nil))
		return
	}

	var server schemas.Servers
	if err := db.DB.Where("uuid = ?", uuid).First(&server).Error; err != nil {
		lib.HandleError(w, lib.NotFound("Server not found"))
		return
	}

	lib.Success(w, "Server retrieved successfully", map[string]interface{}{
		"server": server,
	})
}

func StartServer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverUUID := vars["uuid"]

	if serverUUID == "" {
		lib.HandleError(w, lib.BadRequest("Missing uuid path parameter", nil))
		return
	}

	var server schemas.Servers
	if err := db.DB.Where("uuid = ?", serverUUID).First(&server).Error; err != nil {
		lib.HandleError(w, lib.NotFound("Server not found"))
		return
	}

	sid := server.SID
	name := server.Name

	c, err := golxc.NewContainer(sid)
	if err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to initialize server handle", err))
		return
	}
	defer c.Release()

	if c.Running() {
		lib.HandleError(w, lib.BadRequest("Server is already running", nil))
		return
	}

	diskPath := fmt.Sprintf("/var/lib/lxc/%s/disk.img", sid)
	if _, err := os.Stat(diskPath); os.IsNotExist(err) {
		lib.HandleError(w, lib.InternalServerError("Server disk image not found", err))
		return
	}

	rootfsPath := fmt.Sprintf("/var/lib/lxc/%s/rootfs", sid)
	if err := os.MkdirAll(rootfsPath, 0755); err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to create rootfs directory", err))
		return
	}

	configPath := fmt.Sprintf("/var/lib/lxc/%s/config", sid)
	c.ClearConfig()
	if err := c.LoadConfigFile(configPath); err != nil {
		fmt.Printf("[LXC-%s] Config load error: %v\n", sid, err)
		lib.HandleError(w, lib.InternalServerError("Failed to load container config", err))
		return
	}

	if err := c.Start(); err != nil {
		fmt.Printf("[LXC-%s] Start error: %v\n", sid, err)
		lib.HandleError(w, lib.InternalServerError("Failed to start server", err))
		return
	}

	if err := db.DB.Model(&schemas.Servers{}).Where("id = ?", server.ID).Update("status", "RUNNING").Error; err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to update server status", err))
		return
	}

	lib.Success(w, "Server started successfully", map[string]string{
		"id":   serverUUID,
		"sid":  sid,
		"name": name,
	})
}

func StopServer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverUUID := vars["uuid"]

	if serverUUID == "" {
		lib.HandleError(w, lib.BadRequest("Missing uuid path parameter", nil))
		return
	}

	var server schemas.Servers
	if err := db.DB.Where("uuid = ?", serverUUID).First(&server).Error; err != nil {
		lib.HandleError(w, lib.NotFound("Server not found"))
		return
	}

	sid := server.SID
	name := server.Name

	c, err := golxc.NewContainer(sid)

	if err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to initialize server handle", err))
		return
	}

	defer c.Release()

	if !c.Running() {
		lib.HandleError(w, lib.BadRequest("Server is not running", nil))
		return
	}

	if err := c.Stop(); err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to stop server", err))
		return
	}

	if err := db.DB.Model(&schemas.Servers{}).Where("id = ?", server.ID).Update("status", "STOPPED").Error; err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to update server status", err))
		return
	}

	lib.Success(w, "Server stopped successfully", map[string]string{
		"id":   serverUUID,
		"sid":  sid,
		"name": name,
	})
}

func RestartServer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverUUID := vars["uuid"]

	if serverUUID == "" {
		lib.HandleError(w, lib.BadRequest("Missing uuid path parameter", nil))
		return
	}

	var server schemas.Servers
	if err := db.DB.Where("uuid = ?", serverUUID).First(&server).Error; err != nil {
		lib.HandleError(w, lib.NotFound("Server not found"))
		return
	}

	sid := server.SID
	name := server.Name

	c, err := golxc.NewContainer(sid)

	if err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to initialize server handle", err))
		return
	}

	defer c.Release()

	rootfsPath := fmt.Sprintf("/var/lib/lxc/%s/rootfs", sid)
	if err := os.MkdirAll(rootfsPath, 0755); err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to create rootfs directory", err))
		return
	}

	configPath := fmt.Sprintf("/var/lib/lxc/%s/config", sid)
	c.ClearConfig()
	if err := c.LoadConfigFile(configPath); err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to load container config", err))
		return
	}

	if err := c.Reboot(); err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to restart server", err))
		return
	}

	if err := db.DB.Model(&schemas.Servers{}).Where("uuid = ?", serverUUID).Update("status", "RUNNING").Error; err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to update server status", err))
		return
	}

	lib.Success(w, "Server restarted successfully", map[string]string{
		"id":   serverUUID,
		"sid":  sid,
		"name": name,
	})

}

func CreateServer(w http.ResponseWriter, r *http.Request) {
	var req CreateServerRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		lib.HandleError(w, lib.BadRequest("Invalid request body", err))
		return
	}

	if req.Name == "" || req.Release == "" || req.Archi == "" || req.UserId == "" || req.NodeId == "" || req.Distro == "" || req.Password == "" {
		lib.HandleError(w, lib.BadRequest("Missing required fields", nil))
		return
	}

	if req.Cpu <= 0 || req.Memory <= 0 || req.Disk <= 0 {
		lib.HandleError(w, lib.BadRequest("cpu, memory, and disk must be positive integers", nil))
		return
	}

	if !validName.MatchString(req.Name) || len(req.Name) > 64 {
		lib.HandleError(w, lib.BadRequest("Invalid server name", nil))
		return
	}

	if !validName.MatchString(req.Release) || !validName.MatchString(req.Archi) || !validName.MatchString(req.Distro) {
		lib.HandleError(w, lib.BadRequest("Invalid release, archi, or distro value", nil))
		return
	}

	if req.Disk < 512 {
		lib.HandleError(w, lib.BadRequest("Disk size must be at least 512 MB", nil))
		return
	}

	var server schemas.Servers
	if err := db.DB.Where("name = ?", req.Name).First(&server).Error; err == nil {
		switch server.Status {
		case schemas.ServerInstalling:
			lib.HandleError(w, lib.BadRequest("A server with this name is already being created", nil))
			return
		case schemas.ServerError:
			c, err := golxc.NewContainer(server.SID)
			if err == nil {
				if c.Running() {
					_ = c.Stop()
					c.Wait(golxc.STOPPED, 15*time.Second)
				}
				if c.Defined() {
					_ = c.Destroy()
				}
				c.Release()
			}
			_ = lib.ExecuteCommand(fmt.Sprintf("umount -R /var/lib/lxc/%s/rootfs 2>/dev/null", server.SID))
			_ = os.Remove(fmt.Sprintf("/var/lib/lxc/%s/disk.img", server.SID))
			_ = os.RemoveAll(fmt.Sprintf("/var/lib/lxc/%s", server.SID))
			if err := db.DB.Where("id = ?", server.ID).Delete(&schemas.Servers{}).Error; err != nil {
				lib.HandleError(w, lib.InternalServerError("Failed to clean up failed server before retry", err))
				return
			}
		default:
			lib.HandleError(w, lib.BadRequest("A server with this name already exists", nil))
			return
		}
	}

	var ctx = context.Background()
	sidNum, err := db.RedisClient.Incr(ctx, "lxc:id").Result()
	if err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to allocate server ID", err))
		return
	}
	sid := fmt.Sprintf("%d", sidNum)
	name := req.Name
	memory := strconv.Itoa(req.Memory)
	cpuCount := req.Cpu
	user_id := req.UserId
	node_id := req.NodeId
	disk := strconv.Itoa(req.Disk)

	lxcID, err := utils.GenID()
	if err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to generate server ID", err))
		return
	}
	lxcUUID, err := utils.GenUUID()
	if err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to generate server UUID", err))
		return
	}
	record := schemas.Servers{
		ID:     lxcID,
		UUID:   lxcUUID,
		SID:    sid,
		Name:   name,
		Memory: memory,
		Cpu:    cpuCount,
		Disk:   disk,
		NodeID: node_id,
		UserID: user_id,
		Status: "INSTALLING",
	}

	if err := db.DB.Create(&record).Error; err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to create server", err))
		return
	}

	go func() {
		createServerAsync(lxcID, lxcUUID, sid, name, memory, disk, cpuCount, req.Distro, req.Release, req.Archi, req.Password, req.AllocationIP, req.AllocationID)
	}()

	lib.Created(w, "Server creation started", map[string]string{
		"id":     lxcID,
		"uuid":   lxcUUID,
		"sid":    sid,
		"name":   name,
		"memory": memory,
		"cpu":    strconv.Itoa(cpuCount),
		"disk":   disk,
		"status": "INSTALLING",
	})
}

func createServerAsync(lxcID, lxcUUID, sid, name, memory, disk string, cpuCount int, distro, release, archi, password, allocationIP, allocationID string) {
	bridge := config.AppConfig.LXC.Bridge
	if iface, err := net.InterfaceByName(bridge); err != nil {
		_ = db.DB.Model(&schemas.Servers{}).Where("id = ?", lxcID).Update("status", "ERROR").Error
		return
	} else if iface.Flags&net.FlagUp == 0 {
		_ = db.DB.Model(&schemas.Servers{}).Where("id = ?", lxcID).Update("status", "ERROR").Error
		return
	}

	c, err := golxc.NewContainer(sid)
	if err != nil {
		_ = db.DB.Model(&schemas.Servers{}).Where("id = ?", lxcID).Update("status", "ERROR").Error
		return
	}
	defer c.Release()

	diskMounted := false

	cleanup := func(stepName string) {
		fmt.Printf("[LXC-%s] ERROR at step: %s\n", sid, stepName)
		_ = db.DB.Model(&schemas.Servers{}).Where("id = ?", lxcID).Update("status", "ERROR").Error

		if c.Running() {
			_ = c.Stop()
		}

		if diskMounted {
			_ = lib.ExecuteCommand(fmt.Sprintf("umount -R /var/lib/lxc/%s/rootfs 2>/dev/null", sid))
		}

		if c.Defined() {
			_ = c.Destroy()
		}

		_ = os.Remove(fmt.Sprintf("/var/lib/lxc/%s/disk.img", sid))
		_ = os.RemoveAll(fmt.Sprintf("/var/lib/lxc/%s", sid))
	}

	var installSSHCmd, installPackagesCmd, sshServiceName string
	distroLower := strings.ToLower(distro)

	switch {
	case strings.Contains(distroLower, "ubuntu"), strings.Contains(distroLower, "debian"):
		installSSHCmd = "DEBIAN_FRONTEND=noninteractive apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y openssh-server"
		installPackagesCmd = "DEBIAN_FRONTEND=noninteractive apt-get install -y nano git curl wget vim htop net-tools iputils-ping"
		sshServiceName = "ssh"
	case strings.Contains(distroLower, "alpine"):
		installSSHCmd = "apk update && apk add openssh"
		installPackagesCmd = "apk add nano git curl wget vim htop net-tools busybox-extras"
		sshServiceName = "sshd"
	case strings.Contains(distroLower, "centos"), strings.Contains(distroLower, "rocky"), strings.Contains(distroLower, "alma"), strings.Contains(distroLower, "rhel"), strings.Contains(distroLower, "fedora"):
		installSSHCmd = "if command -v dnf >/dev/null 2>&1; then dnf install -y openssh-server; else yum install -y openssh-server; fi"
		installPackagesCmd = "if command -v dnf >/dev/null 2>&1; then dnf install -y nano git curl wget vim htop net-tools iputils; else yum install -y nano git curl wget vim htop net-tools iputils; fi"
		sshServiceName = "sshd"
	case strings.Contains(distroLower, "arch"):
		installSSHCmd = "pacman -Syu --noconfirm openssh"
		installPackagesCmd = "pacman -S --noconfirm nano git curl wget vim htop net-tools iputils"
		sshServiceName = "sshd"
	case strings.Contains(distroLower, "opensuse"), strings.Contains(distroLower, "suse"):
		installSSHCmd = "zypper refresh && zypper install -y openssh"
		installPackagesCmd = "zypper install -y nano git curl wget vim htop net-tools iputils"
		sshServiceName = "sshd"
	default:
		installSSHCmd = "DEBIAN_FRONTEND=noninteractive apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y openssh-server"
		installPackagesCmd = "DEBIAN_FRONTEND=noninteractive apt-get install -y nano git curl wget vim htop net-tools iputils-ping"
		sshServiceName = "ssh"
	}

	steps := []struct {
		desc string
		fn   func() error
	}{

		{"create server", func() error {
			return c.Create(golxc.TemplateOptions{
				Template: "download",
				Distro:   distro,
				Release:  release,
				Arch:     archi,
			})
		}},

		{"backup original rootfs", func() error {
			return lib.ExecuteCommand(fmt.Sprintf(
				"mv /var/lib/lxc/%s/rootfs /var/lib/lxc/%s/rootfs.bak", sid, sid,
			))
		}},

		{"create sparse disk", func() error {
			return lib.ExecuteCommand(fmt.Sprintf(
				"truncate -s %sM /var/lib/lxc/%s/disk.img", disk, sid,
			))
		}},

		{"format disk", func() error {
			return lib.ExecuteCommand(fmt.Sprintf(
				"mkfs.ext4 -F -E lazy_itable_init=1,lazy_journal_init=1 /var/lib/lxc/%s/disk.img", sid,
			))
		}},

		{"recreate rootfs directory", func() error {
			return lib.ExecuteCommand(fmt.Sprintf(
				"mkdir -p /var/lib/lxc/%s/rootfs", sid,
			))
		}},

		{"verify rootfs.bak exists", func() error {
			rootfsBakPath := fmt.Sprintf("/var/lib/lxc/%s/rootfs.bak", sid)
			if info, err := os.Stat(rootfsBakPath); err != nil || !info.IsDir() {
				return fmt.Errorf("rootfs.bak not found or not a directory")
			}
			return lib.ExecuteCommand(fmt.Sprintf(
				"test $(find /var/lib/lxc/%s/rootfs.bak -mindepth 1 -maxdepth 1 | wc -l) -gt 0", sid,
			))
		}},

		{"mount disk", func() error {
			if err := lib.ExecuteCommand(fmt.Sprintf(
				"mount -o loop,noatime,nodiratime /var/lib/lxc/%s/disk.img /var/lib/lxc/%s/rootfs", sid, sid,
			)); err != nil {
				return err
			}
			diskMounted = true
			return nil
		}},

		{"verify disk mounted", func() error {
			return lib.ExecuteCommand(fmt.Sprintf(
				"mountpoint -q /var/lib/lxc/%s/rootfs", sid,
			))
		}},

		{"copy rootfs (fast)", func() error {
			return lib.ExecuteCommand(fmt.Sprintf(
				"tar -C /var/lib/lxc/%s/rootfs.bak -cf - . | tar -C /var/lib/lxc/%s/rootfs -xf -", sid, sid,
			))
		}},

		{"verify rootfs copied", func() error {
			return lib.ExecuteCommand(fmt.Sprintf(
				"test $(find /var/lib/lxc/%s/rootfs -mindepth 1 -maxdepth 1 | wc -l) -gt 0", sid,
			))
		}},

		{"remove backup rootfs", func() error {
			return lib.ExecuteCommand(fmt.Sprintf(
				"rm -rf /var/lib/lxc/%s/rootfs.bak", sid,
			))
		}},

		{"install systemd (chroot)", func() error {
			rootfsPath := fmt.Sprintf("/var/lib/lxc/%s/rootfs", sid)

			systemdPaths := []string{
				fmt.Sprintf("%s/lib/systemd/systemd", rootfsPath),
				fmt.Sprintf("%s/usr/lib/systemd/systemd", rootfsPath),
			}

			for _, path := range systemdPaths {
				if _, err := os.Stat(path); err == nil {
					return nil
				}
			}

			defer func() {
				for _, dir := range []string{"dev", "sys", "proc"} {
					_ = lib.ExecuteCommand(fmt.Sprintf("umount %s/%s 2>/dev/null", rootfsPath, dir))
				}
			}()

			for _, dir := range []string{"proc", "sys", "dev"} {
				target := fmt.Sprintf("%s/%s", rootfsPath, dir)
				if err := os.MkdirAll(target, 0755); err != nil {
					return fmt.Errorf("failed to create %s in rootfs: %w", dir, err)
				}
				if err := lib.ExecuteCommand(fmt.Sprintf("mount --bind /%s %s", dir, target)); err != nil {
					return fmt.Errorf("failed to bind-mount /%s into rootfs: %w", dir, err)
				}
			}

			var installCmd string
			distroLower := strings.ToLower(distro)

			switch {
			case strings.Contains(distroLower, "ubuntu"), strings.Contains(distroLower, "debian"):
				installCmd = fmt.Sprintf("chroot %s /bin/sh -c 'DEBIAN_FRONTEND=noninteractive apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y systemd systemd-sysv'", rootfsPath)
			case strings.Contains(distroLower, "centos"), strings.Contains(distroLower, "rocky"), strings.Contains(distroLower, "alma"), strings.Contains(distroLower, "rhel"), strings.Contains(distroLower, "fedora"):
				installCmd = fmt.Sprintf("chroot %s /bin/sh -c 'if command -v dnf >/dev/null 2>&1; then dnf install -y systemd; else yum install -y systemd; fi'", rootfsPath)
			case strings.Contains(distroLower, "arch"):
				installCmd = fmt.Sprintf("chroot %s /bin/sh -c 'pacman -Syu --noconfirm systemd'", rootfsPath)
			case strings.Contains(distroLower, "opensuse"), strings.Contains(distroLower, "suse"):
				installCmd = fmt.Sprintf("chroot %s /bin/sh -c 'zypper refresh && zypper install -y systemd'", rootfsPath)
			case strings.Contains(distroLower, "alpine"):
				return nil
			default:
				installCmd = fmt.Sprintf("chroot %s /bin/sh -c 'DEBIAN_FRONTEND=noninteractive apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y systemd systemd-sysv'", rootfsPath)
			}

			return lib.ExecuteCommand(installCmd)
		}},

		{"configure container", func() error {
			configPath := fmt.Sprintf("/var/lib/lxc/%s/config", sid)

			memoryMB, _ := strconv.Atoi(memory)
			if memoryMB <= 0 {
				memoryMB = 512
			}
			if cpuCount <= 0 {
				cpuCount = 1
			}

			c.ClearConfig()
			cpuSet := "0"
			if cpuCount > 1 {
				cpuSet = fmt.Sprintf("0-%d", cpuCount-1)
			}

			bridge := config.AppConfig.LXC.Bridge

			_ = os.MkdirAll("/var/log/lxc", 0755)

			configItems := [][2]string{
				{"lxc.include", "/usr/share/lxc/config/common.conf"},
				{"lxc.arch", "linux64"},
				{"lxc.rootfs.path", fmt.Sprintf("loop:/var/lib/lxc/%s/disk.img", sid)},
				{"lxc.uts.name", name},
				{"lxc.net.0.type", "veth"},
				{"lxc.net.0.link", bridge},
				{"lxc.net.0.flags", "up"},
				{"lxc.start.auto", "1"},
				{"lxc.tty.max", "0"},
				{"lxc.pty.max", "1024"},
				{"lxc.console.path", "none"},
				{"lxc.console.logfile", fmt.Sprintf("/var/log/lxc/%s.log", sid)},
				{"lxc.init.cmd", "/sbin/init"},
				{"lxc.apparmor.profile", "unconfined"},
				{"lxc.apparmor.allow_nesting", "1"},
				{"lxc.cap.drop", "sys_module mac_admin mac_override"},
				{"lxc.cgroup.devices.allow", "c 10:200 rwm"},
				{"lxc.mount.auto", "proc:mixed sys:mixed cgroup:mixed"},
				{"lxc.cgroup2.memory.max", fmt.Sprintf("%dM", memoryMB)},
				{"lxc.cgroup2.memory.swap.max", fmt.Sprintf("%dM", memoryMB)},
				{"lxc.cgroup2.cpuset.cpus", cpuSet},
				{"lxc.cgroup2.cpu.max", fmt.Sprintf("%d 100000", cpuCount*100000)},
			}

			if allocationIP != "" {
				ipv4Addr := allocationIP
				if !strings.Contains(ipv4Addr, "/") {
					ipv4Addr = allocationIP + "/24"
				}

				iface, err := net.InterfaceByName(bridge)
				if err != nil {
					return fmt.Errorf("bridge %q not found", bridge)
				}
				if iface.Flags&net.FlagUp == 0 {
					return fmt.Errorf("bridge %q is down", bridge)
				}

				addrs, err := iface.Addrs()
				if err != nil {
					return fmt.Errorf("failed to get bridge addresses: %w", err)
				}

				var gateway string
				for _, addr := range addrs {
					if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
						gateway = ipNet.IP.String()
						break
					}
				}
				if gateway == "" {
					return fmt.Errorf("no IPv4 gateway found on bridge %q", bridge)
				}

				configItems = append(configItems,
					[2]string{"lxc.net.0.ipv4.address", ipv4Addr},
					[2]string{"lxc.net.0.ipv4.gateway", gateway},
				)

				if allocationID != "" {
					_ = db.DB.Model(&schemas.Servers{}).Where("id = ?", lxcID).Update("allocation_id", allocationID).Error
				}
			}

			for _, item := range configItems {
				if err := c.SetConfigItem(item[0], item[1]); err != nil {
					return fmt.Errorf("failed to set %s: %w", item[0], err)
				}
			}

			if err := c.SaveConfigFile(configPath); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			return nil
		}},

		{"fix init system symlink", func() error {
			rootfsPath := fmt.Sprintf("/var/lib/lxc/%s/rootfs", sid)
			initPath := fmt.Sprintf("%s/sbin/init", rootfsPath)

			if _, err := os.Stat(initPath); os.IsNotExist(err) {
				systemdPaths := []string{
					fmt.Sprintf("%s/lib/systemd/systemd", rootfsPath),
					fmt.Sprintf("%s/usr/lib/systemd/systemd", rootfsPath),
				}

				for _, systemdPath := range systemdPaths {
					if _, err := os.Stat(systemdPath); err == nil {
						_ = os.MkdirAll(fmt.Sprintf("%s/sbin", rootfsPath), 0755)
						relPath := strings.TrimPrefix(systemdPath, rootfsPath)
						return lib.ExecuteCommand(fmt.Sprintf("ln -sf %s %s", relPath, initPath))
					}
				}
			}
			return nil
		}},

		{"verify init system", func() error {
			rootfsPath := fmt.Sprintf("/var/lib/lxc/%s/rootfs", sid)
			initPath := fmt.Sprintf("%s/sbin/init", rootfsPath)

			if _, err := os.Stat(initPath); os.IsNotExist(err) {
				return fmt.Errorf("/sbin/init not found in container rootfs")
			}

			return nil
		}},

		{"unmount disk before LXC starts", func() error {
			if err := lib.ExecuteCommand(fmt.Sprintf(
				"umount /var/lib/lxc/%s/rootfs", sid,
			)); err != nil {
				return err
			}
			diskMounted = false

			if _, err := os.Stat(fmt.Sprintf("/var/lib/lxc/%s/rootfs", sid)); os.IsNotExist(err) {
				return fmt.Errorf("rootfs directory disappeared after unmount")
			}
			return nil
		}},

		{"start server", func() error {
			err := c.Start()
			if err != nil {
				return fmt.Errorf("failed to start container: %w", err)
			}
			return nil
		}},

		{"wait for server to boot", func() error {
			if !c.Wait(golxc.RUNNING, 60*time.Second) {
				return fmt.Errorf("server did not start in time")
			}
			time.Sleep(5 * time.Second)
			return nil
		}},

		{"set root password", func() error {
			_, err := c.RunCommand([]string{"/bin/sh", "-c", fmt.Sprintf("echo 'root:%s' | chpasswd", password)}, golxc.DefaultAttachOptions)
			return err
		}},

		{"install openssh-server", func() error {
			_, err := c.RunCommand([]string{"/bin/sh", "-c", installSSHCmd}, golxc.DefaultAttachOptions)
			return err
		}},

		{"install essential packages", func() error {
			_, err := c.RunCommand([]string{"/bin/sh", "-c", installPackagesCmd}, golxc.DefaultAttachOptions)
			return err
		}},

		{"configure sshd for root login", func() error {
			_, err := c.RunCommand([]string{"/bin/sh", "-c", "sed -i 's/^#*PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config"}, golxc.DefaultAttachOptions)
			return err
		}},

		{"restart ssh service", func() error {
			_, err := c.RunCommand([]string{"/bin/sh", "-c", fmt.Sprintf("systemctl restart %s || service %s restart", sshServiceName, sshServiceName)}, golxc.DefaultAttachOptions)
			return err
		}},

		{"set hostname", func() error {
			_, err := c.RunCommand([]string{"/bin/sh", "-c", fmt.Sprintf("hostnamectl set-hostname %s 2>/dev/null || hostname %s && echo %s > /etc/hostname", name, name, name)}, golxc.DefaultAttachOptions)
			return err
		}},
	}

	for i, step := range steps {
		fmt.Printf("[LXC-%s] Step %d/%d: %s\n", sid, i+1, len(steps), step.desc)
		if err := step.fn(); err != nil {
			fmt.Printf("[LXC-%s] Step failed: %s - Error: %v\n", sid, step.desc, err)
			if i > 0 {
				cleanup(step.desc)
			} else {
				_ = db.DB.Model(&schemas.Servers{}).Where("id = ?", lxcID).Update("status", "ERROR").Error
			}
			return
		}
	}

	fmt.Printf("[LXC-%s] Server creation completed successfully\n", sid)

	_ = db.DB.Model(&schemas.Servers{}).Where("id = ?", lxcID).Update("status", "RUNNING").Error
}

func UpdateServer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverUUID := vars["uuid"]

	var req UpdateServerRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		lib.HandleError(w, lib.BadRequest("Invalid request body", err))
		return
	}

	if req.Memory == nil && req.Cpu == nil {
		lib.HandleError(w, lib.BadRequest("At least one of memory or cpu must be provided", nil))
		return
	}

	if req.Memory != nil && *req.Memory <= 0 {
		lib.HandleError(w, lib.BadRequest("memory must be a positive integer", nil))
		return
	}

	if req.Cpu != nil && *req.Cpu <= 0 {
		lib.HandleError(w, lib.BadRequest("cpu must be a positive integer", nil))
		return
	}

	var server schemas.Servers
	if err := db.DB.Where("uuid = ?", serverUUID).First(&server).Error; err != nil {
		lib.HandleError(w, lib.NotFound("Server not found"))
		return
	}

	c, err := golxc.NewContainer(server.SID)
	if err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to initialize server handle", err))
		return
	}
	defer c.Release()

	if c.Running() {
		if err := c.Stop(); err != nil {
			lib.HandleError(w, lib.InternalServerError("Failed to stop server", err))
			return
		}
		if !c.Wait(golxc.STOPPED, 30*time.Second) {
			lib.HandleError(w, lib.InternalServerError("Timed out waiting for server to stop", nil))
			return
		}
	}

	if err := db.DB.Model(&schemas.Servers{}).Where("id = ?", server.ID).Update("status", "STOPPED").Error; err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to update server status", err))
		return
	}

	configPath := fmt.Sprintf("/var/lib/lxc/%s/config", server.SID)
	c.ClearConfig()
	if err := c.LoadConfigFile(configPath); err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to load server config", err))
		return
	}

	if req.Memory != nil {
		_ = c.ClearConfigItem("lxc.cgroup2.memory.max")
		_ = c.ClearConfigItem("lxc.cgroup2.memory.swap.max")

		if err := c.SetConfigItem("lxc.cgroup2.memory.max", fmt.Sprintf("%dM", *req.Memory)); err != nil {
			lib.HandleError(w, lib.InternalServerError("Failed to update memory limit", err))
			return
		}
		if err := c.SetConfigItem("lxc.cgroup2.memory.swap.max", fmt.Sprintf("%dM", *req.Memory)); err != nil {
			lib.HandleError(w, lib.InternalServerError("Failed to update swap limit", err))
			return
		}
	}

	if req.Cpu != nil {
		_ = c.ClearConfigItem("lxc.cgroup2.cpuset.cpus")
		_ = c.ClearConfigItem("lxc.cgroup2.cpu.max")

		cpuSet := fmt.Sprintf("0-%d", *req.Cpu-1)
		if *req.Cpu == 1 {
			cpuSet = "0"
		}
		if err := c.SetConfigItem("lxc.cgroup2.cpuset.cpus", cpuSet); err != nil {
			lib.HandleError(w, lib.InternalServerError("Failed to update server cpuset", err))
			return
		}
		if err := c.SetConfigItem("lxc.cgroup2.cpu.max", fmt.Sprintf("%d 100000", *req.Cpu*100000)); err != nil {
			lib.HandleError(w, lib.InternalServerError("Failed to update server cpu quota", err))
			return
		}
	}

	if err := c.SaveConfigFile(configPath); err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to save server config", err))
		return
	}

	updates := map[string]interface{}{}
	if req.Memory != nil {
		updates["memory"] = strconv.Itoa(*req.Memory)
	}
	if req.Cpu != nil {
		updates["cpu"] = *req.Cpu
	}

	if err := db.DB.Model(&schemas.Servers{}).Where("id = ?", server.ID).Updates(updates).Error; err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to update server", err))
		return
	}

	if err := c.Start(); err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to start server", err))
		return
	}

	if err := db.DB.Model(&schemas.Servers{}).Where("id = ?", server.ID).Update("status", "RUNNING").Error; err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to update server status", err))
		return
	}

	lib.Success(w, "Server updated successfully", map[string]string{
		"id":   server.ID,
		"uuid": server.UUID,
		"sid":  server.SID,
		"name": server.Name,
	})
}

func DeleteServer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverUUID := vars["uuid"]

	if serverUUID == "" {
		lib.HandleError(w, lib.BadRequest("Missing server UUID", nil))
		return
	}

	var server schemas.Servers
	if err := db.DB.Where("uuid = ?", serverUUID).First(&server).Error; err != nil {
		lib.HandleError(w, lib.NotFound("Server not found"))
		return
	}

	c, err := golxc.NewContainer(server.SID)
	if err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to initialize server handle", err))
		return
	}
	defer c.Release()

	if c.Running() {
		if err := c.Stop(); err != nil {
			lib.HandleError(w, lib.InternalServerError("Failed to stop server", err))
			return
		}
		if !c.Wait(golxc.STOPPED, 30*time.Second) {
			lib.HandleError(w, lib.InternalServerError("Timed out waiting for server to stop", nil))
			return
		}
	}

	if err := db.DB.Model(&schemas.Servers{}).Where("id = ?", server.ID).Update("status", "STOPPED").Error; err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to update server status", err))
		return
	}

	_ = lib.ExecuteCommand(fmt.Sprintf("umount -R /var/lib/lxc/%s/rootfs 2>/dev/null", server.SID))

	if c.Defined() {
		if err := c.Destroy(); err != nil {
			lib.HandleError(w, lib.InternalServerError("Failed to destroy server", err))
			return
		}
	}

	_ = os.Remove(fmt.Sprintf("/var/lib/lxc/%s/disk.img", server.SID))
	_ = os.RemoveAll(fmt.Sprintf("/var/lib/lxc/%s", server.SID))

	if err := db.DB.Where("id = ?", server.ID).Delete(&schemas.Servers{}).Error; err != nil {
		lib.HandleError(w, lib.InternalServerError("Failed to delete server from database", err))
		return
	}

	lib.Success(w, "Server deleted successfully", map[string]string{
		"id":     serverUUID,
		"sid":    server.SID,
		"name":   server.Name,
		"memory": server.Memory,
		"disk":   server.Disk,
	})
}
