package qq

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	loginMu   sync.RWMutex
	configDir = "config"
)

// SetConfigDir sets the directory for cookie persistence.
func SetConfigDir(dir string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	configDir = dir
}

// SetCookie updates the global cookie and replaces the default QQ instance.
func SetCookie(cookie string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	defaultQQ = New(cookie)
}

// GetCookie returns the current cookie string.
func GetCookie() string {
	loginMu.RLock()
	defer loginMu.RUnlock()
	return defaultQQ.cookie
}

// getDefault returns the current default QQ instance (thread-safe).
func getDefault() *QQ {
	loginMu.RLock()
	defer loginMu.RUnlock()
	return defaultQQ
}

// --- Cookie persistence ---

type cookieFile struct {
	Cookie   string `json:"cookie"`
	Nickname string `json:"nickname"`
}

func cookiePath() string {
	loginMu.RLock()
	dir := configDir
	loginMu.RUnlock()
	return filepath.Join(dir, "qq_cookie.json")
}

// LoadCookieFromDisk reads the persisted cookie file and sets the global cookie.
func LoadCookieFromDisk() {
	data, err := os.ReadFile(cookiePath())
	if err != nil {
		return
	}
	var cf cookieFile
	if json.Unmarshal(data, &cf) == nil && cf.Cookie != "" {
		SetCookie(cf.Cookie)
	}
}

// SaveCookieToDisk persists the cookie and nickname to disk.
func SaveCookieToDisk(cookie, nickname string) error {
	p := cookiePath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(cookieFile{Cookie: cookie, Nickname: nickname}, "", "  ")
	return os.WriteFile(p, data, 0600)
}

func removeCookieFromDisk() {
	os.Remove(cookiePath())
}

// --- Login status ---

// GetLoginStatus checks whether the current cookie contains the required QQ music keys.
// Returns loggedIn status and a placeholder nickname.
func GetLoginStatus() (bool, string) {
	cookie := GetCookie()
	if cookie == "" {
		return false, ""
	}
	// QQ music requires qqmusic_key or qm_keyst to be considered logged in.
	if strings.Contains(cookie, "qqmusic_key") || strings.Contains(cookie, "qm_keyst") {
		// Try to extract uin as nickname hint.
		for _, part := range strings.Split(cookie, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "uin=") {
				return true, strings.TrimPrefix(part, "uin=")
			}
		}
		return true, "QQ用户"
	}
	return false, ""
}

// Logout clears the cookie and removes the persisted file.
func Logout() {
	SetCookie("")
	removeCookieFromDisk()
}
