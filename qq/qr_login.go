package qq

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/guohuiyuan/music-lib/login"
)

const (
	xloginURL    = "https://xui.ptlogin2.qq.com/cgi-bin/xlogin?appid=716027609&daid=383&style=33&login_text=%E7%99%BB%E5%BD%95&hide_title_bar=1&hide_border=0&target=self&s_url=https%3A%2F%2Fgraph.qq.com%2Foauth2.0%2Flogin_jump&pt_3rd_aid=100497308&pt_feedback_link=https%3A%2F%2Fsupport.qq.com%2Fproducts%2F77942&theme=2&verify_theme="
	ptqrshowFmt  = "https://ssl.ptlogin2.qq.com/ptqrshow?appid=716027609&e=2&l=M&s=3&d=72&v=4&t=0.%d&daid=383&pt_3rd_aid=100497308"
	ptqrloginFmt = "https://ssl.ptlogin2.qq.com/ptqrlogin?u1=https%%3A%%2F%%2Fgraph.qq.com%%2Foauth2.0%%2Flogin_jump&ptqrtoken=%d&ptredirect=0&h=1&t=1&g=1&from_ui=1&ptlang=2052&action=0-0-%d&js_ver=24102114&js_type=1&pt_uistyle=40&aid=716027609&daid=383&pt_3rd_aid=100497308&has_resolve=0"

	qqUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

var (
	ptuiCBRe = regexp.MustCompile(`ptuiCB\('(\d+)',\s*'(\d+)',\s*'([^']*)',\s*'(\d+)',\s*'([^']*)',\s*'([^']*)'(?:,\s*'([^']*)')?\)`)
)

// QQQRProvider implements login.QRProvider for QQ Music.
type QQQRProvider struct {
	mu       sync.Mutex
	sessions map[string]*qqSession
}

type qqSession struct {
	qrsig      string
	ptLoginSig string
}

// NewQRProvider creates a QQ Music QR login provider.
func NewQRProvider() *QQQRProvider {
	return &QQQRProvider{
		sessions: make(map[string]*qqSession),
	}
}

// hash33 computes the ptqrtoken from qrsig cookie.
func hash33(s string) int {
	e := 0
	for _, c := range s {
		e += (e << 5) + int(c)
	}
	return e & 0x7FFFFFFF
}

// StartQR generates a QR code for QQ Music login.
func (p *QQQRProvider) StartQR(ctx context.Context) (string, string, error) {
	// Step 1: Get pt_login_sig from xlogin page.
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects.
		},
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", xloginURL, nil)
	req.Header.Set("User-Agent", qqUA)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("xlogin request: %w", err)
	}
	resp.Body.Close()

	ptLoginSig := getCookieValue(resp.Cookies(), "pt_login_sig")

	// Step 2: Get QR code image + qrsig from ptqrshow.
	showURL := fmt.Sprintf(ptqrshowFmt, time.Now().UnixMilli())
	req, _ = http.NewRequestWithContext(ctx, "GET", showURL, nil)
	req.Header.Set("User-Agent", qqUA)
	req.Header.Set("Referer", xloginURL)

	resp, err = client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("ptqrshow request: %w", err)
	}
	defer resp.Body.Close()

	imgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read qr image: %w", err)
	}

	qrsig := getCookieValue(resp.Cookies(), "qrsig")
	if qrsig == "" {
		return "", "", fmt.Errorf("no qrsig cookie in ptqrshow response")
	}

	b64 := base64.StdEncoding.EncodeToString(imgBytes)
	slog.Info("qq.qr_image_generated", "image_bytes", len(imgBytes), "qrsig_len", len(qrsig))

	// Store session.
	p.mu.Lock()
	p.sessions[qrsig] = &qqSession{
		qrsig:      qrsig,
		ptLoginSig: ptLoginSig,
	}
	p.mu.Unlock()

	return b64, qrsig, nil
}

// PollQR checks the QR scan status.
func (p *QQQRProvider) PollQR(ctx context.Context, sessionKey string) (login.PollResult, error) {
	qrsig := sessionKey
	ptqrtoken := hash33(qrsig)

	// Retrieve pt_login_sig from session.
	p.mu.Lock()
	sess, ok := p.sessions[qrsig]
	p.mu.Unlock()
	if !ok {
		return login.PollResult{State: login.StateError}, fmt.Errorf("qq session not found for qrsig")
	}

	loginURL := fmt.Sprintf(ptqrloginFmt, ptqrtoken, time.Now().Unix())

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", loginURL, nil)
	req.Header.Set("User-Agent", qqUA)
	req.Header.Set("Referer", "https://xui.ptlogin2.qq.com/")
	req.Header.Set("Cookie", "qrsig="+qrsig+"; pt_login_sig="+sess.ptLoginSig)

	resp, err := client.Do(req)
	if err != nil {
		return login.PollResult{}, fmt.Errorf("ptqrlogin request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	text := string(body)

	matches := ptuiCBRe.FindStringSubmatch(text)
	if len(matches) < 7 {
		slog.Warn("qq.poll_unexpected_response", "body_len", len(text), "body_head", text[:min(len(text), 200)], "body_tail", text[max(0, len(text)-100):])
		return login.PollResult{}, fmt.Errorf("unexpected ptqrlogin response: %s", text[:min(len(text), 200)])
	}

	code := matches[1]
	redirectURL := matches[3]
	nickname := matches[6]

	slog.Info("qq.poll_result", "code", code, "nickname", nickname, "has_redirect", redirectURL != "")

	switch code {
	case "66": // Not scanned
		return login.PollResult{State: login.StateWaitingScan}, nil
	case "67": // Scanned, waiting confirmation
		return login.PollResult{State: login.StateScanned}, nil
	case "65": // QR expired
		return login.PollResult{State: login.StateExpired}, nil
	case "0": // Success
		cookies, err := p.followRedirect(ctx, redirectURL, qrsig)
		if err != nil {
			slog.Error("qq.follow_redirect_failed", "error", err)
			return login.PollResult{State: login.StateError}, fmt.Errorf("follow redirect: %w", err)
		}
		if nickname == "" {
			nickname = extractUin(cookies)
		}

		// Cleanup session.
		p.mu.Lock()
		delete(p.sessions, qrsig)
		p.mu.Unlock()

		slog.Info("qq.login_success", "nickname", nickname, "cookie_len", len(cookies))
		return login.PollResult{
			State:    login.StateSuccess,
			Cookies:  cookies,
			Nickname: nickname,
		}, nil
	default:
		slog.Warn("qq.poll_unknown_code", "code", code, "body", text[:min(len(text), 200)])
		return login.PollResult{State: login.StateError}, fmt.Errorf("ptqrlogin code: %s", code)
	}
}

// followRedirect completes the QQ Music login in 4 steps:
//  1. Follow ptqrlogin redirect → check_sig → get p_skey cookies
//  2. POST OAuth2 authorize → get authorization code
//  3. POST musicu.fcg QQLogin → exchange code for musickey
//  4. Build cookie string with musicid + musickey (= qqmusic_key)
func (p *QQQRProvider) followRedirect(ctx context.Context, redirectURL, qrsig string) (string, error) {
	if redirectURL == "" {
		return "", fmt.Errorf("empty redirect URL")
	}

	jar, _ := cookiejar.New(nil)

	// Don't follow redirects automatically — we need to inspect intermediate responses.
	noRedirectClient := &http.Client{
		Timeout: 15 * time.Second,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	// Auto-follow client for check_sig.
	followClient := &http.Client{
		Timeout: 15 * time.Second,
		Jar:     jar,
	}

	// --- Step 1: Follow redirect to check_sig, collect p_skey cookies ---
	req, _ := http.NewRequestWithContext(ctx, "GET", redirectURL, nil)
	req.Header.Set("User-Agent", qqUA)
	req.Header.Set("Referer", "https://xui.ptlogin2.qq.com/")
	req.Header.Set("Cookie", "qrsig="+qrsig)

	resp, err := followClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("check_sig request: %w", err)
	}
	resp.Body.Close()

	slog.Info("qq.step1_check_sig", "final_url", resp.Request.URL.String(), "status", resp.StatusCode)

	// Extract p_skey for g_tk computation.
	var pSkey string
	graphURL, _ := url.Parse("https://graph.qq.com")
	for _, c := range jar.Cookies(graphURL) {
		if c.Name == "p_skey" {
			pSkey = c.Value
		}
	}
	if pSkey == "" {
		return "", fmt.Errorf("p_skey not found after check_sig")
	}

	gtk := computeGTK(pSkey)
	slog.Info("qq.step1_done", "p_skey_len", len(pSkey), "g_tk", gtk)

	// --- Step 2: POST OAuth2 authorize to get authorization code ---
	authForm := url.Values{
		"response_type": {"code"},
		"client_id":     {"100497308"},
		"redirect_uri":  {"https://y.qq.com/portal/wx_redirect.html?login_type=1&surl=https://y.qq.com/"},
		"scope":         {"get_user_info,get_app_friends"},
		"state":         {"state"},
		"switch":        {""},
		"from_ptlogin":  {"1"},
		"src":           {"1"},
		"update_auth":   {"1"},
		"openapi":       {"1010_1030"},
		"g_tk":          {strconv.Itoa(gtk)},
		"auth_time":     {strconv.FormatInt(time.Now().UnixMilli(), 10)},
		"ui":            {fmt.Sprintf("%d", time.Now().UnixNano())},
	}

	authReq, _ := http.NewRequestWithContext(ctx, "POST", "https://graph.qq.com/oauth2.0/authorize", strings.NewReader(authForm.Encode()))
	authReq.Header.Set("User-Agent", qqUA)
	authReq.Header.Set("Referer", "https://graph.qq.com/")
	authReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	authResp, err := noRedirectClient.Do(authReq)
	if err != nil {
		return "", fmt.Errorf("oauth authorize: %w", err)
	}
	authResp.Body.Close()

	location := authResp.Header.Get("Location")
	slog.Info("qq.step2_authorize", "status", authResp.StatusCode, "location_len", len(location))

	if location == "" {
		return "", fmt.Errorf("no redirect from authorize (status %d)", authResp.StatusCode)
	}

	// Extract code from redirect URL.
	locURL, err := url.Parse(location)
	if err != nil {
		return "", fmt.Errorf("parse authorize redirect: %w", err)
	}
	code := locURL.Query().Get("code")
	if code == "" {
		return "", fmt.Errorf("no code in authorize redirect: %s", location[:min(len(location), 200)])
	}
	slog.Info("qq.step2_done", "code_len", len(code))

	// --- Step 3: Exchange code for musickey via musicu.fcg ---
	musicReqBody := map[string]interface{}{
		"comm": map[string]interface{}{
			"tmeLoginType": 2,
			"format":       "json",
		},
		"req1": map[string]interface{}{
			"module": "QQConnectLogin.LoginServer",
			"method": "QQLogin",
			"param": map[string]interface{}{
				"code": code,
			},
		},
	}
	musicJSON, _ := json.Marshal(musicReqBody)

	musicReq, _ := http.NewRequestWithContext(ctx, "POST", "https://u.y.qq.com/cgi-bin/musicu.fcg", bytes.NewReader(musicJSON))
	musicReq.Header.Set("User-Agent", qqUA)
	musicReq.Header.Set("Referer", "https://y.qq.com/")
	musicReq.Header.Set("Content-Type", "application/json")

	musicResp, err := http.DefaultClient.Do(musicReq)
	if err != nil {
		return "", fmt.Errorf("musicu.fcg QQLogin: %w", err)
	}
	defer musicResp.Body.Close()

	musicBody, _ := io.ReadAll(musicResp.Body)

	var musicResult struct {
		Code int `json:"code"`
		Req1 struct {
			Code int `json:"code"`
			Data struct {
				MusicID      string `json:"musicid"`
				MusicKey     string `json:"musickey"`
				OpenID       string `json:"openid"`
				RefreshKey   string `json:"refresh_key"`
				RefreshToken string `json:"refresh_token"`
				LoginType    int    `json:"login_type"`
			} `json:"data"`
		} `json:"req1"`
	}

	if err := json.Unmarshal(musicBody, &musicResult); err != nil {
		return "", fmt.Errorf("parse QQLogin response: %w (body: %s)", err, string(musicBody[:min(len(musicBody), 300)]))
	}

	if musicResult.Req1.Code != 0 {
		return "", fmt.Errorf("QQLogin failed: code=%d (body: %s)", musicResult.Req1.Code, string(musicBody[:min(len(musicBody), 500)]))
	}

	musicID := musicResult.Req1.Data.MusicID
	musicKey := musicResult.Req1.Data.MusicKey

	if musicKey == "" {
		return "", fmt.Errorf("empty musickey from QQLogin response")
	}

	slog.Info("qq.step3_done", "musicid", musicID, "musickey_len", len(musicKey))

	// --- Step 4: Build final cookie string ---
	cookieParts := []string{
		"uin=" + musicID,
		"qqmusic_key=" + musicKey,
		"qm_keyst=" + musicKey,
	}

	// Include p_skey and other OAuth cookies for compatibility.
	for _, domain := range []string{"https://graph.qq.com", "https://qq.com"} {
		if u, parseErr := url.Parse(domain); parseErr == nil {
			for _, c := range jar.Cookies(u) {
				if c.Name == "p_skey" || c.Name == "p_uin" || c.Name == "pt4_token" {
					cookieParts = append(cookieParts, c.Name+"="+c.Value)
				}
			}
		}
	}

	// Deduplicate by name.
	seen := make(map[string]bool)
	var unique []string
	for _, part := range cookieParts {
		name := strings.SplitN(part, "=", 2)[0]
		if !seen[name] {
			seen[name] = true
			unique = append(unique, part)
		}
	}

	cookies := strings.Join(unique, "; ")
	slog.Info("qq.login_complete", "musicid", musicID, "cookie_keys", len(unique))

	return cookies, nil
}

func getCookieValue(cookies []*http.Cookie, name string) string {
	for _, c := range cookies {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func extractUin(cookies string) string {
	for _, part := range strings.Split(cookies, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "uin=") {
			v := strings.TrimPrefix(part, "uin=")
			if strings.HasPrefix(v, "o") {
				v = v[1:]
			}
			return v
		}
	}
	return "QQ用户"
}
