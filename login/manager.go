package login

import (
	"context"
	"log/slog"
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

// PollResult is the outcome of a single QR poll cycle.
type PollResult struct {
	State    SessionState
	Cookies  string
	Nickname string
}

// QRProvider handles QR code generation and polling for a specific platform.
type QRProvider interface {
	// StartQR generates a QR login session. Returns base64 PNG and an opaque session key.
	StartQR(ctx context.Context) (qrImage string, sessionKey string, err error)
	// PollQR checks the current login status for the given session key.
	PollQR(ctx context.Context, sessionKey string) (PollResult, error)
}

// LoginSession tracks a single platform's login process.
type LoginSession struct {
	mu       sync.Mutex
	Platform string
	State    SessionState
	QRImage  string // base64 PNG
	Nickname string
	Cookies  string
	Error    string
	cancel   context.CancelFunc
}

// Manager manages login sessions for multiple platforms.
type Manager struct {
	mu        sync.RWMutex
	sessions  map[string]*LoginSession
	providers map[string]QRProvider
	onSuccess func(platform, cookies, nickname string)
}

// NewManager creates a new login manager.
func NewManager(providers map[string]QRProvider, onSuccess func(platform, cookies, nickname string)) *Manager {
	if providers == nil {
		providers = make(map[string]QRProvider)
	}
	return &Manager{
		sessions:  make(map[string]*LoginSession),
		providers: providers,
		onSuccess: onSuccess,
	}
}

// StartLogin begins a QR login session for the given platform.
func (m *Manager) StartLogin(platform string) error {
	provider, ok := m.providers[platform]
	if !ok {
		return &ErrNoProvider{Platform: platform}
	}

	m.mu.Lock()
	// Kill any existing session.
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
	m.mu.Unlock()

	go m.runQRLogin(ctx, cancel, platform, session, provider)

	return nil
}

// runQRLogin drives the QR login lifecycle in a goroutine.
func (m *Manager) runQRLogin(ctx context.Context, cancel context.CancelFunc, platform string, session *LoginSession, provider QRProvider) {
	defer cancel()

	// Generate QR code.
	qrImage, sessionKey, err := provider.StartQR(ctx)
	if err != nil {
		session.mu.Lock()
		session.State = StateError
		session.Error = err.Error()
		session.mu.Unlock()
		slog.Warn("login.qr_generate_failed", "platform", platform, "error", err)
		return
	}

	session.mu.Lock()
	session.QRImage = qrImage
	session.State = StateWaitingScan
	session.mu.Unlock()
	slog.Info("login.qr_ready", "platform", platform)

	// Poll loop.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			session.mu.Lock()
			if session.State != StateSuccess {
				session.State = StateExpired
			}
			session.mu.Unlock()
			return
		case <-ticker.C:
			result, err := provider.PollQR(ctx, sessionKey)
			if err != nil {
				slog.Warn("login.poll_error", "platform", platform, "error", err)
				continue
			}

			session.mu.Lock()
			switch result.State {
			case StateScanned:
				if session.State != StateScanned {
					session.State = StateScanned
					slog.Info("login.scanned", "platform", platform)
				}
			case StateSuccess:
				session.Cookies = result.Cookies
				session.Nickname = result.Nickname
				session.State = StateSuccess
				session.mu.Unlock()
				slog.Info("login.success", "platform", platform, "nickname", result.Nickname)
				if m.onSuccess != nil {
					m.onSuccess(platform, result.Cookies, result.Nickname)
				}
				return
			case StateExpired:
				session.State = StateExpired
				session.mu.Unlock()
				slog.Info("login.qr_expired", "platform", platform)
				return
			case StateError:
				session.State = StateError
				session.Error = "登录失败"
				session.mu.Unlock()
				return
			}
			session.mu.Unlock()
		}
	}
}

// SetCookie sets cookies directly (manual cookie input).
func (m *Manager) SetCookie(platform, cookies, nickname string) {
	m.mu.Lock()
	// Create or replace session with success state.
	session := &LoginSession{
		Platform: platform,
		State:    StateSuccess,
		Cookies:  cookies,
		Nickname: nickname,
	}
	m.sessions[platform] = session
	m.mu.Unlock()

	slog.Info("login.manual_cookie", "platform", platform, "nickname", nickname)
	if m.onSuccess != nil {
		m.onSuccess(platform, cookies, nickname)
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

// ErrNoProvider is returned when no QR provider is registered for a platform.
type ErrNoProvider struct {
	Platform string
}

func (e *ErrNoProvider) Error() string {
	return "no QR login provider for platform: " + e.Platform
}
