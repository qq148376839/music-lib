package scrape

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/guohuiyuan/music-lib/model"
)

// --- StripLRCTimestamps ---

func TestStripLRCTimestamps_Standard(t *testing.T) {
	input := "[00:12.34]第一行歌词\n[00:15.67]第二行歌词\n[00:20.00]第三行"
	got := StripLRCTimestamps(input)
	want := "第一行歌词\n第二行歌词\n第三行"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStripLRCTimestamps_ThreeDigitMs(t *testing.T) {
	input := "[00:12.345]三位毫秒"
	got := StripLRCTimestamps(input)
	if got != "三位毫秒" {
		t.Fatalf("got %q", got)
	}
}

func TestStripLRCTimestamps_NoMs(t *testing.T) {
	input := "[01:30]无毫秒"
	got := StripLRCTimestamps(input)
	if got != "无毫秒" {
		t.Fatalf("got %q", got)
	}
}

func TestStripLRCTimestamps_MultipleTagsPerLine(t *testing.T) {
	input := "[00:12.34][00:45.67]重复时间轴"
	got := StripLRCTimestamps(input)
	if got != "重复时间轴" {
		t.Fatalf("got %q", got)
	}
}

func TestStripLRCTimestamps_EmptyLinesRemoved(t *testing.T) {
	input := "[00:01.00]\n[00:02.00]有内容\n[00:03.00]\n"
	got := StripLRCTimestamps(input)
	if got != "有内容" {
		t.Fatalf("got %q", got)
	}
}

func TestStripLRCTimestamps_PlainTextPassthrough(t *testing.T) {
	input := "纯文本歌词\n没有时间轴"
	got := StripLRCTimestamps(input)
	if got != input {
		t.Fatalf("got %q", got)
	}
}

func TestStripLRCTimestamps_MetadataPreserved(t *testing.T) {
	input := "[ti:歌名]\n[ar:歌手]\n[00:01.00]歌词"
	got := StripLRCTimestamps(input)
	// [ti:...] and [ar:...] don't match the timestamp pattern, so they're preserved.
	if got != "[ti:歌名]\n[ar:歌手]\n歌词" {
		t.Fatalf("got %q", got)
	}
}

func TestStripLRCTimestamps_Empty(t *testing.T) {
	if got := StripLRCTimestamps(""); got != "" {
		t.Fatalf("got %q", got)
	}
}

// --- IsLRC ---

func TestIsLRC_True(t *testing.T) {
	if !IsLRC("[00:12.34]歌词") {
		t.Fatal("expected true for LRC text")
	}
}

func TestIsLRC_False(t *testing.T) {
	if IsLRC("纯文本歌词") {
		t.Fatal("expected false for plain text")
	}
}

func TestIsLRC_MetadataOnly(t *testing.T) {
	if IsLRC("[ti:Song Title]") {
		t.Fatal("expected false for metadata-only LRC")
	}
}

// --- Scrape orchestration ---

func TestScrape_Disabled(t *testing.T) {
	result := Scrape(Config{Enabled: false}, &model.Song{}, "/nonexistent.mp3", "")
	if result.Status != "skipped" {
		t.Fatalf("expected skipped, got %q", result.Status)
	}
}

func TestScrape_UnsupportedFormat(t *testing.T) {
	// Create a temp file with unsupported extension.
	tmp := filepath.Join(t.TempDir(), "song.m4a")
	os.WriteFile(tmp, []byte("fake"), 0644)

	result := Scrape(Config{Enabled: true}, &model.Song{Name: "Test"}, tmp, "")
	if result.Status != "skipped" {
		t.Fatalf("expected skipped for m4a, got %q", result.Status)
	}
	if result.Error == "" {
		t.Fatal("expected error message for unsupported format")
	}
}

// --- downloadCoverImage ---

func TestDownloadCoverImage_Success(t *testing.T) {
	// 1x1 white JPEG.
	jpegData := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
		0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
		0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
		0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
		0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
		0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32,
		0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
		0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x1F, 0x00, 0x00,
		0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0A, 0x0B, 0xFF, 0xC4, 0x00, 0xB5, 0x10, 0x00, 0x02, 0x01, 0x03,
		0x03, 0x02, 0x04, 0x03, 0x05, 0x05, 0x04, 0x04, 0x00, 0x00, 0x01, 0x7D,
		0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12, 0x21, 0x31, 0x41, 0x06,
		0x13, 0x51, 0x61, 0x07, 0x22, 0x71, 0x14, 0x32, 0x81, 0x91, 0xA1, 0x08,
		0x23, 0x42, 0xB1, 0xC1, 0x15, 0x52, 0xD1, 0xF0, 0x24, 0x33, 0x62, 0x72,
		0x82, 0x09, 0x0A, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x25, 0x26, 0x27, 0x28,
		0x29, 0x2A, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3A, 0x43, 0x44, 0x45,
		0x46, 0x47, 0x48, 0x49, 0x4A, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59,
		0x5A, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6A, 0x73, 0x74, 0x75,
		0x76, 0x77, 0x78, 0x79, 0x7A, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89,
		0x8A, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9A, 0xA2, 0xA3,
		0xA4, 0xA5, 0xA6, 0xA7, 0xA8, 0xA9, 0xAA, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6,
		0xB7, 0xB8, 0xB9, 0xBA, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7, 0xC8, 0xC9,
		0xCA, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7, 0xD8, 0xD9, 0xDA, 0xE1, 0xE2,
		0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9, 0xEA, 0xF1, 0xF2, 0xF3, 0xF4,
		0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01,
		0x00, 0x00, 0x3F, 0x00, 0x7B, 0x94, 0x11, 0x00, 0x00, 0x00, 0x00, 0xFF,
		0xD9,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(jpegData)
	}))
	defer srv.Close()

	data, mime, err := downloadCoverImage(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mime != "image/jpeg" {
		t.Fatalf("expected image/jpeg, got %q", mime)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty data")
	}
}

func TestDownloadCoverImage_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, _, err := downloadCoverImage(srv.URL)
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestDownloadCoverImage_ContentSniff(t *testing.T) {
	// Minimal PNG header.
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Deliberately set wrong content type.
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(pngHeader)
	}))
	defer srv.Close()

	_, mime, err := downloadCoverImage(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mime != "image/png" {
		t.Fatalf("expected image/png from sniffing, got %q", mime)
	}
}

// --- MP3 tag writing ---

func TestWriteMP3Tags_InvalidFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "bad.mp3")
	os.WriteFile(tmp, []byte("not a real mp3"), 0644)

	song := &model.Song{Name: "Test", Artist: "Artist", Album: "Album"}
	// id3v2 should still be able to open and write tags to any file,
	// since ID3v2 tags are prepended. This tests that no panic occurs.
	err := writeMP3Tags(tmp, song, nil, "", "")
	if err != nil {
		t.Fatalf("writeMP3Tags on non-MP3 should not error (ID3v2 prepends): %v", err)
	}

	// Verify we can read back tags.
	info, statErr := os.Stat(tmp)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Size() <= 14 {
		t.Fatal("file should have grown with ID3 header")
	}
}

func TestWriteMP3Tags_WithCoverAndLyrics(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "song.mp3")
	os.WriteFile(tmp, make([]byte, 512), 0644) // dummy file (id3v2 needs >= 10 bytes to check header)

	song := &model.Song{
		Name:   "歌名",
		Artist: "歌手",
		Album:  "专辑",
		Extra:  map[string]string{"year": "2024", "genre": "Pop"},
	}
	cover := []byte{0xFF, 0xD8, 0xFF, 0xE0} // JPEG magic
	err := writeMP3Tags(tmp, song, cover, "image/jpeg", "歌词内容")
	if err != nil {
		t.Fatalf("writeMP3Tags: %v", err)
	}

	info, _ := os.Stat(tmp)
	if info.Size() <= 4 {
		t.Fatal("file should have grown with ID3 tags")
	}
}

// --- Scrape integration ---

func TestScrape_MP3(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.mp3")
	os.WriteFile(tmp, make([]byte, 512), 0644)

	song := &model.Song{Name: "Test", Artist: "Artist", Album: "Album"}
	result := Scrape(Config{Enabled: true, Cover: false, Lyrics: true}, song, tmp, "[00:01.00]歌词")
	if result.Status != "done" {
		t.Fatalf("expected done, got %q (error: %s)", result.Status, result.Error)
	}
}

func TestScrape_CoverFromServer(t *testing.T) {
	// Set up a cover image server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0})
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "test.mp3")
	os.WriteFile(tmp, make([]byte, 512), 0644)

	song := &model.Song{Name: "Test", Artist: "Artist", Album: "Album", Cover: srv.URL}
	result := Scrape(Config{Enabled: true, Cover: true, Lyrics: false}, song, tmp, "")
	if result.Status != "done" {
		t.Fatalf("expected done, got %q (error: %s)", result.Status, result.Error)
	}
}

func TestScrape_CoverDownloadFails_StillSucceeds(t *testing.T) {
	// Cover URL points to a server that returns 500.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "test.mp3")
	os.WriteFile(tmp, make([]byte, 512), 0644)

	song := &model.Song{Name: "Test", Artist: "Artist", Album: "Album", Cover: srv.URL}
	result := Scrape(Config{Enabled: true, Cover: true, Lyrics: false}, song, tmp, "")
	// Cover download fails but tag writing should still succeed.
	if result.Status != "done" {
		t.Fatalf("expected done even with cover failure, got %q (error: %s)", result.Status, result.Error)
	}
}

func TestScrape_FLAC_InvalidFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "bad.flac")
	os.WriteFile(tmp, []byte("not flac"), 0644)

	song := &model.Song{Name: "Test", Artist: "Artist", Album: "Album"}
	result := Scrape(Config{Enabled: true}, song, tmp, "lyrics")
	// FLAC parser should fail on invalid data.
	if result.Status != "failed" {
		t.Fatalf("expected failed for invalid FLAC, got %q", result.Status)
	}
	if result.Error == "" {
		t.Fatal("expected error message")
	}
	fmt.Println("FLAC error (expected):", result.Error)
}
