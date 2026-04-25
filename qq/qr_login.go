package qq

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
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

// followRedirect follows the OAuth redirect chain to capture all cookies,
// then attempts the QQ Music authorize step to get qqmusic_key.
func (p *QQQRProvider) followRedirect(ctx context.Context, redirectURL, qrsig string) (string, error) {
	if redirectURL == "" {
		return "", fmt.Errorf("empty redirect URL")
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Timeout: 15 * time.Second,
		Jar:     jar,
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", redirectURL, nil)
	req.Header.Set("User-Agent", qqUA)
	req.Header.Set("Cookie", "qrsig="+qrsig)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("redirect request: %w", err)
	}
	resp.Body.Close()

	slog.Info("qq.follow_redirect", "final_url", resp.Request.URL.String(), "status", resp.StatusCode)

	// Step 2: Try QQ Music authorize to get qqmusic_key.
	// This exchanges OAuth tokens (p_skey) for QQ Music tokens.
	authURL := "https://graph.qq.com/oauth2.0/authorize?response_type=code&client_id=100497308&redirect_uri=https%3A%2F%2Fy.qq.com%2Fcgi-bin%2Fmusicu.fcg%3Fcallback&state=&display=&scope="
	authReq, _ := http.NewRequestWithContext(ctx, "GET", authURL, nil)
	authReq.Header.Set("User-Agent", qqUA)
	authReq.Header.Set("Referer", "https://graph.qq.com/")
	authResp, authErr := client.Do(authReq)
	if authErr != nil {
		slog.Warn("qq.authorize_step_failed", "error", authErr)
	} else {
		authResp.Body.Close()
		slog.Info("qq.authorize_step", "final_url", authResp.Request.URL.String(), "status", authResp.StatusCode)
	}

	// Collect all cookies from the jar across all domains.
	var parts []string
	for _, domain := range []string{
		"https://y.qq.com",
		"https://qq.com",
		"https://graph.qq.com",
		"https://ptlogin2.qq.com",
		"https://ssl.ptlogin2.qq.com",
	} {
		if u, err := url.Parse(domain); err == nil {
			for _, c := range jar.Cookies(u) {
				parts = append(parts, c.Name+"="+c.Value)
			}
		}
	}

	// Also include cookies from the response headers directly.
	for _, c := range resp.Cookies() {
		parts = append(parts, c.Name+"="+c.Value)
	}

	// Deduplicate.
	seen := make(map[string]bool)
	var unique []string
	for _, p := range parts {
		name := strings.SplitN(p, "=", 2)[0]
		if !seen[name] {
			seen[name] = true
			unique = append(unique, p)
		}
	}

	slog.Info("qq.cookies_collected", "count", len(unique), "has_qqmusic_key", seen["qqmusic_key"], "has_p_skey", seen["p_skey"])

	return strings.Join(unique, "; "), nil
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
