package download

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/guohuiyuan/music-lib/model"
)

// ProviderFuncs holds the callbacks needed for downloading from a specific source.
type ProviderFuncs struct {
	Search         func(string) ([]model.Song, error)
	GetDownloadURL func(*model.Song) (string, error)
	GetLyrics      func(*model.Song) (string, error)
}

// Handlers exposes the HTTP handlers for download endpoints.
type Handlers struct {
	mgr       *Manager
	providers map[string]ProviderFuncs
}

// NewHandlers creates a Handlers instance.
func NewHandlers(mgr *Manager, providers map[string]ProviderFuncs) *Handlers {
	return &Handlers{mgr: mgr, providers: providers}
}

// --- response helpers (local to package) ---

type apiResp struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

func writeJSON(w http.ResponseWriter, httpCode int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(httpCode)
	json.NewEncoder(w).Encode(data)
}

func writeOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, apiResp{Code: 0, Data: data})
}

func writeError(w http.ResponseWriter, httpCode int, msg string) {
	writeJSON(w, httpCode, apiResp{Code: -1, Message: msg})
}

func (h *Handlers) resolveProvider(r *http.Request) (ProviderFuncs, string, bool) {
	source := r.URL.Query().Get("source")
	if source == "" {
		return ProviderFuncs{}, "", false
	}
	p, ok := h.providers[source]
	return p, source, ok
}

func decodeSong(r *http.Request) (model.Song, error) {
	var song model.Song
	if err := json.NewDecoder(r.Body).Decode(&song); err != nil {
		return song, err
	}
	return song, nil
}

// --- Handlers ---

// HandleProxyDownload streams audio from the source through the server to the browser.
// POST /api/download/file?source=X  body: Song JSON
func (h *Handlers) HandleProxyDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	pf, source, ok := h.resolveProvider(r)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if pf.GetDownloadURL == nil {
		writeError(w, http.StatusNotImplemented, fmt.Sprintf("download not supported for %s", source))
		return
	}

	song, err := decodeSong(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	audioURL, err := pf.GetDownloadURL(&song)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get download url: "+err.Error())
		return
	}

	// Fetch the remote audio.
	resp, err := longClient.Get(audioURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "fetch audio: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("remote returned status %d", resp.StatusCode))
		return
	}

	// Stream to browser with attachment header.
	filename := song.Filename()
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}
	io.Copy(w, resp.Body)
}

// HandleNASStatus returns the NAS download configuration status.
// GET /api/nas/status
func (h *Handlers) HandleNASStatus(w http.ResponseWriter, r *http.Request) {
	enabled := h.mgr != nil && h.mgr.MusicDir() != ""
	data := map[string]any{
		"enabled": enabled,
	}
	if enabled {
		data["music_dir"] = h.mgr.MusicDir()
		data["concurrency"] = h.mgr.Concurrency()
	}
	writeOK(w, data)
}

// HandleNASDownload enqueues a single song for NAS download.
// POST /api/nas/download?source=X  body: Song JSON
func (h *Handlers) HandleNASDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if h.mgr == nil || h.mgr.MusicDir() == "" {
		writeError(w, http.StatusServiceUnavailable, "NAS download not configured (MUSIC_DIR not set)")
		return
	}

	pf, source, ok := h.resolveProvider(r)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if pf.GetDownloadURL == nil {
		writeError(w, http.StatusNotImplemented, fmt.Sprintf("download not supported for %s", source))
		return
	}

	song, err := decodeSong(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	taskID := h.mgr.Enqueue(song, source, pf.GetDownloadURL, pf.GetLyrics)
	writeOK(w, map[string]string{"task_id": taskID})
}

// HandleNASBatchDownload enqueues a playlist of songs for NAS download.
// POST /api/nas/download/batch?source=X  body: { "playlist_name": "...", "songs": [...] }
func (h *Handlers) HandleNASBatchDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if h.mgr == nil || h.mgr.MusicDir() == "" {
		writeError(w, http.StatusServiceUnavailable, "NAS download not configured (MUSIC_DIR not set)")
		return
	}

	pf, source, ok := h.resolveProvider(r)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if pf.GetDownloadURL == nil {
		writeError(w, http.StatusNotImplemented, fmt.Sprintf("download not supported for %s", source))
		return
	}

	var body struct {
		PlaylistName string       `json:"playlist_name"`
		Songs        []model.Song `json:"songs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if len(body.Songs) == 0 {
		writeError(w, http.StatusBadRequest, "no songs provided")
		return
	}

	batchID := h.mgr.EnqueueBatch(body.Songs, body.PlaylistName, source, pf.GetDownloadURL, pf.GetLyrics)
	writeOK(w, map[string]any{
		"batch_id":   batchID,
		"task_count": len(body.Songs),
	})
}

// HandleListTasks returns all download tasks.
// GET /api/nas/tasks
func (h *Handlers) HandleListTasks(w http.ResponseWriter, r *http.Request) {
	if h.mgr == nil {
		writeOK(w, []*Task{})
		return
	}
	writeOK(w, h.mgr.ListTasks())
}

// HandleGetTask returns a single task by ID.
// GET /api/nas/task?id=X
func (h *Handlers) HandleGetTask(w http.ResponseWriter, r *http.Request) {
	if h.mgr == nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id parameter")
		return
	}
	task, ok := h.mgr.GetTask(id)
	if !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeOK(w, task)
}

// HandleListBatches returns batch summaries.
// GET /api/nas/batches
func (h *Handlers) HandleListBatches(w http.ResponseWriter, r *http.Request) {
	if h.mgr == nil {
		writeOK(w, []BatchInfo{})
		return
	}
	writeOK(w, h.mgr.ListBatches())
}
