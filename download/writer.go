package download

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/utils"
)

// longClient has a generous timeout for large audio file downloads.
var longClient = &http.Client{Timeout: 10 * time.Minute}

// WriteSongToDisk downloads an audio file and saves it along with lyrics into
// the directory structure: {baseDir}/{Artist}/{Album}/{filename}.
// It returns the final file path on success.
func WriteSongToDisk(baseDir string, song *model.Song, audioURL, lyrics string, progressFn func(int64)) (string, error) {
	dir := buildSongDir(baseDir, song)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create dir: %w", err)
	}

	destPath := filepath.Join(dir, song.Filename())

	// Skip if the file already exists with a non-zero size.
	if info, err := os.Stat(destPath); err == nil && info.Size() > 0 {
		if progressFn != nil {
			progressFn(info.Size())
		}
		// Save lyrics even if audio already exists.
		if lyrics != "" {
			_ = saveLyrics(dir, song, lyrics)
		}
		return destPath, nil
	}

	n, err := downloadFile(destPath, audioURL, progressFn)
	if err != nil {
		return "", err
	}
	_ = n

	// Save lyrics alongside the audio file.
	if lyrics != "" {
		if lrcErr := saveLyrics(dir, song, lyrics); lrcErr != nil {
			// Non-fatal: log but don't fail the task.
			fmt.Printf("[download] lyrics save warning: %v\n", lrcErr)
		}
	}

	return destPath, nil
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
func downloadFile(destPath, url string, progressFn func(int64)) (int64, error) {
	resp, err := longClient.Get(url)
	if err != nil {
		return 0, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("http status %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return 0, fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	var written int64
	buf := make([]byte, 32*1024)
	for {
		nr, readErr := resp.Body.Read(buf)
		if nr > 0 {
			nw, writeErr := f.Write(buf[:nr])
			if writeErr != nil {
				return written, fmt.Errorf("write: %w", writeErr)
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
			return written, fmt.Errorf("read: %w", readErr)
		}
	}

	return written, nil
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
