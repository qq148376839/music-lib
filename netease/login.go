package netease

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/guohuiyuan/music-lib/utils"
)

const (
	QRKeyAPI    = "https://music.163.com/weapi/login/qrcode/unikey"
	QRCheckAPI  = "https://music.163.com/weapi/login/qrcode/client/login"
	AccountAPI  = "https://music.163.com/weapi/w/nuser/account/get"
)

var (
	loginMu    sync.RWMutex
	configDir  = "config"
)

// SetConfigDir sets the directory for cookie persistence.
func SetConfigDir(dir string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	configDir = dir
}

// SetCookie updates the global cookie and replaces the default Netease instance.
func SetCookie(cookie string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	defaultNetease = New(cookie)
}

// GetCookie returns the current cookie string.
func GetCookie() string {
	loginMu.RLock()
	defer loginMu.RUnlock()
	return defaultNetease.cookie
}

// getDefault returns the current default Netease instance (thread-safe).
func getDefault() *Netease {
	loginMu.RLock()
	defer loginMu.RUnlock()
	return defaultNetease
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
	return filepath.Join(dir, "netease_cookie.json")
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

func saveCookieToDisk(cookie, nickname string) error {
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

// --- QR login flow ---

// GenerateQRKey requests a unikey for QR code login.
func GenerateQRKey() (string, error) {
	reqData := map[string]interface{}{
		"type": 3,
	}
	reqJSON, _ := json.Marshal(reqData)
	params, encSecKey := EncryptWeApi(string(reqJSON))
	form := url.Values{}
	form.Set("params", params)
	form.Set("encSecKey", encSecKey)

	headers := []utils.RequestOption{
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Content-Type", "application/x-www-form-urlencoded"),
	}

	body, err := utils.Post(QRKeyAPI, strings.NewReader(form.Encode()), headers...)
	if err != nil {
		return "", fmt.Errorf("generate qr key failed: %w", err)
	}

	var resp struct {
		Code   int    `json:"code"`
		Unikey string `json:"unikey"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse qr key response failed: %w", err)
	}
	if resp.Code != 200 {
		return "", fmt.Errorf("qr key api error code: %d", resp.Code)
	}
	return resp.Unikey, nil
}

// QRLoginStatus checks the QR scan status.
// Returns: code (801=waiting, 802=scanned, 803=success, 800=expired), cookie, nickname, error
func QRLoginStatus(unikey string) (int, string, string, error) {
	reqData := map[string]interface{}{
		"key":  unikey,
		"type": 3,
	}
	reqJSON, _ := json.Marshal(reqData)
	params, encSecKey := EncryptWeApi(string(reqJSON))
	form := url.Values{}
	form.Set("params", params)
	form.Set("encSecKey", encSecKey)

	headers := []utils.RequestOption{
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Content-Type", "application/x-www-form-urlencoded"),
	}

	resp, err := utils.PostRaw(QRCheckAPI, strings.NewReader(form.Encode()), headers...)
	if err != nil {
		return 0, "", "", fmt.Errorf("qr check failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", "", fmt.Errorf("read qr check response failed: %w", err)
	}

	var result struct {
		Code     int    `json:"code"`
		Nickname string `json:"nickname"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return 0, "", "", fmt.Errorf("parse qr check response failed: %w", err)
	}

	if result.Code == 803 {
		// Login success — extract cookies from Set-Cookie headers
		var cookieParts []string
		for _, c := range resp.Cookies() {
			cookieParts = append(cookieParts, c.Name+"="+c.Value)
		}
		cookieStr := strings.Join(cookieParts, "; ")
		SetCookie(cookieStr)
		saveCookieToDisk(cookieStr, result.Nickname)
		return result.Code, cookieStr, result.Nickname, nil
	}

	return result.Code, "", result.Nickname, nil
}

// GetLoginStatus checks whether the current cookie is valid.
// Returns loggedIn status and nickname.
func GetLoginStatus() (bool, string) {
	cookie := GetCookie()
	if cookie == "" {
		return false, ""
	}

	reqData := map[string]interface{}{}
	reqJSON, _ := json.Marshal(reqData)
	params, encSecKey := EncryptWeApi(string(reqJSON))
	form := url.Values{}
	form.Set("params", params)
	form.Set("encSecKey", encSecKey)

	headers := []utils.RequestOption{
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Content-Type", "application/x-www-form-urlencoded"),
		utils.WithHeader("Cookie", cookie),
	}

	body, err := utils.Post(AccountAPI, strings.NewReader(form.Encode()), headers...)
	if err != nil {
		return false, ""
	}

	var resp struct {
		Code    int `json:"code"`
		Profile struct {
			Nickname string `json:"nickname"`
		} `json:"profile"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, ""
	}
	if resp.Code == 200 && resp.Profile.Nickname != "" {
		return true, resp.Profile.Nickname
	}
	return false, ""
}

// Logout clears the cookie and removes the persisted file.
func Logout() {
	SetCookie("")
	removeCookieFromDisk()
}
