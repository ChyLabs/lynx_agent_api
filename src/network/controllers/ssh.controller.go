package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"lynx_agent_api/src/infra/db"
	schemas "lynx_agent_api/src/infra/schemas"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/creack/pty"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	golxc "github.com/lxc/go-lxc"
)

var sshUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type TerminalSession struct {
	ID           string
	ServerID     string
	PTY          *os.File
	CMD          *exec.Cmd
	Clients      map[*websocket.Conn]bool
	ClientsMutex sync.Mutex
	OutputBuffer []byte
	LastActivity time.Time
	Done         chan struct{}
}

var (
	sessions      = make(map[string]*TerminalSession)
	sessionsMutex sync.RWMutex
)

func init() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cleanupInactiveSessions()
		}
	}()
}

func cleanupInactiveSessions() {
	sessionsMutex.Lock()
	defer sessionsMutex.Unlock()

	now := time.Now()
	for id, session := range sessions {
		session.ClientsMutex.Lock()
		hasClients := len(session.Clients) > 0
		session.ClientsMutex.Unlock()

		if !hasClients && now.Sub(session.LastActivity) > 30*time.Minute {
			log.Printf("Cleaning up inactive session: %s", id)
			session.PTY.Close()
			session.CMD.Process.Kill()
			close(session.Done)
			delete(sessions, id)
		}
	}
}

func SSHTerminalWebSocket(w http.ResponseWriter, r *http.Request) {
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

	conn, err := sshUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	c, err := golxc.NewContainer(server.SID)
	if err != nil {
		log.Printf("Failed to initialize container handle for %s: %v", server.SID, err)
		conn.WriteJSON(map[string]interface{}{
			"type":  "error",
			"error": fmt.Sprintf("Container not found: %s", server.SID),
		})
		return
	}
	defer c.Release()

	if !c.Running() {
		log.Printf("Container %s is not running", server.SID)
		conn.WriteJSON(map[string]interface{}{
			"type":  "error",
			"error": "Container is not running. Please start the server first.",
		})
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	var session *TerminalSession

	if sessionID != "" {
		sessionsMutex.RLock()
		session = sessions[sessionID]
		sessionsMutex.RUnlock()

		if session != nil && session.ServerID == server.ID {
			log.Printf("Reconnecting to session: %s", sessionID)
			conn.WriteJSON(map[string]interface{}{
				"type":       "reconnect",
				"session_id": sessionID,
			})

			if len(session.OutputBuffer) > 0 {
				conn.WriteMessage(websocket.BinaryMessage, session.OutputBuffer)
			}
		} else {
			session = nil
			log.Printf("Session not found or mismatched: %s", sessionID)
		}
	}

	if session == nil {
		sessionID = fmt.Sprintf("%s-%d", server.ID, time.Now().UnixNano())
		session = createTerminalSession(sessionID, server.ID, server.SID)
		if session == nil {
			conn.WriteJSON(map[string]interface{}{
				"type":  "error",
				"error": "Failed to create terminal session",
			})
			return
		}

		sessionsMutex.Lock()
		sessions[sessionID] = session
		sessionsMutex.Unlock()

		conn.WriteJSON(map[string]interface{}{
			"type":       "connected",
			"session_id": sessionID,
		})

		log.Printf("Created new session: %s", sessionID)
	}

	session.ClientsMutex.Lock()
	for oldClient := range session.Clients {
		log.Printf("[Session %s] Disconnecting old client %p due to new connection", sessionID, oldClient)
		oldClient.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "New connection established"))
		oldClient.Close()
		delete(session.Clients, oldClient)
	}
	session.Clients[conn] = true
	session.LastActivity = time.Now()
	session.ClientsMutex.Unlock()

	defer func() {
		session.ClientsMutex.Lock()
		delete(session.Clients, conn)
		session.LastActivity = time.Now()
		session.ClientsMutex.Unlock()
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				return
			}

			if messageType == websocket.TextMessage {
				var msg map[string]interface{}
				if err := json.Unmarshal(message, &msg); err == nil {
					if msg["type"] == "resize" {
						if rows, ok := msg["rows"].(float64); ok {
							if cols, ok := msg["cols"].(float64); ok {
								ws := pty.Winsize{
									Rows: uint16(rows),
									Cols: uint16(cols),
								}
								if err := pty.Setsize(session.PTY, &ws); err != nil {
									log.Printf("Failed to resize PTY: %v", err)
								} else {
									log.Printf("[Session %s] Resized PTY to %dx%d", sessionID, int(cols), int(rows))
								}
							}
						}
					}
				}
				continue
			}

			log.Printf("[Session %s] Writing %d bytes to PTY from client %p", sessionID, len(message), conn)
			if _, err := session.PTY.Write(message); err != nil {
				log.Printf("PTY write error: %v", err)
				return
			}
			session.LastActivity = time.Now()
		}
	}()

	<-done
}

func setRawMode(fd uintptr) error {
	termios := &syscall.Termios{}

	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TCGETS, uintptr(unsafe.Pointer(termios))); err != 0 {
		return err
	}

	termios.Lflag &^= syscall.ECHO
	termios.Lflag &^= syscall.ECHOE  // Don't echo erase character
	termios.Lflag &^= syscall.ECHOK  // Don't echo kill character
	termios.Lflag &^= syscall.ECHONL // Don't echo newline

	termios.Lflag &^= syscall.ICANON
	termios.Lflag &^= syscall.ISIG
	termios.Iflag &^= syscall.IXON
	termios.Iflag &^= syscall.ICRNL

	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TCSETS, uintptr(unsafe.Pointer(termios))); err != 0 {
		return err
	}

	return nil
}

func createTerminalSession(sessionID, serverID, containerName string) *TerminalSession {
	log.Printf("Creating terminal session for container: %s (serverID: %s, sessionID: %s)", containerName, serverID, sessionID)

	bashWrapper := `#!/bin/bash
# Override dangerous commands
poweroff() { echo -e "\033[31mError: Command blocked. Use the control panel to manage server power state.\033[0m"; }
shutdown() { echo -e "\033[31mError: Command blocked. Use the control panel to manage server power state.\033[0m"; }
halt() { echo -e "\033[31mError: Command blocked. Use the control panel to manage server power state.\033[0m"; }
reboot() { echo -e "\033[31mError: Command blocked. Use the control panel to manage server power state.\033[0m"; }
export -f poweroff shutdown halt reboot
export HOME=/root
export TERM=xterm-256color
cd ~
exec bash -i
`

	cmd := exec.Command("lxc-attach", "-n", containerName, "--", "bash", "-c", bashWrapper)
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")

	log.Printf("Executing: lxc-attach -n %s -- bash -c <wrapper>", containerName)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("Failed to start PTY for container %s: %v", containerName, err)
		return nil
	}

	if err := setRawMode(ptmx.Fd()); err != nil {
		log.Printf("Warning: Failed to set raw mode: %v", err)
	}

	ws := pty.Winsize{Rows: 24, Cols: 80}
	if err := pty.Setsize(ptmx, &ws); err != nil {
		log.Printf("Warning: Failed to set initial PTY size: %v", err)
	}

	session := &TerminalSession{
		ID:           sessionID,
		ServerID:     serverID,
		PTY:          ptmx,
		CMD:          cmd,
		Clients:      make(map[*websocket.Conn]bool),
		OutputBuffer: make([]byte, 0, 65536),
		LastActivity: time.Now(),
		Done:         make(chan struct{}),
	}

	go func() {
		for {
			select {
			case <-session.Done:
				return
			default:
				buf := make([]byte, 8192)
				session.PTY.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
				n, err := session.PTY.Read(buf)

				if n > 0 {
					data := buf[:n]

					session.OutputBuffer = append(session.OutputBuffer, data...)
					if len(session.OutputBuffer) > 65536 {
						session.OutputBuffer = session.OutputBuffer[len(session.OutputBuffer)-65536:]
					}

					session.ClientsMutex.Lock()
					for client := range session.Clients {
						if err := client.WriteMessage(websocket.BinaryMessage, data); err != nil {
							log.Printf("WebSocket write error: %v", err)
							client.Close()
							delete(session.Clients, client)
						}
					}
					session.ClientsMutex.Unlock()
				}

				if err != nil && err != os.ErrDeadlineExceeded {
					if err != io.EOF {
						log.Printf("PTY read error: %v", err)
					}
					return
				}
			}
		}
	}()

	go func() {
		cmd.Wait()
		close(session.Done)

		session.ClientsMutex.Lock()
		for client := range session.Clients {
			client.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Session ended"))
			client.Close()
		}
		session.ClientsMutex.Unlock()

		time.Sleep(5 * time.Minute)
		sessionsMutex.Lock()
		delete(sessions, sessionID)
		sessionsMutex.Unlock()
		log.Printf("Session ended and cleaned up: %s", sessionID)
	}()

	return session
}
