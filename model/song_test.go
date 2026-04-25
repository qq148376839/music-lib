package model

import "testing"

func TestQualityString_Flac(t *testing.T) {
	s := &Song{Ext: "flac", Bitrate: 0}
	if got := s.QualityString(); got != "FLAC" {
		t.Errorf("expected FLAC, got %q", got)
	}
}

func TestQualityString_Wav(t *testing.T) {
	s := &Song{Ext: "wav", Bitrate: 0}
	if got := s.QualityString(); got != "WAV" {
		t.Errorf("expected WAV, got %q", got)
	}
}

func TestQualityString_MP3_WithBitrate(t *testing.T) {
	s := &Song{Ext: "mp3", Bitrate: 320}
	if got := s.QualityString(); got != "320kbps MP3" {
		t.Errorf("expected '320kbps MP3', got %q", got)
	}
}

func TestQualityString_MP3_128(t *testing.T) {
	s := &Song{Ext: "mp3", Bitrate: 128}
	if got := s.QualityString(); got != "128kbps MP3" {
		t.Errorf("expected '128kbps MP3', got %q", got)
	}
}

func TestQualityString_M4A_WithBitrate(t *testing.T) {
	s := &Song{Ext: "m4a", Bitrate: 96}
	if got := s.QualityString(); got != "96kbps M4A" {
		t.Errorf("expected '96kbps M4A', got %q", got)
	}
}

func TestQualityString_MP3_NoBitrate(t *testing.T) {
	s := &Song{Ext: "mp3", Bitrate: 0}
	if got := s.QualityString(); got != "MP3" {
		t.Errorf("expected 'MP3', got %q", got)
	}
}

func TestQualityString_Empty(t *testing.T) {
	s := &Song{Ext: "", Bitrate: 0}
	if got := s.QualityString(); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestQualityString_FlacIgnoresBitrate(t *testing.T) {
	// flac should NOT show bitrate even if it has one (shouldn't happen, but defensive)
	s := &Song{Ext: "flac", Bitrate: 1000}
	if got := s.QualityString(); got != "FLAC" {
		t.Errorf("expected 'FLAC' (no bitrate for lossless), got %q", got)
	}
}

func TestQualityString_CaseInsensitive(t *testing.T) {
	s := &Song{Ext: "FLAC", Bitrate: 0}
	if got := s.QualityString(); got != "FLAC" {
		t.Errorf("expected 'FLAC', got %q", got)
	}
}
