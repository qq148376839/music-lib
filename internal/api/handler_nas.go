package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/guohuiyuan/music-lib/download"
	"github.com/guohuiyuan/music-lib/internal/store"
	"github.com/guohuiyuan/music-lib/model"
)

// proxyClient has a generous timeout for streaming audio files to the browser.
var proxyClient = &http.Client{Timeout: 10 * time.Minute}

// GET /api/nas/status
func (s *Server) handleNASStatus(c *gin.Context) {
	enabled := s.dlMgr != nil && s.dlMgr.MusicDir() != ""
	data := map[string]any{
		"enabled": enabled,
	}
	if enabled {
		data["music_dir"] = s.dlMgr.MusicDir()
		data["concurrency"] = s.dlMgr.Concurrency()
	}
	writeOK(c, data)
}

// POST /api/download/file?source=X  body: Song JSON
// Streams audio from source through server to browser.
func (s *Server) handleProxyDownload(c *gin.Context) {
	pf, source, ok := s.getProvider(c)
	if !ok {
		writeError(c, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if pf.GetDownloadURL == nil {
		writeError(c, http.StatusNotImplemented, fmt.Sprintf("download not supported for %s", source))
		return
	}

	var song model.Song
	if err := json.NewDecoder(c.Request.Body).Decode(&song); err != nil {
		writeError(c, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if quality := c.Query("quality"); quality != "" {
		if song.Extra == nil {
			song.Extra = map[string]string{}
		}
		song.Extra["quality"] = quality
	}

	audioURL, err := pf.GetDownloadURL(&song)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "get download url: "+err.Error())
		return
	}

	resp, err := proxyClient.Get(audioURL)
	if err != nil {
		writeError(c, http.StatusBadGateway, "fetch audio: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeError(c, http.StatusBadGateway, fmt.Sprintf("remote returned status %d", resp.StatusCode))
		return
	}

	filename := song.Filename()
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Header("Content-Type", ct)
	} else {
		c.Header("Content-Type", "application/octet-stream")
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		c.Header("Content-Length", cl)
	}
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		slog.Warn("proxy download stream interrupted", "song", song.Display(), "error", err)
	}
}

// POST /api/nas/download?source=X  body: Song JSON
func (s *Server) handleNASDownload(c *gin.Context) {
	if s.dlMgr == nil || s.dlMgr.MusicDir() == "" {
		writeError(c, http.StatusServiceUnavailable, "NAS download not configured (MUSIC_DIR not set)")
		return
	}

	pf, source, ok := s.getProvider(c)
	if !ok {
		writeError(c, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if pf.GetDownloadURL == nil {
		writeError(c, http.StatusNotImplemented, fmt.Sprintf("download not supported for %s", source))
		return
	}

	var song model.Song
	if err := json.NewDecoder(c.Request.Body).Decode(&song); err != nil {
		writeError(c, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if quality := c.Query("quality"); quality != "" {
		if song.Extra == nil {
			song.Extra = map[string]string{}
		}
		song.Extra["quality"] = quality
	}

	taskID := s.dlMgr.Enqueue(song, source, pf.GetDownloadURL, pf.GetLyrics)
	writeOK(c, map[string]string{"task_id": taskID})
}

// POST /api/nas/download/batch?source=X
// body: { "name": "歌单名", "songs": [...] }
// Accepts both "name" (new) and "playlist_name" (legacy) for the batch name field.
func (s *Server) handleNASBatchDownload(c *gin.Context) {
	if s.dlMgr == nil || s.dlMgr.MusicDir() == "" {
		writeError(c, http.StatusServiceUnavailable, "NAS download not configured (MUSIC_DIR not set)")
		return
	}

	pf, source, ok := s.getProvider(c)
	if !ok {
		writeError(c, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if pf.GetDownloadURL == nil {
		writeError(c, http.StatusNotImplemented, fmt.Sprintf("download not supported for %s", source))
		return
	}

	// Accept both "name" and legacy "playlist_name".
	var body struct {
		Name         string       `json:"name"`
		PlaylistName string       `json:"playlist_name"`
		Songs        []model.Song `json:"songs"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&body); err != nil {
		writeError(c, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if len(body.Songs) == 0 {
		writeError(c, http.StatusBadRequest, "no songs provided")
		return
	}
	// Prefer "name", fall back to legacy "playlist_name".
	batchName := body.Name
	if batchName == "" {
		batchName = body.PlaylistName
	}

	if quality := c.Query("quality"); quality != "" {
		for i := range body.Songs {
			if body.Songs[i].Extra == nil {
				body.Songs[i].Extra = map[string]string{}
			}
			body.Songs[i].Extra["quality"] = quality
		}
	}

	batchID := s.dlMgr.EnqueueBatch(body.Songs, batchName, source, pf.GetDownloadURL, pf.GetLyrics)

	// Persist the batch record to DB.
	if s.db != nil {
		if err := store.CreateBatch(s.db, batchID, source, batchName, len(body.Songs)); err != nil {
			slog.Warn("create batch record", "batch_id", batchID, "error", err)
		}
	}

	writeOK(c, map[string]any{
		"batch_id":   batchID,
		"task_count": len(body.Songs),
	})
}

// GET /api/nas/tasks
func (s *Server) handleListTasks(c *gin.Context) {
	if s.dlMgr == nil {
		writeOK(c, []*download.Task{})
		return
	}
	writeOK(c, s.dlMgr.ListTasks())
}

// GET /api/nas/task?id=X
func (s *Server) handleGetTask(c *gin.Context) {
	if s.dlMgr == nil {
		writeError(c, http.StatusNotFound, "task not found")
		return
	}
	id := c.Query("id")
	if id == "" {
		writeError(c, http.StatusBadRequest, "missing id parameter")
		return
	}
	task, ok := s.dlMgr.GetTask(id)
	if !ok {
		writeError(c, http.StatusNotFound, "task not found")
		return
	}
	writeOK(c, task)
}

// GET /api/nas/batches
// Aggregates batch stats from the DB (persistent, survives restarts).
// Falls back to in-memory aggregation if DB is unavailable.
func (s *Server) handleListBatches(c *gin.Context) {
	if s.db != nil {
		batches, err := store.ListBatchesWithStats(s.db)
		if err != nil {
			slog.Warn("list batches from db", "error", err)
		} else {
			writeOK(c, batches)
			return
		}
	}
	// Fallback to in-memory.
	if s.dlMgr == nil {
		writeOK(c, []download.BatchInfo{})
		return
	}
	writeOK(c, s.dlMgr.ListBatches())
}

// POST /api/nas/download/upgrade
// Re-queues completed tasks to attempt a higher-quality download.
//
// Query param:
//
//	quality: lossless (default) / high / standard
//
// Body:
//
//	{ "task_ids": ["t-xxx", ...] }  — empty array or omitted = upgrade all done tasks
func (s *Server) handleNASUpgrade(c *gin.Context) {
	if s.dlMgr == nil || s.dlMgr.MusicDir() == "" {
		writeError(c, http.StatusServiceUnavailable, "NAS download not configured (MUSIC_DIR not set)")
		return
	}

	quality := c.DefaultQuery("quality", "lossless")

	var body struct {
		TaskIDs []string `json:"task_ids"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(c, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	result := s.dlMgr.EnqueueUpgrade(body.TaskIDs, quality)

	if result.Queued == 0 && result.Skipped == 0 && len(result.Errors) == 0 {
		writeError(c, http.StatusNotFound, "no upgradeable tasks found")
		return
	}

	writeOK(c, result)
}
