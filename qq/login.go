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

// GetLoginStatus checks whether the current cookie contains valid QQ login keys.
// Accepts both QQ Music tokens (qqmusic_key, qm_keyst) and OAuth tokens (p_skey).
func GetLoginStatus() (bool, string) {
	cookie := GetCookie()
	if cookie == "" {
		return false, ""
	}
	// Accept QQ Music tokens or OAuth tokens (p_skey from QR login).
	validKeys := []string{"qqmusic_key", "qm_keyst", "p_skey"}
	loggedIn := false
	for _, key := range validKeys {
		if strings.Contains(cookie, key+"=") {
			loggedIn = true
			break
		}
	}
	if !loggedIn {
		return false, ""
	}
	// Try to extract uin/p_uin as nickname hint.
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "p_uin=") {
			v := strings.TrimPrefix(part, "p_uin=")
			if strings.HasPrefix(v, "o") {
				v = v[1:]
			}
			return true, v
		}
		if strings.HasPrefix(part, "uin=") {
			return true, strings.TrimPrefix(part, "uin=")
		}
	}
	return true, "QQ用户"
}

// Logout clears the cookie and removes the persisted file.
func Logout() {
	SetCookie("")
	removeCookieFromDisk()
}
