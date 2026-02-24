package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/guohuiyuan/music-lib/bilibili"
	"github.com/guohuiyuan/music-lib/download"
	"github.com/guohuiyuan/music-lib/fivesing"
	"github.com/guohuiyuan/music-lib/jamendo"
	"github.com/guohuiyuan/music-lib/joox"
	"github.com/guohuiyuan/music-lib/kugou"
	"github.com/guohuiyuan/music-lib/kuwo"
	"github.com/guohuiyuan/music-lib/migu"
	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/netease"
	"github.com/guohuiyuan/music-lib/qianqian"
	"github.com/guohuiyuan/music-lib/qq"
	"github.com/guohuiyuan/music-lib/soda"
)

// providerFuncs holds the function references for each provider.
type providerFuncs struct {
	Search             func(string) ([]model.Song, error)
	GetDownloadURL     func(*model.Song) (string, error)
	GetLyrics          func(*model.Song) (string, error)
	Parse              func(string) (*model.Song, error)
	SearchPlaylist     func(string) ([]model.Playlist, error)
	GetPlaylistSongs   func(string) ([]model.Song, error)
	ParsePlaylist      func(string) (*model.Playlist, []model.Song, error)
	GetRecommended     func() ([]model.Playlist, error)
}

var providers map[string]providerFuncs

func init() {
	providers = map[string]providerFuncs{
		"netease": {
			Search:           netease.Search,
			GetDownloadURL:   netease.GetDownloadURL,
			GetLyrics:        netease.GetLyrics,
			Parse:            netease.Parse,
			SearchPlaylist:   netease.SearchPlaylist,
			GetPlaylistSongs: netease.GetPlaylistSongs,
			ParsePlaylist:    netease.ParsePlaylist,
			GetRecommended:   netease.GetRecommendedPlaylists,
		},
		"qq": {
			Search:           qq.Search,
			GetDownloadURL:   qq.GetDownloadURL,
			GetLyrics:        qq.GetLyrics,
			Parse:            qq.Parse,
			SearchPlaylist:   qq.SearchPlaylist,
			GetPlaylistSongs: qq.GetPlaylistSongs,
			ParsePlaylist:    qq.ParsePlaylist,
			GetRecommended:   qq.GetRecommendedPlaylists,
		},
		"kugou": {
			Search:           kugou.Search,
			GetDownloadURL:   kugou.GetDownloadURL,
			GetLyrics:        kugou.GetLyrics,
			Parse:            kugou.Parse,
			SearchPlaylist:   kugou.SearchPlaylist,
			GetPlaylistSongs: kugou.GetPlaylistSongs,
			ParsePlaylist:    kugou.ParsePlaylist,
			GetRecommended:   kugou.GetRecommendedPlaylists,
		},
		"kuwo": {
			Search:           kuwo.Search,
			GetDownloadURL:   kuwo.GetDownloadURL,
			GetLyrics:        kuwo.GetLyrics,
			Parse:            kuwo.Parse,
			SearchPlaylist:   kuwo.SearchPlaylist,
			GetPlaylistSongs: kuwo.GetPlaylistSongs,
			ParsePlaylist:    kuwo.ParsePlaylist,
			GetRecommended:   kuwo.GetRecommendedPlaylists,
		},
		"migu": {
			Search:           migu.Search,
			GetDownloadURL:   migu.GetDownloadURL,
			GetLyrics:        migu.GetLyrics,
			Parse:            migu.Parse,
			SearchPlaylist:   migu.SearchPlaylist,
			GetPlaylistSongs: migu.GetPlaylistSongs,
		},
		"qianqian": {
			Search:           qianqian.Search,
			GetDownloadURL:   qianqian.GetDownloadURL,
			GetLyrics:        qianqian.GetLyrics,
			Parse:            qianqian.Parse,
			SearchPlaylist:   qianqian.SearchPlaylist,
			GetPlaylistSongs: qianqian.GetPlaylistSongs,
		},
		"soda": {
			Search:           soda.Search,
			GetDownloadURL:   soda.GetDownloadURL,
			GetLyrics:        soda.GetLyrics,
			Parse:            soda.Parse,
			SearchPlaylist:   soda.SearchPlaylist,
			GetPlaylistSongs: soda.GetPlaylistSongs,
			ParsePlaylist:    soda.ParsePlaylist,
		},
		"fivesing": {
			Search:           fivesing.Search,
			GetDownloadURL:   fivesing.GetDownloadURL,
			GetLyrics:        fivesing.GetLyrics,
			Parse:            fivesing.Parse,
			SearchPlaylist:   fivesing.SearchPlaylist,
			GetPlaylistSongs: fivesing.GetPlaylistSongs,
			ParsePlaylist:    fivesing.ParsePlaylist,
		},
		"jamendo": {
			Search:           jamendo.Search,
			GetDownloadURL:   jamendo.GetDownloadURL,
			GetLyrics:        jamendo.GetLyrics,
			Parse:            jamendo.Parse,
			SearchPlaylist:   jamendo.SearchPlaylist,
			GetPlaylistSongs: jamendo.GetPlaylistSongs,
		},
		"joox": {
			Search:           joox.Search,
			GetDownloadURL:   joox.GetDownloadURL,
			GetLyrics:        joox.GetLyrics,
			Parse:            joox.Parse,
			SearchPlaylist:   joox.SearchPlaylist,
			GetPlaylistSongs: joox.GetPlaylistSongs,
		},
		"bilibili": {
			Search:           bilibili.Search,
			GetDownloadURL:   bilibili.GetDownloadURL,
			GetLyrics:        bilibili.GetLyrics,
			Parse:            bilibili.Parse,
			SearchPlaylist:   bilibili.SearchPlaylist,
			GetPlaylistSongs: bilibili.GetPlaylistSongs,
			ParsePlaylist:    bilibili.ParsePlaylist,
		},
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "35280"
	}

	// --- Netease cookie persistence ---
	cfgDir := os.Getenv("CONFIG_DIR")
	if cfgDir == "" {
		cfgDir = "config"
	}
	netease.SetConfigDir(cfgDir)
	netease.LoadCookieFromDisk()
	if loggedIn, nickname := netease.GetLoginStatus(); loggedIn {
		log.Printf("netease cookie loaded (user: %s)", nickname)
	}

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", handleHealth)

	// List available providers
	mux.HandleFunc("/providers", handleProviders)

	// Song APIs
	mux.HandleFunc("/api/search", handleSearch)
	mux.HandleFunc("/api/lyrics", handleLyrics)
	mux.HandleFunc("/api/parse", handleParse)

	// Playlist APIs
	mux.HandleFunc("/api/playlist/search", handlePlaylistSearch)
	mux.HandleFunc("/api/playlist/songs", handlePlaylistSongs)
	mux.HandleFunc("/api/playlist/parse", handlePlaylistParse)
	mux.HandleFunc("/api/playlist/recommended", handlePlaylistRecommended)

	// --- Netease QR Login ---
	mux.HandleFunc("/api/netease/qr/key", handleNeteaseQRKey)
	mux.HandleFunc("/api/netease/qr/check", handleNeteaseQRCheck)
	mux.HandleFunc("/api/netease/login/status", handleNeteaseLoginStatus)
	mux.HandleFunc("/api/netease/logout", handleNeteaseLogout)

	// --- Download / NAS ---
	musicDir := os.Getenv("MUSIC_DIR")
	concurrency := 3
	if v := os.Getenv("DOWNLOAD_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			concurrency = n
		}
	}

	// Build provider funcs map for download handlers.
	dlProviders := make(map[string]download.ProviderFuncs)
	for name, p := range providers {
		dlProviders[name] = download.ProviderFuncs{
			Search:         p.Search,
			GetDownloadURL: p.GetDownloadURL,
			GetLyrics:      p.GetLyrics,
		}
	}

	var dlMgr *download.Manager
	if musicDir != "" {
		// Validate directory exists and is writable.
		if err := os.MkdirAll(musicDir, 0755); err != nil {
			log.Fatalf("MUSIC_DIR %q is not usable: %v", musicDir, err)
		}
		dlMgr = download.NewManager(musicDir, concurrency, dlProviders)
		log.Printf("NAS download enabled: dir=%s concurrency=%d", musicDir, concurrency)
	} else {
		log.Printf("NAS download disabled (MUSIC_DIR not set)")
	}

	dlHandlers := download.NewHandlers(dlMgr, dlProviders)

	mux.HandleFunc("/api/download/file", dlHandlers.HandleProxyDownload)
	mux.HandleFunc("/api/nas/status", dlHandlers.HandleNASStatus)
	mux.HandleFunc("/api/nas/download", dlHandlers.HandleNASDownload)
	mux.HandleFunc("/api/nas/download/batch", dlHandlers.HandleNASBatchDownload)
	mux.HandleFunc("/api/nas/tasks", dlHandlers.HandleListTasks)
	mux.HandleFunc("/api/nas/task", dlHandlers.HandleGetTask)
	mux.HandleFunc("/api/nas/batches", dlHandlers.HandleListBatches)

	// Serve frontend static files
	webDir := os.Getenv("WEB_DIR")
	if webDir == "" {
		webDir = "web"
	}
	if info, err := os.Stat(webDir); err == nil && info.IsDir() {
		fileServer := http.FileServer(http.Dir(webDir))
		mux.Handle("/", fileServer)
		log.Printf("serving frontend from %s", webDir)
	}

	log.Printf("music-lib API server starting on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// --- Response helpers ---

type apiResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

func writeOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: data})
}

func writeError(w http.ResponseWriter, httpCode int, msg string) {
	writeJSON(w, httpCode, apiResponse{Code: -1, Message: msg})
}

func getProvider(r *http.Request) (providerFuncs, string, bool) {
	source := r.URL.Query().Get("source")
	if source == "" {
		return providerFuncs{}, "", false
	}
	p, ok := providers[source]
	return p, source, ok
}

// --- Handlers ---

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeOK(w, map[string]string{"status": "ok"})
}

func handleProviders(w http.ResponseWriter, r *http.Request) {
	type providerInfo struct {
		Name             string `json:"name"`
		Search           bool   `json:"search"`
		Download         bool   `json:"download"`
		Lyrics           bool   `json:"lyrics"`
		Parse            bool   `json:"parse"`
		PlaylistSearch   bool   `json:"playlist_search"`
		PlaylistSongs    bool   `json:"playlist_songs"`
		PlaylistParse    bool   `json:"playlist_parse"`
		PlaylistRecommended bool `json:"playlist_recommended"`
	}
	var list []providerInfo
	for name, p := range providers {
		list = append(list, providerInfo{
			Name:             name,
			Search:           p.Search != nil,
			Download:         p.GetDownloadURL != nil,
			Lyrics:           p.GetLyrics != nil,
			Parse:            p.Parse != nil,
			PlaylistSearch:   p.SearchPlaylist != nil,
			PlaylistSongs:    p.GetPlaylistSongs != nil,
			PlaylistParse:    p.ParsePlaylist != nil,
			PlaylistRecommended: p.GetRecommended != nil,
		})
	}
	writeOK(w, list)
}

// GET /api/search?source=netease&keyword=周杰伦
func handleSearch(w http.ResponseWriter, r *http.Request) {
	p, source, ok := getProvider(r)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if p.Search == nil {
		writeError(w, http.StatusNotImplemented, fmt.Sprintf("search not supported for %s", source))
		return
	}
	keyword := r.URL.Query().Get("keyword")
	if keyword == "" {
		writeError(w, http.StatusBadRequest, "missing keyword parameter")
		return
	}
	songs, err := p.Search(keyword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, songs)
}

// POST /api/lyrics?source=netease  body: Song JSON
func handleLyrics(w http.ResponseWriter, r *http.Request) {
	p, source, ok := getProvider(r)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if p.GetLyrics == nil {
		writeError(w, http.StatusNotImplemented, fmt.Sprintf("lyrics not supported for %s", source))
		return
	}
	var song model.Song
	if err := json.NewDecoder(r.Body).Decode(&song); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	lyrics, err := p.GetLyrics(&song)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]string{"lyrics": lyrics})
}

// GET /api/parse?source=netease&link=https://...
func handleParse(w http.ResponseWriter, r *http.Request) {
	p, source, ok := getProvider(r)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if p.Parse == nil {
		writeError(w, http.StatusNotImplemented, fmt.Sprintf("parse not supported for %s", source))
		return
	}
	link := r.URL.Query().Get("link")
	if link == "" {
		writeError(w, http.StatusBadRequest, "missing link parameter")
		return
	}
	song, err := p.Parse(link)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, song)
}

// GET /api/playlist/search?source=netease&keyword=...
func handlePlaylistSearch(w http.ResponseWriter, r *http.Request) {
	p, source, ok := getProvider(r)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if p.SearchPlaylist == nil {
		writeError(w, http.StatusNotImplemented, fmt.Sprintf("playlist search not supported for %s", source))
		return
	}
	keyword := r.URL.Query().Get("keyword")
	if keyword == "" {
		writeError(w, http.StatusBadRequest, "missing keyword parameter")
		return
	}
	playlists, err := p.SearchPlaylist(keyword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, playlists)
}

// GET /api/playlist/songs?source=netease&id=123456
func handlePlaylistSongs(w http.ResponseWriter, r *http.Request) {
	p, source, ok := getProvider(r)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if p.GetPlaylistSongs == nil {
		writeError(w, http.StatusNotImplemented, fmt.Sprintf("playlist songs not supported for %s", source))
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id parameter")
		return
	}
	songs, err := p.GetPlaylistSongs(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, songs)
}

// GET /api/playlist/parse?source=netease&link=https://...
func handlePlaylistParse(w http.ResponseWriter, r *http.Request) {
	p, source, ok := getProvider(r)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if p.ParsePlaylist == nil {
		writeError(w, http.StatusNotImplemented, fmt.Sprintf("playlist parse not supported for %s", source))
		return
	}
	link := r.URL.Query().Get("link")
	if link == "" {
		writeError(w, http.StatusBadRequest, "missing link parameter")
		return
	}
	playlist, songs, err := p.ParsePlaylist(link)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]any{
		"playlist": playlist,
		"songs":    songs,
	})
}

// GET /api/playlist/recommended?source=netease
func handlePlaylistRecommended(w http.ResponseWriter, r *http.Request) {
	p, source, ok := getProvider(r)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if p.GetRecommended == nil {
		writeError(w, http.StatusNotImplemented, fmt.Sprintf("playlist recommended not supported for %s", source))
		return
	}
	playlists, err := p.GetRecommended()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, playlists)
}

// --- Netease QR Login Handlers ---

// GET /api/netease/qr/key
func handleNeteaseQRKey(w http.ResponseWriter, r *http.Request) {
	unikey, err := netease.GenerateQRKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	qrURL := "https://music.163.com/login?codekey=" + unikey
	writeOK(w, map[string]string{
		"unikey": unikey,
		"qr_url": qrURL,
	})
}

// GET /api/netease/qr/check?key=xxx
func handleNeteaseQRCheck(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing key parameter")
		return
	}
	code, _, nickname, err := netease.QRLoginStatus(key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]interface{}{
		"code":     code,
		"nickname": nickname,
	})
}

// GET /api/netease/login/status
func handleNeteaseLoginStatus(w http.ResponseWriter, r *http.Request) {
	loggedIn, nickname := netease.GetLoginStatus()
	writeOK(w, map[string]interface{}{
		"logged_in": loggedIn,
		"nickname":  nickname,
	})
}

// POST /api/netease/logout
func handleNeteaseLogout(w http.ResponseWriter, r *http.Request) {
	netease.Logout()
	writeOK(w, map[string]string{"status": "ok"})
}
