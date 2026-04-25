package download

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/utils"
)

// HTTPError represents an HTTP response with an unexpected status code.
// Used by isRetryable to make structured decisions instead of string matching.
type HTTPError struct {
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("http status %d", e.StatusCode)
}

// longClient has a generous timeout for large audio file downloads.
var longClient = &http.Client{Timeout: 10 * time.Minute}

// WriteAction describes what happened when WriteSongToDisk ran.
type WriteAction string

const (
	// ActionNew means no existing file was found; the song was downloaded fresh.
	ActionNew WriteAction = "new"
	// ActionSkipped means an existing file with equal or higher quality was found; download was skipped.
	ActionSkipped WriteAction = "skipped"
	// ActionUpgraded means the new file has higher quality than the existing one; the old file was replaced.
	ActionUpgraded WriteAction = "upgraded"
)

// WriteResult contains the outcome of WriteSongToDisk.
type WriteResult struct {
	FilePath     string
	Action       WriteAction
	PreviousExt  string // populated only when Action == ActionUpgraded
	PreviousSize int64  // populated only when Action == ActionUpgraded
}

// qualityScore returns a numeric quality score for a file.
// Higher score = better quality. Used to decide whether to replace an existing file.
//
// Score table:
//
//	flac/wav:                1000
//	mp3, bitrate >= 320:      320
//	mp3, bitrate >= 128:      128
//	mp3, bitrate == 0:        size-based heuristic (>8MB=300, >4MB=200, else=100)
//	m4a, bitrate >= 256:      250
//	m4a (other):               90
//	anything else:             50
func qualityScore(ext string, bitrate int, fileSize int64) int {
	switch strings.ToLower(ext) {
	case "flac", "wav":
		return 1000
	case "mp3":
		if bitrate >= 320 {
			return 320
		}
		if bitrate >= 128 {
			return 128
		}
		// Heuristic based on file size when bitrate is unknown.
		if fileSize > 8*1024*1024 {
			return 300
		}
		if fileSize > 4*1024*1024 {
			return 200
		}
		return 100
	case "m4a", "aac":
		if bitrate >= 256 {
			return 250
		}
		return 90
	default:
		return 50
	}
}

// WriteSongToDisk downloads an audio file and saves it along with lyrics into
// the directory structure: {baseDir}/{Artist}/{Album}/{filename}.
//
// It searches for any existing file matching "{Artist} - {Name}.*" and compares
// quality scores. If an existing file has equal or higher quality, the download
// is skipped (ActionSkipped). If the new file has higher quality, the old file
// is atomically replaced (ActionUpgraded). Otherwise the file is written fresh
// (ActionNew).
//
// Lyrics and cover are saved regardless of the Action.
func WriteSongToDisk(baseDir string, song *model.Song, audioURL, lyrics string, progressFn func(int64)) (WriteResult, error) {
	dir := buildSongDir(baseDir, song)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return WriteResult{}, fmt.Errorf("create dir: %w", err)
	}

	newFilename := song.Filename()
	destPath := filepath.Join(dir, newFilename)

	// Find existing files with the same base name using ReadDir + prefix match.
	// Avoids filepath.Glob which treats [ ] as character class syntax — breaks
	// on song names containing brackets (e.g. "[Bonus Track]").
	baseName := utils.SanitizeFilename(fmt.Sprintf("%s - %s", song.Artist, song.Name))
	prefix := baseName + "."

	var audioMatches []string
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		slog.Warn("download.readdir_error", "dir", dir, "error", readErr)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".lrc" || ext == ".tmp" {
			continue
		}
		audioMatches = append(audioMatches, filepath.Join(dir, name))
	}

	if len(audioMatches) == 0 {
		// No existing file — normal download.
		if err := downloadFile(destPath, audioURL, progressFn); err != nil {
			return WriteResult{}, err
		}
		if lyrics != "" {
			if lrcErr := saveLyrics(dir, song, lyrics); lrcErr != nil {
				slog.Warn("download.lyrics_save", "error", lrcErr)
			}
		}
		return WriteResult{FilePath: destPath, Action: ActionNew}, nil
	}

	// Find the highest-quality existing file.
	bestExisting := audioMatches[0]
	bestScore := 0
	for _, m := range audioMatches {
		info, statErr := os.Stat(m)
		var size int64
		if statErr == nil {
			size = info.Size()
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(m)), ".")
		score := qualityScore(ext, 0, size)
		if score > bestScore {
			bestScore = score
			bestExisting = m
		}
	}

	existingInfo, statErr := os.Stat(bestExisting)
	var existingSize int64
	if statErr == nil {
		existingSize = existingInfo.Size()
	}
	existingExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(bestExisting)), ".")

	newScore := qualityScore(song.Ext, song.Bitrate, 0)
	existingScore := qualityScore(existingExt, 0, existingSize)

	if newScore <= existingScore {
		// Existing file is at least as good — skip download, still save lyrics.
		if lyrics != "" {
			if lrcErr := saveLyrics(dir, song, lyrics); lrcErr != nil {
				slog.Warn("download.lyrics_save", "error", lrcErr)
			}
		}
		slog.Info("download.skipped",
			"existing", bestExisting,
			"existing_score", existingScore,
			"new_score", newScore,
		)
		return WriteResult{FilePath: bestExisting, Action: ActionSkipped}, nil
	}

	// New file has higher quality — safe replace via tmp file.
	tmpPath := destPath + ".tmp"

	if err := downloadFile(tmpPath, audioURL, progressFn); err != nil {
		// Clean up tmp on failure; old file is untouched.
		_ = os.Remove(tmpPath)
		return WriteResult{}, fmt.Errorf("download upgrade: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return WriteResult{}, fmt.Errorf("rename tmp file: %w", err)
	}

	// Remove old file if it has a different path (different extension).
	if bestExisting != destPath {
		if removeErr := os.Remove(bestExisting); removeErr != nil {
			slog.Warn("download.remove_old_file_failed",
				"old_path", bestExisting,
				"error", removeErr,
			)
		}
	}

	if lyrics != "" {
		if lrcErr := saveLyrics(dir, song, lyrics); lrcErr != nil {
			slog.Warn("download.lyrics_save", "error", lrcErr)
		}
	}

	slog.Info("download.upgrade",
		"old_path", bestExisting,
		"new_path", destPath,
		"previous_ext", existingExt,
		"previous_size", existingSize,
		"new_ext", song.Ext,
		"action", "upgraded",
	)

	return WriteResult{
		FilePath:     destPath,
		Action:       ActionUpgraded,
		PreviousExt:  existingExt,
		PreviousSize: existingSize,
	}, nil
}

// buildSongDir returns {base}/{Artist}/{Album}.
func buildSongDir(baseDir string, song *model.Song) string {
	artist := song.Artist
	if artist == "" {
		artist = "Unknown Artist"
	}
	album := song.Album
	if album == "" {
		album = "Unknown Album"
	}
	return filepath.Join(baseDir, utils.SanitizeFilename(artist), utils.SanitizeFilename(album))
}

// downloadFile streams a URL to destPath, calling progressFn with cumulative bytes.
func downloadFile(destPath, url string, progressFn func(int64)) error {
	resp, err := longClient.Get(url)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &HTTPError{StatusCode: resp.StatusCode}
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	var written int64
	buf := make([]byte, 32*1024)
	for {
		nr, readErr := resp.Body.Read(buf)
		if nr > 0 {
			nw, writeErr := f.Write(buf[:nr])
			if writeErr != nil {
				return fmt.Errorf("write: %w", writeErr)
			}
			written += int64(nw)
			if progressFn != nil {
				progressFn(written)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return fmt.Errorf("read: %w", readErr)
		}
	}

	return nil
}

// saveLyrics writes an LRC file next to the audio file.
func saveLyrics(dir string, song *model.Song, lyrics string) error {
	lrcPath := filepath.Join(dir, song.LrcFilename())
	return os.WriteFile(lrcPath, []byte(lyrics), 0644)
}

// saveCover downloads and saves cover.jpg into the given directory.
// It skips if cover.jpg already exists.
func saveCover(dir, coverURL string) error {
	coverPath := filepath.Join(dir, "cover.jpg")
	if _, err := os.Stat(coverPath); err == nil {
		return nil // already exists
	}

	resp, err := longClient.Get(coverURL)
	if err != nil {
		return fmt.Errorf("http get cover: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cover http status %d", resp.StatusCode)
	}

	f, err := os.Create(coverPath)
	if err != nil {
		return fmt.Errorf("create cover file: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
