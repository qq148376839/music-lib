package netease

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/guohuiyuan/music-lib/login"
	"github.com/guohuiyuan/music-lib/utils"
	qrcode "github.com/skip2/go-qrcode"
)

const (
	qrUnikeyAPI = "https://music.163.com/weapi/login/qrcode/unikey"
	qrPollAPI   = "https://music.163.com/weapi/login/qrcode/client/login"
	qrLoginURL  = "https://music.163.com/login?codekey=%s"
)

// QRProvider implements login.QRProvider for Netease.
// Note: QR generation works but polling is blocked by Netease anti-bot (error 8821).
// Users should use manual cookie paste instead.
type QRProvider struct{}

// NewQRProvider creates a Netease QR login provider.
func NewQRProvider() *QRProvider {
	return &QRProvider{}
}

// StartQR generates a QR code for Netease login.
func (p *QRProvider) StartQR(ctx context.Context) (string, string, error) {
	reqData, _ := json.Marshal(map[string]interface{}{"type": 1})
	params, encSecKey := EncryptWeApi(string(reqData))

	form := url.Values{}
	form.Set("params", params)
	form.Set("encSecKey", encSecKey)

	body, err := utils.Post(qrUnikeyAPI, strings.NewReader(form.Encode()),
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Content-Type", "application/x-www-form-urlencoded"),
	)
	if err != nil {
		return "", "", fmt.Errorf("request unikey: %w", err)
	}

	var resp struct {
		Code    int    `json:"code"`
		Unikey  string `json:"unikey"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", fmt.Errorf("parse unikey response: %w", err)
	}
	if resp.Code != 200 || resp.Unikey == "" {
		return "", "", fmt.Errorf("unikey failed: code=%d msg=%s", resp.Code, resp.Message)
	}

	loginURL := fmt.Sprintf(qrLoginURL, resp.Unikey)
	png, err := qrcode.Encode(loginURL, qrcode.Medium, 256)
	if err != nil {
		return "", "", fmt.Errorf("generate qrcode: %w", err)
	}

	b64 := base64.StdEncoding.EncodeToString(png)
	return b64, resp.Unikey, nil
}

// PollQR checks the QR scan status.
func (p *QRProvider) PollQR(ctx context.Context, unikey string) (login.PollResult, error) {
	reqData, _ := json.Marshal(map[string]interface{}{
		"key":  unikey,
		"type": 1,
	})
	params, encSecKey := EncryptWeApi(string(reqData))

	form := url.Values{}
	form.Set("params", params)
	form.Set("encSecKey", encSecKey)

	resp, err := utils.PostRaw(qrPollAPI, strings.NewReader(form.Encode()),
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Content-Type", "application/x-www-form-urlencoded"),
	)
	if err != nil {
		return login.PollResult{}, fmt.Errorf("poll qr: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Code     int    `json:"code"`
		Message  string `json:"message"`
		Nickname string `json:"nickname"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return login.PollResult{}, fmt.Errorf("decode poll response: %w", err)
	}

	switch result.Code {
	case 800:
		return login.PollResult{State: login.StateExpired}, nil
	case 801:
		return login.PollResult{State: login.StateWaitingScan}, nil
	case 802:
		return login.PollResult{State: login.StateScanned}, nil
	case 803:
		cookies := extractCookies(resp.Cookies(), "163.com")
		nickname := result.Nickname
		if nickname == "" {
			nickname = getNicknameFromCookie(cookies)
		}
		return login.PollResult{
			State:    login.StateSuccess,
			Cookies:  cookies,
			Nickname: nickname,
		}, nil
	default:
		return login.PollResult{State: login.StateError}, fmt.Errorf("unexpected code: %d %s", result.Code, result.Message)
	}
}

func extractCookies(cookies []*http.Cookie, domainSuffix string) string {
	var parts []string
	for _, c := range cookies {
		if strings.Contains(c.Domain, domainSuffix) || c.Domain == "" {
			parts = append(parts, c.Name+"="+c.Value)
		}
	}
	return strings.Join(parts, "; ")
}

func getNicknameFromCookie(cookie string) string {
	if cookie == "" {
		return ""
	}
	reqData, _ := json.Marshal(map[string]interface{}{})
	params, encSecKey := EncryptWeApi(string(reqData))
	form := url.Values{}
	form.Set("params", params)
	form.Set("encSecKey", encSecKey)

	body, err := utils.Post(AccountAPI, strings.NewReader(form.Encode()),
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Content-Type", "application/x-www-form-urlencoded"),
		utils.WithHeader("Cookie", cookie),
	)
	if err != nil {
		return ""
	}
	var r struct {
		Code    int `json:"code"`
		Profile struct {
			Nickname string `json:"nickname"`
		} `json:"profile"`
	}
	if json.Unmarshal(body, &r) == nil && r.Profile.Nickname != "" {
		return r.Profile.Nickname
	}
	return ""
}
