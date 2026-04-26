package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/guohuiyuan/music-lib/internal/monitor"
	"github.com/guohuiyuan/music-lib/internal/store"
)

// monScheduler is set by the router init to allow handlers to trigger runs.
var monScheduler *monitor.Scheduler

// SetMonitorScheduler wires the scheduler for manual trigger support.
func SetMonitorScheduler(s *monitor.Scheduler) {
	monScheduler = s
}

// platformForURL guesses the platform from the URL's hostname.
// Returns empty string if unrecognized.
func platformForURL(rawURL string) string {
	lower := strings.ToLower(rawURL)
	switch {
	case strings.Contains(lower, "music.163.com"):
		return "netease"
	case strings.Contains(lower, "y.qq.com"), strings.Contains(lower, "music.qq.com"):
		return "qq"
	case strings.Contains(lower, "kugou.com"):
		return "kugou"
	case strings.Contains(lower, "kuwo.cn"):
		return "kuwo"
	case strings.Contains(lower, "5sing.kugou.com"), strings.Contains(lower, "5sing.com"):
		return "fivesing"
	case strings.Contains(lower, "bilibili.com"):
		return "bilibili"
	case strings.Contains(lower, "music.migu.cn"), strings.Contains(lower, "soda."):
		return "soda"
	}
	return ""
}

// POST /api/monitors/resolve
func (s *Server) handleResolvePlaylist(c *gin.Context) {
	var body struct {
		URL string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}

	// Guess the platform from the URL to avoid trying every provider.
	guessedPlatform := platformForURL(body.URL)

	// Try providers that have ParsePlaylist. If we guessed a platform, try that first.
	type tryResult struct {
		platform string
		pf       ProviderFuncs
	}
	var candidates []tryResult
	if guessedPlatform != "" {
		if pf, ok := s.providers[guessedPlatform]; ok && pf.ParsePlaylist != nil {
			candidates = append(candidates, tryResult{guessedPlatform, pf})
		}
	}
	// Also add remaining providers as fallback (in case the guess was wrong).
	for name, pf := range s.providers {
		if name == guessedPlatform {
			continue
		}
		if pf.ParsePlaylist != nil {
			candidates = append(candidates, tryResult{name, pf})
		}
	}

	if len(candidates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported platform for URL"})
		return
	}

	for _, cand := range candidates {
		playlist, songs, err := cand.pf.ParsePlaylist(body.URL)
		if err != nil {
			continue
		}
		c.JSON(http.StatusOK, gin.H{
			"platform":    cand.platform,
			"playlist_id": playlist.ID,
			"name":        playlist.Name,
			"track_count": len(songs),
			"cover":       playlist.Cover,
			"creator":     playlist.Creator,
		})
		return
	}

	c.JSON(http.StatusBadGateway, gin.H{"error": "failed to resolve playlist: all providers failed"})
}

// GET /api/charts?platform=X
func (s *Server) handleGetCharts(c *gin.Context) {
	pf, _, ok := s.getProvider(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing or invalid source"})
		return
	}
	if pf.GetCharts == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "platform does not support charts"})
		return
	}
	charts, err := pf.GetCharts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": charts})
}

// GET /api/monitors
func (s *Server) handleListMonitors(c *gin.Context) {
	if s.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
		return
	}
	monitors, err := store.ListMonitors(s.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": monitors})
}

// POST /api/monitors
func (s *Server) handleCreateMonitor(c *gin.Context) {
	if s.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
		return
	}

	var body struct {
		Name      string `json:"name" binding:"required"`
		Platform  string `json:"platform" binding:"required"`
		ChartID   string `json:"chart_id" binding:"required"`
		TopN      int    `json:"top_n"`
		Interval  int    `json:"interval"`
		Type      string `json:"type"`
		SourceURL string `json:"source_url"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if body.Type == "" {
		body.Type = "chart"
	}

	// playlist type requires source_url.
	if body.Type == "playlist" && body.SourceURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_url is required for playlist type"})
		return
	}

	// Validate platform exists.
	if _, ok := s.providers[body.Platform]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown platform"})
		return
	}

	m := &store.Monitor{
		Name:      body.Name,
		Platform:  body.Platform,
		ChartID:   body.ChartID,
		TopN:      body.TopN,
		Interval:  body.Interval,
		Enabled:   true,
		Type:      body.Type,
		SourceURL: body.SourceURL,
	}
	if err := store.CreateMonitor(s.db, m); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": m})
}

// PUT /api/monitors/:id
func (s *Server) handleUpdateMonitor(c *gin.Context) {
	if s.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	m, err := store.GetMonitor(s.db, uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "monitor not found"})
		return
	}

	var body struct {
		Name     *string `json:"name"`
		TopN     *int    `json:"top_n"`
		Interval *int    `json:"interval"`
		Enabled  *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if body.Name != nil {
		m.Name = *body.Name
	}
	if body.TopN != nil {
		m.TopN = *body.TopN
	}
	if body.Interval != nil {
		m.Interval = *body.Interval
	}
	if body.Enabled != nil {
		m.Enabled = *body.Enabled
	}

	if err := store.UpdateMonitor(s.db, m); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": m})
}

// DELETE /api/monitors/:id
func (s *Server) handleDeleteMonitor(c *gin.Context) {
	if s.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := store.DeleteMonitor(s.db, uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0})
}

// GET /api/monitors/:id/runs
func (s *Server) handleListMonitorRuns(c *gin.Context) {
	if s.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	runs, err := store.ListMonitorRuns(s.db, uint(id), 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": runs})
}

// POST /api/monitors/:id/trigger
func (s *Server) handleTriggerMonitor(c *gin.Context) {
	if s.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	// Verify monitor exists.
	if _, err := store.GetMonitor(s.db, uint(id)); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "monitor not found"})
		return
	}

	if monScheduler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "scheduler not available"})
		return
	}

	// Execute asynchronously.
	go monScheduler.Execute(uint(id))

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "triggered"})
}
