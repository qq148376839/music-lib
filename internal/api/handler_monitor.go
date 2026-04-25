package api

import (
	"net/http"
	"strconv"

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
		Name     string `json:"name" binding:"required"`
		Platform string `json:"platform" binding:"required"`
		ChartID  string `json:"chart_id" binding:"required"`
		TopN     int    `json:"top_n"`
		Interval int    `json:"interval"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate platform has chart support.
	if _, ok := s.providers[body.Platform]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown platform"})
		return
	}

	m := &store.Monitor{
		Name:     body.Name,
		Platform: body.Platform,
		ChartID:  body.ChartID,
		TopN:     body.TopN,
		Interval: body.Interval,
		Enabled:  true,
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
