package login

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"time"
)

// SessionState represents the current state of a login session.
type SessionState string

const (
	StateStarting    SessionState = "starting"
	StateWaitingScan SessionState = "waiting_scan"
	StateScanned     SessionState = "scanned"
	StateSuccess     SessionState = "success"
	StateExpired     SessionState = "expired"
	StateError       SessionState = "error"
)

// LoginSession tracks a single platform's login process.
type LoginSession struct {
	mu       sync.Mutex
	Platform string
	State    SessionState
	QRImage  string // base64 PNG
	Nickname string
	Cookies  string
	Error    string
	cmd      *exec.Cmd
	cancel   context.CancelFunc
}

// Manager manages login sessions for multiple platforms.
type Manager struct {
	mu         sync.RWMutex
	sessions   map[string]*LoginSession
	scriptPath string
	pythonPath string
	onSuccess  func(platform, cookies, nickname string)
}

// NewManager creates a new login manager.
// onSuccess is called when a login succeeds, allowing the caller to persist cookies.
func NewManager(scriptPath, pythonPath string, onSuccess func(platform, cookies, nickname string)) *Manager {
	if pythonPath == "" {
		pythonPath = "python3"
	}
	return &Manager{
		sessions:   make(map[string]*LoginSession),
		scriptPath: scriptPath,
		pythonPath: pythonPath,
		onSuccess:  onSuccess,
	}
}

// StartLogin begins a login session for the given platform.
// If a session already exists and is not in a terminal state, it returns an error.
func (m *Manager) StartLogin(platform string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Kill any existing session for this platform.
	if existing, ok := m.sessions[platform]; ok {
		existing.mu.Lock()
		if existing.cancel != nil {
			existing.cancel()
		}
		existing.mu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	session := &LoginSession{
		Platform: platform,
		State:    StateStarting,
		cancel:   cancel,
	}
	m.sessions[platform] = session

	cmd := exec.CommandContext(ctx, m.pythonPath, m.scriptPath, platform)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	cmd.Stderr = nil // let Python stderr go to parent's stderr for debugging
	session.cmd = cmd

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("failed to start login script: %w", err)
	}

	// Read stdout JSON lines in a goroutine.
	go func() {
		defer cancel()
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 4*1024*1024), 4*1024*1024) // 4MB buffer for base64 images
		for scanner.Scan() {
			line := scanner.Text()
			m.handleMessage(platform, line)
		}

		// Process exited — check if we reached success.
		_ = cmd.Wait()
		session.mu.Lock()
		if session.State != StateSuccess {
			if session.State != StateError {
				session.State = StateError
				if session.Error == "" {
					session.Error = "登录脚本意外退出"
				}
			}
		}
		session.mu.Unlock()
	}()

	return nil
}

// handleMessage parses a JSON line from the Python script.
func (m *Manager) handleMessage(platform, line string) {
	m.mu.RLock()
	session, ok := m.sessions[platform]
	m.mu.RUnlock()
	if !ok {
		return
	}

	var msg struct {
		Type     string `json:"type"`
		Image    string `json:"image"`
		Code     int    `json:"code"`
		Cookies  string `json:"cookies"`
		Nickname string `json:"nickname"`
		Message  string `json:"message"`
	}
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		log.Printf("[login/%s] invalid JSON: %s", platform, line)
		return
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	switch msg.Type {
	case "qr_ready":
		session.QRImage = msg.Image
		session.State = StateWaitingScan
		log.Printf("[login/%s] QR ready", platform)

	case "status":
		if msg.Code == 802 {
			session.State = StateScanned
			log.Printf("[login/%s] scanned", platform)
		}

	case "login_success":
		session.Cookies = msg.Cookies
		session.Nickname = msg.Nickname
		session.State = StateSuccess
		log.Printf("[login/%s] success (nickname: %s)", platform, msg.Nickname)

		// Call the onSuccess callback (sets cookie + persists).
		if m.onSuccess != nil {
			m.onSuccess(platform, msg.Cookies, msg.Nickname)
		}

	case "expired":
		session.State = StateExpired
		log.Printf("[login/%s] QR expired, script will refresh", platform)

	case "error":
		session.Error = msg.Message
		session.State = StateError
		log.Printf("[login/%s] error: %s", platform, msg.Message)
	}
}

// GetStatus returns the current state of a login session.
func (m *Manager) GetStatus(platform string) (state SessionState, qrImage, nickname, errMsg string) {
	m.mu.RLock()
	session, ok := m.sessions[platform]
	m.mu.RUnlock()

	if !ok {
		return StateError, "", "", "没有进行中的登录会话"
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	return session.State, session.QRImage, session.Nickname, session.Error
}

// StopLogin terminates a login session.
func (m *Manager) StopLogin(platform string) {
	m.mu.Lock()
	session, ok := m.sessions[platform]
	m.mu.Unlock()

	if !ok {
		return
	}

	session.mu.Lock()
	if session.cancel != nil {
		session.cancel()
	}
	session.mu.Unlock()
}
