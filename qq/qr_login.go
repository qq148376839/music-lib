package qq

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"nhooyr.io/websocket"

	"github.com/guohuiyuan/music-lib/login"
)

const (
	musicuURL = "https://u.y.qq.com/cgi-bin/musicu.fcg"
	mqttWSURL = "wss://mu.y.qq.com/ws/handshake"
	qqUA      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// QQQRProvider implements login.QRProvider for QQ Music native QR login.
// Uses MQTT over WebSocket to receive scan events from mu.y.qq.com.
type QQQRProvider struct {
	mu       sync.Mutex
	sessions map[string]*qqMusicSession
}

type qqMusicSession struct {
	qrcodeID string
	cancel   context.CancelFunc

	mu      sync.Mutex
	state   string            // waiting, scanned, cookies, timeout, canceled, error
	cookies map[string]string // populated when state == "cookies"
}

// NewQRProvider creates a QQ Music QR login provider.
func NewQRProvider() *QQQRProvider {
	return &QQQRProvider{
		sessions: make(map[string]*qqMusicSession),
	}
}

// StartQR generates a QR code via QQ Music's native CreateQRCode API.
func (p *QQQRProvider) StartQR(ctx context.Context) (string, string, error) {
	reqBody := map[string]interface{}{
		"comm": map[string]interface{}{"ct": 11, "cv": 14090008},
		"req_0": map[string]interface{}{
			"module": "music.login.LoginServer",
			"method": "CreateQRCode",
			"param":  map[string]interface{}{"tmeAppID": "qqmusic", "ct": 11, "cv": 14090008},
		},
	}

	bodyJSON, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", musicuURL, bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", qqUA)
	req.Header.Set("Referer", "https://y.qq.com/")

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", "", fmt.Errorf("CreateQRCode: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Code int `json:"code"`
		Req0 struct {
			Code int `json:"code"`
			Data struct {
				QRCode   string `json:"qrcode"`
				QRCodeID string `json:"qrcodeID"`
			} `json:"data"`
		} `json:"req_0"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", "", fmt.Errorf("parse CreateQRCode: %w", err)
	}
	if result.Req0.Code != 0 {
		return "", "", fmt.Errorf("CreateQRCode code=%d", result.Req0.Code)
	}

	qrImage := result.Req0.Data.QRCode
	qrcodeID := result.Req0.Data.QRCodeID
	if qrcodeID == "" {
		return "", "", fmt.Errorf("empty qrcodeID")
	}

	// Strip data URI prefix — frontend expects raw base64.
	if i := strings.Index(qrImage, ","); i >= 0 {
		qrImage = qrImage[i+1:]
	}

	// Start MQTT listener for scan events.
	mqttCtx, mqttCancel := context.WithCancel(context.Background())
	sess := &qqMusicSession{qrcodeID: qrcodeID, cancel: mqttCancel, state: "waiting"}

	p.mu.Lock()
	p.sessions[qrcodeID] = sess
	p.mu.Unlock()

	go p.listenMQTT(mqttCtx, sess)

	slog.Info("qq.qr_created", "qrcodeID_len", len(qrcodeID), "image_len", len(qrImage))
	return qrImage, qrcodeID, nil
}

// listenMQTT connects to QQ Music's MQTT broker via WebSocket and listens for scan events.
func (p *QQQRProvider) listenMQTT(ctx context.Context, sess *qqMusicSession) {
	defer sess.cancel()

	wsConn, _, err := websocket.Dial(ctx, mqttWSURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin":     {"https://y.qq.com"},
			"Referer":    {"https://y.qq.com/"},
			"User-Agent": {qqUA},
		},
		Subprotocols: []string{"mqtt"},
	})
	if err != nil {
		slog.Error("qq.mqtt_dial_failed", "error", err)
		sess.mu.Lock()
		sess.state = "error"
		sess.mu.Unlock()
		return
	}
	wsConn.SetReadLimit(1 << 20)

	netConn := websocket.NetConn(ctx, wsConn, websocket.MessageBinary)

	router := paho.NewStandardRouter()
	router.RegisterHandler("management.qrcode_login/"+sess.qrcodeID, func(m *paho.Publish) {
		p.handleMQTTMessage(sess, m)
	})

	client := paho.NewClient(paho.ClientConfig{
		Conn:   netConn,
		Router: router,
		OnClientError: func(err error) {
			slog.Warn("qq.mqtt_client_error", "error", err)
		},
	})

	connAck, err := client.Connect(ctx, &paho.Connect{
		KeepAlive:  45,
		CleanStart: true,
		Properties: &paho.ConnectProperties{
			AuthMethod: "pass",
			User: paho.UserProperties{
				{Key: "tmeAppID", Value: "qqmusic"},
				{Key: "business", Value: "management"},
				{Key: "hashTag", Value: sess.qrcodeID},
				{Key: "clientTag", Value: "management.user"},
				{Key: "userID", Value: sess.qrcodeID},
			},
		},
	})
	if err != nil {
		slog.Error("qq.mqtt_connect_failed", "error", err)
		sess.mu.Lock()
		sess.state = "error"
		sess.mu.Unlock()
		return
	}
	if connAck.ReasonCode != 0 {
		slog.Error("qq.mqtt_connect_rejected", "code", connAck.ReasonCode)
		sess.mu.Lock()
		sess.state = "error"
		sess.mu.Unlock()
		return
	}

	slog.Info("qq.mqtt_connected")

	_, err = client.Subscribe(ctx, &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{{
			Topic: "management.qrcode_login/" + sess.qrcodeID,
			QoS:   0,
		}},
		Properties: &paho.SubscribeProperties{
			User: paho.UserProperties{
				{Key: "authorization", Value: "tmelogin"},
				{Key: "pubsub", Value: "unicast"},
			},
		},
	})
	if err != nil {
		slog.Error("qq.mqtt_subscribe_failed", "error", err)
		sess.mu.Lock()
		sess.state = "error"
		sess.mu.Unlock()
		return
	}

	slog.Info("qq.mqtt_subscribed")

	// Block until context is cancelled (session cleanup or timeout).
	<-ctx.Done()
	_ = client.Disconnect(&paho.Disconnect{})
}

func (p *QQQRProvider) handleMQTTMessage(sess *qqMusicSession, m *paho.Publish) {
	var msgType string
	if m.Properties != nil {
		for _, u := range m.Properties.User {
			if u.Key == "type" {
				msgType = u.Value
				break
			}
		}
	}

	slog.Info("qq.mqtt_message", "type", msgType, "payload_len", len(m.Payload))

	sess.mu.Lock()
	defer sess.mu.Unlock()

	switch msgType {
	case "scanned":
		sess.state = "scanned"
	case "canceled":
		sess.state = "canceled"
	case "timeout":
		sess.state = "timeout"
	case "loginFailed":
		sess.state = "error"
	case "cookies":
		sess.state = "cookies"
		sess.cookies = parseMQTTCookies(m.Payload)
		slog.Info("qq.mqtt_cookies_raw", "payload", string(m.Payload[:min(len(m.Payload), 500)]))
		slog.Info("qq.mqtt_cookies_parsed", "keys", fmt.Sprintf("%v", sess.cookies))
	default:
		slog.Warn("qq.mqtt_unknown_type", "type", msgType)
	}
}

func parseMQTTCookies(payload []byte) map[string]string {
	var raw struct {
		Cookies map[string]json.RawMessage `json:"cookies"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		slog.Warn("qq.parse_mqtt_cookies", "error", err)
		return nil
	}
	out := make(map[string]string, len(raw.Cookies))
	for k, v := range raw.Cookies {
		var obj struct {
			Value string `json:"value"`
		}
		if json.Unmarshal(v, &obj) == nil && obj.Value != "" {
			out[k] = obj.Value
			continue
		}
		var s string
		if json.Unmarshal(v, &s) == nil {
			out[k] = s
		}
	}
	return out
}

// PollQR checks QR scan status from MQTT state.
func (p *QQQRProvider) PollQR(ctx context.Context, sessionKey string) (login.PollResult, error) {
	p.mu.Lock()
	sess, ok := p.sessions[sessionKey]
	p.mu.Unlock()
	if !ok {
		return login.PollResult{State: login.StateError}, fmt.Errorf("qq session not found")
	}

	sess.mu.Lock()
	state := sess.state
	cookies := sess.cookies
	sess.mu.Unlock()

	switch state {
	case "waiting":
		return login.PollResult{State: login.StateWaitingScan}, nil
	case "scanned":
		return login.PollResult{State: login.StateScanned}, nil
	case "timeout":
		p.cleanup(sessionKey)
		return login.PollResult{State: login.StateExpired}, nil
	case "canceled", "error":
		p.cleanup(sessionKey)
		return login.PollResult{State: login.StateError}, fmt.Errorf("login failed")
	case "cookies":
		result, err := p.exchangeForMusicKey(ctx, sess.qrcodeID, cookies)
		p.cleanup(sessionKey)
		if err != nil {
			slog.Error("qq.exchange_failed", "error", err)
			return login.PollResult{State: login.StateError}, err
		}
		return result, nil
	default:
		return login.PollResult{State: login.StateWaitingScan}, nil
	}
}

// exchangeForMusicKey exchanges MQTT cookies for a full QQ Music credential.
func (p *QQQRProvider) exchangeForMusicKey(ctx context.Context, qrcodeID string, cookies map[string]string) (login.PollResult, error) {
	uin := cookies["qqmusic_uin"]
	key := cookies["qqmusic_key"]
	if uin == "" || key == "" {
		return login.PollResult{}, fmt.Errorf("missing qqmusic_uin/qqmusic_key in MQTT cookies")
	}

	var musicid int
	fmt.Sscanf(uin, "%d", &musicid)

	slog.Info("qq.exchange_request", "uin", uin, "musicid", musicid, "key_len", len(key), "key_prefix", key[:min(len(key), 10)], "qrcodeID_len", len(qrcodeID))

	reqBody := map[string]interface{}{
		"comm": map[string]interface{}{"tmeLoginType": 6},
		"req_0": map[string]interface{}{
			"module": "music.login.LoginServer",
			"method": "Login",
			"param":  map[string]interface{}{"musicid": musicid, "qrCodeID": qrcodeID, "token": key},
		},
	}

	bodyJSON, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", musicuURL, bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", qqUA)
	req.Header.Set("Referer", "https://y.qq.com/")

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return login.PollResult{}, fmt.Errorf("Login request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Code int `json:"code"`
		Req0 struct {
			Code int `json:"code"`
			Data struct {
				MusicID    int    `json:"musicid"`
				StrMusicID string `json:"str_musicid"`
				MusicKey   string `json:"musickey"`
			} `json:"data"`
		} `json:"req_0"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return login.PollResult{}, fmt.Errorf("parse Login: %w (body: %s)", err, string(respBody[:min(len(respBody), 300)]))
	}
	if result.Req0.Code != 0 {
		return login.PollResult{}, fmt.Errorf("Login code=%d (body: %s)", result.Req0.Code, string(respBody[:min(len(respBody), 500)]))
	}

	mid := result.Req0.Data.StrMusicID
	if mid == "" {
		mid = fmt.Sprintf("%d", result.Req0.Data.MusicID)
	}
	mkey := result.Req0.Data.MusicKey
	if mkey == "" {
		return login.PollResult{}, fmt.Errorf("empty musickey")
	}

	slog.Info("qq.login_done", "musicid", mid, "musickey_len", len(mkey))

	cookieStr := fmt.Sprintf("uin=%s; qqmusic_key=%s; qm_keyst=%s", mid, mkey, mkey)
	return login.PollResult{
		State:    login.StateSuccess,
		Cookies:  cookieStr,
		Nickname: mid,
	}, nil
}

func (p *QQQRProvider) cleanup(qrcodeID string) {
	p.mu.Lock()
	if sess, ok := p.sessions[qrcodeID]; ok {
		sess.cancel()
		delete(p.sessions, qrcodeID)
	}
	p.mu.Unlock()
}
