package qq

import (
	"encoding/json"
	"testing"
)

func TestLoginTypePersistence(t *testing.T) {
	cf := cookieFile{
		Cookie:    "uin=123; qqmusic_key=abc",
		Nickname:  "testuser",
		LoginType: "5",
	}

	data, err := json.Marshal(cf)
	if err != nil {
		t.Fatal(err)
	}

	var restored cookieFile
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}

	if restored.LoginType != "5" {
		t.Errorf("expected LoginType=5, got %q", restored.LoginType)
	}
}

func TestLoginTypePersistence_Empty(t *testing.T) {
	cf := cookieFile{Cookie: "abc", Nickname: "user"}
	data, _ := json.Marshal(cf)

	var restored cookieFile
	json.Unmarshal(data, &restored)

	if restored.LoginType != "" {
		t.Errorf("expected empty LoginType, got %q", restored.LoginType)
	}
}

func TestLoginTypePersistence_OmitEmpty(t *testing.T) {
	cf := cookieFile{Cookie: "abc", Nickname: "user"}
	data, _ := json.Marshal(cf)
	// login_type should not appear in JSON when empty
	if json.Valid(data) {
		var raw map[string]interface{}
		json.Unmarshal(data, &raw)
		if _, ok := raw["login_type"]; ok {
			t.Error("login_type should be omitted when empty")
		}
	}
}

func TestSetGetLoginType(t *testing.T) {
	SetLoginType("3")
	if got := getLoginType(); got != "3" {
		t.Errorf("expected 3, got %q", got)
	}
	SetLoginType("")
	if got := getLoginType(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}
