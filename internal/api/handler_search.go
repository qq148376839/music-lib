package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/guohuiyuan/music-lib/model"
)

// GET /health
func (s *Server) handleHealth(c *gin.Context) {
	writeOK(c, map[string]string{"status": "ok"})
}

// GET /providers
func (s *Server) handleProviders(c *gin.Context) {
	type providerInfo struct {
		Name                string `json:"name"`
		Search              bool   `json:"search"`
		Download            bool   `json:"download"`
		Lyrics              bool   `json:"lyrics"`
		Parse               bool   `json:"parse"`
		PlaylistSearch      bool   `json:"playlist_search"`
		PlaylistSongs       bool   `json:"playlist_songs"`
		PlaylistParse       bool   `json:"playlist_parse"`
		PlaylistRecommended bool   `json:"playlist_recommended"`
		Charts              bool   `json:"charts"`
		ChartSongs          bool   `json:"chart_songs"`
	}
	var list []providerInfo
	for name, p := range s.providers {
		list = append(list, providerInfo{
			Name:                name,
			Search:              p.Search != nil,
			Download:            p.GetDownloadURL != nil,
			Lyrics:              p.GetLyrics != nil,
			Parse:               p.Parse != nil,
			PlaylistSearch:      p.SearchPlaylist != nil,
			PlaylistSongs:       p.GetPlaylistSongs != nil,
			PlaylistParse:       p.ParsePlaylist != nil,
			PlaylistRecommended: p.GetRecommended != nil,
			Charts:              p.GetCharts != nil,
			ChartSongs:          p.GetChartSongs != nil,
		})
	}
	writeOK(c, list)
}

// GET /api/search?source=X&keyword=Y
func (s *Server) handleSearch(c *gin.Context) {
	p, source, ok := s.getProvider(c)
	if !ok {
		writeError(c, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if p.Search == nil {
		writeError(c, http.StatusNotImplemented, fmt.Sprintf("search not supported for %s", source))
		return
	}
	keyword := c.Query("keyword")
	if keyword == "" {
		writeError(c, http.StatusBadRequest, "missing keyword parameter")
		return
	}
	songs, err := p.Search(keyword)
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, songs)
}

// POST /api/lyrics?source=X  body: Song JSON
func (s *Server) handleLyrics(c *gin.Context) {
	p, source, ok := s.getProvider(c)
	if !ok {
		writeError(c, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if p.GetLyrics == nil {
		writeError(c, http.StatusNotImplemented, fmt.Sprintf("lyrics not supported for %s", source))
		return
	}
	var song model.Song
	if err := json.NewDecoder(c.Request.Body).Decode(&song); err != nil {
		writeError(c, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	lyrics, err := p.GetLyrics(&song)
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, map[string]string{"lyrics": lyrics})
}

// GET /api/parse?source=X&link=Y
func (s *Server) handleParse(c *gin.Context) {
	p, source, ok := s.getProvider(c)
	if !ok {
		writeError(c, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if p.Parse == nil {
		writeError(c, http.StatusNotImplemented, fmt.Sprintf("parse not supported for %s", source))
		return
	}
	link := c.Query("link")
	if link == "" {
		writeError(c, http.StatusBadRequest, "missing link parameter")
		return
	}
	song, err := p.Parse(link)
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, song)
}
