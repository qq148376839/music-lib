package qq

import (
	"encoding/json"
	"testing"
)

func TestDetectAuthMode_AppAuth(t *testing.T) {
	q := New("uin=12345; qqmusic_key=abc123; qm_keyst=abc123; tmeLoginType=2")
	auth := q.detectAuthMode()
	if auth.mode != "app" {
		t.Errorf("expected app mode, got %q", auth.mode)
	}
	if auth.authst != "abc123" {
		t.Errorf("expected authst=abc123, got %q", auth.authst)
	}
	if auth.uin != "12345" {
		t.Errorf("expected uin=12345, got %q", auth.uin)
	}
	if auth.loginType != "2" {
		t.Errorf("expected loginType=2, got %q", auth.loginType)
	}
}

func TestDetectAuthMode_WebAuth(t *testing.T) {
	q := New("p_uin=o0012345; p_skey=abcdef")
	auth := q.detectAuthMode()
	if auth.mode != "web" {
		t.Errorf("expected web mode, got %q", auth.mode)
	}
	if auth.gtk == 5381 {
		t.Error("gtk should not be anonymous value when p_skey is present")
	}
}

func TestDetectAuthMode_Empty(t *testing.T) {
	q := New("")
	auth := q.detectAuthMode()
	if auth.mode != "web" {
		t.Errorf("expected web mode for empty cookie, got %q", auth.mode)
	}
	if auth.uin != "0" {
		t.Errorf("expected uin=0, got %q", auth.uin)
	}
	if auth.gtk != 5381 {
		t.Errorf("expected anonymous gtk=5381, got %d", auth.gtk)
	}
}

func TestDetectAuthMode_LoginTypeFromCookie(t *testing.T) {
	q := New("uin=123; qqmusic_key=key; tmeLoginType=5")
	auth := q.detectAuthMode()
	if auth.loginType != "5" {
		t.Errorf("expected loginType=5, got %q", auth.loginType)
	}
}

func TestDetectAuthMode_LoginTypeFallback(t *testing.T) {
	// No tmeLoginType in cookie, no persisted value → default "2"
	SetLoginType("") // clear
	q := New("uin=123; qqmusic_key=key")
	auth := q.detectAuthMode()
	if auth.loginType != "2" {
		t.Errorf("expected default loginType=2, got %q", auth.loginType)
	}
}

func TestGenerateHexGUID(t *testing.T) {
	guid := generateHexGUID()
	if len(guid) != 32 {
		t.Fatalf("expected 32 chars, got %d", len(guid))
	}
	for _, c := range guid {
		if !((c >= 'a' && c <= 'f') || (c >= '0' && c <= '9')) {
			t.Errorf("invalid char %c in GUID %q", c, guid)
		}
	}
}

func TestGenerateHexGUID_Unique(t *testing.T) {
	g1 := generateHexGUID()
	g2 := generateHexGUID()
	if g1 == g2 {
		t.Error("two GUIDs should not be identical")
	}
}

func TestBuildAppRequest(t *testing.T) {
	q := New("uin=12345; qqmusic_key=testkey; tmeLoginType=2")
	auth := q.detectAuthMode()
	if auth.mode != "app" {
		t.Fatal("expected app mode")
	}

	// Build a sample request body like GetDownloadURL does.
	guid := generateHexGUID()
	reqData := map[string]interface{}{
		"comm": map[string]interface{}{
			"cv": appCV, "v": appCV, "ct": appCT,
			"tmeAppID": "qqmusic", "qq": auth.uin,
			"authst": auth.authst, "tmeLoginType": auth.loginType,
		},
		"music.vkey.GetVkey.UrlGetVkey": map[string]interface{}{
			"module": "music.vkey.GetVkey",
			"method": "UrlGetVkey",
			"param": map[string]interface{}{
				"guid": guid, "songmid": []string{"test"},
				"filename": []string{"F000testtest.flac"},
			},
		},
	}

	data, _ := json.Marshal(reqData)
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)

	comm := parsed["comm"].(map[string]interface{})
	if comm["ct"] != appCT {
		t.Errorf("expected ct=%q, got %v", appCT, comm["ct"])
	}
	if comm["authst"] != "testkey" {
		t.Errorf("expected authst=testkey, got %v", comm["authst"])
	}
	if _, ok := parsed["music.vkey.GetVkey.UrlGetVkey"]; !ok {
		t.Error("expected app-mode key 'music.vkey.GetVkey.UrlGetVkey'")
	}
}

func TestBuildWebRequest(t *testing.T) {
	q := New("p_uin=o0012345; p_skey=abc")
	auth := q.detectAuthMode()
	if auth.mode != "web" {
		t.Fatal("expected web mode")
	}

	reqData := map[string]interface{}{
		"comm": map[string]interface{}{
			"cv": 4747474, "ct": 24, "g_tk": auth.gtk, "uin": auth.uin,
		},
		"req_1": map[string]interface{}{
			"module": "music.vkey.GetVkey",
		},
	}

	data, _ := json.Marshal(reqData)
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)

	comm := parsed["comm"].(map[string]interface{})
	if ct, ok := comm["ct"].(float64); !ok || int(ct) != 24 {
		t.Errorf("expected ct=24, got %v", comm["ct"])
	}
	if _, ok := parsed["req_1"]; !ok {
		t.Error("expected web-mode key 'req_1'")
	}
}

func TestExtractMusicKey(t *testing.T) {
	tests := []struct {
		cookie string
		want   string
	}{
		{"uin=123; qqmusic_key=ABCDEF; qm_keyst=ABCDEF", "ABCDEF"},
		{"uin=123; p_skey=xyz", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractCookieValue(tt.cookie, "qqmusic_key")
		if got != tt.want {
			t.Errorf("extractCookieValue(%q, qqmusic_key) = %q, want %q", tt.cookie, got, tt.want)
		}
	}
}

func TestParseVkeyResponse_AppMode(t *testing.T) {
	resp := `{"music.vkey.GetVkey.UrlGetVkey":{"code":0,"data":{"midurlinfo":[{"purl":"test.flac?vkey=abc","result":0}]}}}`
	purl, code, err := parseVkeyResponse([]byte(resp), "music.vkey.GetVkey.UrlGetVkey")
	if err != nil {
		t.Fatal(err)
	}
	if purl != "test.flac?vkey=abc" {
		t.Errorf("expected purl, got %q", purl)
	}
	if code != 0 {
		t.Errorf("expected code=0, got %d", code)
	}
}

func TestParseVkeyResponse_WebMode(t *testing.T) {
	resp := `{"req_1":{"code":0,"data":{"midurlinfo":[{"purl":"test.mp3?vkey=def","result":0}]}}}`
	purl, _, err := parseVkeyResponse([]byte(resp), "req_1")
	if err != nil {
		t.Fatal(err)
	}
	if purl != "test.mp3?vkey=def" {
		t.Errorf("expected purl, got %q", purl)
	}
}

func TestParseVkeyResponse_EmptyPurl(t *testing.T) {
	resp := `{"req_1":{"code":0,"data":{"midurlinfo":[{"purl":"","result":301}]}}}`
	purl, code, err := parseVkeyResponse([]byte(resp), "req_1")
	if err != nil {
		t.Fatal(err)
	}
	if purl != "" {
		t.Errorf("expected empty purl, got %q", purl)
	}
	if code != 301 {
		t.Errorf("expected code=301, got %d", code)
	}
}

func TestParseVkeyResponse_MissingKey(t *testing.T) {
	resp := `{"req_1":{"code":0,"data":{}}}`
	_, _, err := parseVkeyResponse([]byte(resp), "wrong_key")
	if err == nil {
		t.Error("expected error for missing key")
	}
}
