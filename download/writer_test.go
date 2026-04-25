package download

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guohuiyuan/music-lib/model"
)

// --- qualityScore ---

func TestQualityScore_Flac(t *testing.T) {
	score := qualityScore("flac", 0, 0)
	if score != 1000 {
		t.Fatalf("flac: expected 1000, got %d", score)
	}
}

func TestQualityScore_Wav(t *testing.T) {
	score := qualityScore("wav", 0, 0)
	if score != 1000 {
		t.Fatalf("wav: expected 1000, got %d", score)
	}
}

func TestQualityScore_MP3_320(t *testing.T) {
	score := qualityScore("mp3", 320, 0)
	if score != 320 {
		t.Fatalf("mp3 320k: expected 320, got %d", score)
	}
}

func TestQualityScore_MP3_128(t *testing.T) {
	score := qualityScore("mp3", 128, 0)
	if score != 128 {
		t.Fatalf("mp3 128k: expected 128, got %d", score)
	}
}

func TestQualityScore_MP3_Unknown_LargeFile(t *testing.T) {
	score := qualityScore("mp3", 0, 10*1024*1024) // 10 MB
	if score != 300 {
		t.Fatalf("mp3 unknown large: expected 300, got %d", score)
	}
}

func TestQualityScore_MP3_Unknown_MediumFile(t *testing.T) {
	score := qualityScore("mp3", 0, 5*1024*1024) // 5 MB
	if score != 200 {
		t.Fatalf("mp3 unknown medium: expected 200, got %d", score)
	}
}

func TestQualityScore_MP3_Unknown_SmallFile(t *testing.T) {
	score := qualityScore("mp3", 0, 1*1024*1024) // 1 MB
	if score != 100 {
		t.Fatalf("mp3 unknown small: expected 100, got %d", score)
	}
}

func TestQualityScore_M4A_High(t *testing.T) {
	score := qualityScore("m4a", 256, 0)
	if score != 250 {
		t.Fatalf("m4a 256k: expected 250, got %d", score)
	}
}

func TestQualityScore_M4A_Low(t *testing.T) {
	score := qualityScore("m4a", 96, 0)
	if score != 90 {
		t.Fatalf("m4a 96k: expected 90, got %d", score)
	}
}

func TestQualityScore_Unknown(t *testing.T) {
	score := qualityScore("ogg", 0, 0)
	if score != 50 {
		t.Fatalf("ogg: expected 50, got %d", score)
	}
}

// Quality ordering: flac > mp3 320k > mp3 128k > m4a 96k
func TestQualityScore_Ordering(t *testing.T) {
	flac := qualityScore("flac", 0, 0)
	mp3_320 := qualityScore("mp3", 320, 0)
	mp3_128 := qualityScore("mp3", 128, 0)
	m4a := qualityScore("m4a", 96, 0)

	if !(flac > mp3_320) {
		t.Errorf("expected flac(%d) > mp3_320(%d)", flac, mp3_320)
	}
	if !(mp3_320 > mp3_128) {
		t.Errorf("expected mp3_320(%d) > mp3_128(%d)", mp3_320, mp3_128)
	}
	if !(mp3_128 > m4a) {
		t.Errorf("expected mp3_128(%d) > m4a(%d)", mp3_128, m4a)
	}
}

// --- WriteSongToDisk ---

// makeAudioServer returns a test HTTP server that serves the given bytes as audio.
func makeAudioServer(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
}

func testSong(ext, artist, name string, bitrate int) model.Song {
	return model.Song{
		Source:  "test",
		ID:      "s-001",
		Name:    name,
		Artist:  artist,
		Album:   "Test Album",
		Ext:     ext,
		Bitrate: bitrate,
	}
}

// TestWriteSongToDisk_ActionNew: no existing file → ActionNew
func TestWriteSongToDisk_ActionNew(t *testing.T) {
	srv := makeAudioServer(t, []byte("fake mp3 data"))
	defer srv.Close()

	baseDir := t.TempDir()
	song := testSong("mp3", "TestArtist", "TestSong", 128)

	result, err := WriteSongToDisk(baseDir, &song, srv.URL, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionNew {
		t.Errorf("expected ActionNew, got %s", result.Action)
	}
	if result.FilePath == "" {
		t.Error("FilePath should not be empty")
	}
	if _, statErr := os.Stat(result.FilePath); statErr != nil {
		t.Errorf("file not found: %v", statErr)
	}
}

// TestWriteSongToDisk_ActionSkipped: existing FLAC file, new MP3 → ActionSkipped
func TestWriteSongToDisk_ActionSkipped(t *testing.T) {
	baseDir := t.TempDir()
	song := testSong("mp3", "TestArtist", "TestSong", 128)

	// Create the directory structure manually.
	dir := buildSongDir(baseDir, &song)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write an existing FLAC file (higher quality than mp3 128k).
	existingFlac := filepath.Join(dir, "TestArtist - TestSong.flac")
	if err := os.WriteFile(existingFlac, make([]byte, 9*1024*1024), 0644); err != nil {
		t.Fatal(err)
	}

	srv := makeAudioServer(t, []byte("fake mp3 data"))
	defer srv.Close()

	result, err := WriteSongToDisk(baseDir, &song, srv.URL, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionSkipped {
		t.Errorf("expected ActionSkipped, got %s", result.Action)
	}
	// Existing FLAC file should still be present.
	if _, statErr := os.Stat(existingFlac); statErr != nil {
		t.Errorf("existing file should remain: %v", statErr)
	}
}

// TestWriteSongToDisk_ActionUpgraded: existing MP3 128k, new FLAC → ActionUpgraded
func TestWriteSongToDisk_ActionUpgraded(t *testing.T) {
	baseDir := t.TempDir()
	song := testSong("flac", "TestArtist", "TestSong", 0)

	// Create existing low-quality MP3 file.
	dir := buildSongDir(baseDir, &song)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	existingMP3 := filepath.Join(dir, "TestArtist - TestSong.mp3")
	if err := os.WriteFile(existingMP3, make([]byte, 3*1024*1024), 0644); err != nil {
		t.Fatal(err)
	}

	srv := makeAudioServer(t, []byte("fake flac data with extra bytes to be larger"))
	defer srv.Close()

	result, err := WriteSongToDisk(baseDir, &song, srv.URL, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionUpgraded {
		t.Errorf("expected ActionUpgraded, got %s", result.Action)
	}
	if result.PreviousExt != "mp3" {
		t.Errorf("expected PreviousExt=mp3, got %q", result.PreviousExt)
	}
	// New FLAC file should exist.
	newFlac := filepath.Join(dir, "TestArtist - TestSong.flac")
	if _, statErr := os.Stat(newFlac); statErr != nil {
		t.Errorf("new FLAC file not found: %v", statErr)
	}
	// Old MP3 file should have been deleted.
	if _, statErr := os.Stat(existingMP3); statErr == nil {
		t.Error("old MP3 file should have been removed")
	}
}

// TestWriteSongToDisk_SameFormatSameBitrate: same ext+bitrate → ActionSkipped (score equal)
func TestWriteSongToDisk_SameFormatSameBitrate(t *testing.T) {
	baseDir := t.TempDir()
	song := testSong("mp3", "TestArtist", "TestSong", 128)

	dir := buildSongDir(baseDir, &song)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	existingMP3 := filepath.Join(dir, "TestArtist - TestSong.mp3")
	if err := os.WriteFile(existingMP3, make([]byte, 5*1024*1024), 0644); err != nil {
		t.Fatal(err)
	}

	srv := makeAudioServer(t, []byte("new mp3 data"))
	defer srv.Close()

	result, err := WriteSongToDisk(baseDir, &song, srv.URL, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Same quality score → skip.
	if result.Action != ActionSkipped {
		t.Errorf("expected ActionSkipped for same format+bitrate, got %s", result.Action)
	}
}

// TestWriteSongToDisk_LyricsAlwaysSaved: lyrics saved even when ActionSkipped
func TestWriteSongToDisk_LyricsAlwaysSaved(t *testing.T) {
	baseDir := t.TempDir()
	song := testSong("mp3", "TestArtist", "TestSong", 128)

	dir := buildSongDir(baseDir, &song)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write existing higher-quality file to force skip.
	existingFlac := filepath.Join(dir, "TestArtist - TestSong.flac")
	if err := os.WriteFile(existingFlac, make([]byte, 1024), 0644); err != nil {
		t.Fatal(err)
	}

	srv := makeAudioServer(t, []byte("fake mp3"))
	defer srv.Close()

	const lyricsContent = "[00:00.00] Test lyrics line"
	result, err := WriteSongToDisk(baseDir, &song, srv.URL, lyricsContent, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionSkipped {
		t.Fatalf("expected ActionSkipped, got %s", result.Action)
	}

	lrcPath := filepath.Join(dir, song.LrcFilename())
	data, readErr := os.ReadFile(lrcPath)
	if readErr != nil {
		t.Fatalf("lyrics file not saved: %v", readErr)
	}
	if !strings.Contains(string(data), "Test lyrics line") {
		t.Error("lyrics content mismatch")
	}
}
