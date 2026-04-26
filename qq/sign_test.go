package qq

import (
	"strings"
	"testing"
)

func TestZZBSign(t *testing.T) {
	// Fixed input should produce a deterministic output.
	input := `{"comm":{"ct":"11"},"test":true}`
	sig := zzbSign(input)

	if !strings.HasPrefix(sig, "zzb") {
		t.Errorf("sign should start with 'zzb', got %q", sig)
	}
	if len(sig) < 10 {
		t.Errorf("sign too short: %d chars", len(sig))
	}
	// Must not contain /, +, =
	for _, c := range []string{"/", "+", "="} {
		if strings.Contains(sig, c) {
			t.Errorf("sign should not contain %q, got %q", c, sig)
		}
	}
	// Must be lowercase
	if sig != strings.ToLower(sig) {
		t.Errorf("sign should be lowercase, got %q", sig)
	}
}

func TestZZBSign_Deterministic(t *testing.T) {
	input := `{"hello":"world"}`
	s1 := zzbSign(input)
	s2 := zzbSign(input)
	if s1 != s2 {
		t.Errorf("same input should produce same sign: %q != %q", s1, s2)
	}
}

func TestZZBSign_Empty(t *testing.T) {
	// Should not panic on empty input.
	sig := zzbSign("")
	if !strings.HasPrefix(sig, "zzb") {
		t.Errorf("sign should start with 'zzb', got %q", sig)
	}
}

func TestZZBSign_DifferentInputs(t *testing.T) {
	s1 := zzbSign("abc")
	s2 := zzbSign("def")
	if s1 == s2 {
		t.Error("different inputs should produce different signs")
	}
}

func TestMiddle(t *testing.T) {
	// MD5 of "" is D41D8CD98F00B204E9800998ECF8427E (uppercase)
	hex := []byte("D41D8CD98F00B204E9800998ECF8427E")
	m := middle(hex)
	if len(m) != 16 {
		t.Fatalf("middle should return 16 bytes, got %d", len(m))
	}
}

func TestExtractBytes(t *testing.T) {
	b := []byte("ABCDEFGHIJ")
	result := extractBytes(b, []int{0, 2, 4})
	if string(result) != "ACE" {
		t.Errorf("expected ACE, got %s", string(result))
	}
}

func TestHexVal(t *testing.T) {
	tests := []struct {
		in   byte
		want byte
	}{
		{'0', 0}, {'9', 9}, {'A', 10}, {'F', 15},
	}
	for _, tt := range tests {
		if got := hexVal(tt.in); got != tt.want {
			t.Errorf("hexVal(%c) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
