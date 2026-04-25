package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GET /api/playlist/search?source=X&keyword=Y
func (s *Server) handlePlaylistSearch(c *gin.Context) {
	p, source, ok := s.getProvider(c)
	if !ok {
		writeError(c, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if p.SearchPlaylist == nil {
		writeError(c, http.StatusNotImplemented, fmt.Sprintf("playlist search not supported for %s", source))
		return
	}
	keyword := c.Query("keyword")
	if keyword == "" {
		writeError(c, http.StatusBadRequest, "missing keyword parameter")
		return
	}
	playlists, err := p.SearchPlaylist(keyword)
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, playlists)
}

// GET /api/playlist/songs?source=X&id=Y
func (s *Server) handlePlaylistSongs(c *gin.Context) {
	p, source, ok := s.getProvider(c)
	if !ok {
		writeError(c, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if p.GetPlaylistSongs == nil {
		writeError(c, http.StatusNotImplemented, fmt.Sprintf("playlist songs not supported for %s", source))
		return
	}
	id := c.Query("id")
	if id == "" {
		writeError(c, http.StatusBadRequest, "missing id parameter")
		return
	}
	songs, err := p.GetPlaylistSongs(id)
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, songs)
}

// GET /api/playlist/parse?source=X&link=Y
func (s *Server) handlePlaylistParse(c *gin.Context) {
	p, source, ok := s.getProvider(c)
	if !ok {
		writeError(c, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if p.ParsePlaylist == nil {
		writeError(c, http.StatusNotImplemented, fmt.Sprintf("playlist parse not supported for %s", source))
		return
	}
	link := c.Query("link")
	if link == "" {
		writeError(c, http.StatusBadRequest, "missing link parameter")
		return
	}
	playlist, songs, err := p.ParsePlaylist(link)
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, map[string]any{
		"playlist": playlist,
		"songs":    songs,
	})
}

// GET /api/playlist/recommended?source=X
func (s *Server) handlePlaylistRecommended(c *gin.Context) {
	p, source, ok := s.getProvider(c)
	if !ok {
		writeError(c, http.StatusBadRequest, fmt.Sprintf("unknown or missing source: %q", source))
		return
	}
	if p.GetRecommended == nil {
		writeError(c, http.StatusNotImplemented, fmt.Sprintf("playlist recommended not supported for %s", source))
		return
	}
	playlists, err := p.GetRecommended()
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, playlists)
}
