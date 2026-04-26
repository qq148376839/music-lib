package scrape

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/guohuiyuan/music-lib/model"
)

// Config controls which scraping steps are enabled.
type Config struct {
	Enabled bool
	Cover   bool
	Lyrics  bool
}

// Result reports the outcome of a scrape operation.
type Result struct {
	Status string // "done", "failed", "skipped"
	Error  string
}

var coverClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	},
}

const coverUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// Scrape writes metadata tags into the audio file at filePath.
// MP3 files get ID3v2.4 tags; FLAC files get Vorbis Comments.
// Cover art is downloaded and embedded if available.
// Lyrics are stripped of LRC timestamps and embedded as plain text.
//
// Scrape is best-effort: partial failures (e.g. cover download) are logged
// but do not prevent tag writing from succeeding.
func Scrape(cfg Config, song *model.Song, filePath, lyrics string) Result {
	if !cfg.Enabled {
		return Result{Status: "skipped"}
	}

	ext := strings.ToLower(filepath.Ext(filePath))

	// Download cover image for embedding.
	var coverData []byte
	var coverMIME string
	if cfg.Cover && song.Cover != "" {
		var err error
		coverData, coverMIME, err = downloadCoverImage(song.Cover)
		if err != nil {
			slog.Warn("scrape.cover_download", "error", err, "url", song.Cover)
		}
	}

	// Strip LRC timestamps for embedding as plain text.
	var plainLyrics string
	if cfg.Lyrics && lyrics != "" {
		plainLyrics = StripLRCTimestamps(lyrics)
	}

	// Write tags based on audio format.
	var err error
	switch ext {
	case ".mp3":
		err = writeMP3Tags(filePath, song, coverData, coverMIME, plainLyrics)
	case ".flac":
		err = writeFLACTags(filePath, song, coverData, coverMIME, plainLyrics)
	default:
		// m4a/aac: TODO — not yet supported.
		return Result{Status: "skipped", Error: fmt.Sprintf("unsupported format: %s", ext)}
	}

	if err != nil {
		return Result{Status: "failed", Error: err.Error()}
	}
	return Result{Status: "done"}
}

// downloadCoverImage fetches the cover image and returns raw bytes + MIME type.
func downloadCoverImage(url string) ([]byte, string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("User-Agent", coverUserAgent)
	resp, err := coverClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("http status %d", resp.StatusCode)
	}

	// Limit to 10 MB to guard against unexpectedly large responses.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}

	// Determine MIME from Content-Type header, falling back to content sniffing.
	mime := resp.Header.Get("Content-Type")
	if mime == "" || mime == "application/octet-stream" {
		mime = http.DetectContentType(data)
	}
	if strings.Contains(mime, "jpeg") || strings.Contains(mime, "jpg") {
		mime = "image/jpeg"
	} else if strings.Contains(mime, "png") {
		mime = "image/png"
	}

	return data, mime, nil
}
