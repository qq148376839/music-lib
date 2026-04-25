package api

import (
	"github.com/gin-gonic/gin"
	"github.com/guohuiyuan/music-lib/download"
	"github.com/guohuiyuan/music-lib/login"
	"github.com/guohuiyuan/music-lib/model"
	"gorm.io/gorm"
)

// ProviderFuncs holds all capabilities for a single music provider.
type ProviderFuncs struct {
	Search           func(string) ([]model.Song, error)
	GetDownloadURL   func(*model.Song) (string, error)
	GetLyrics        func(*model.Song) (string, error)
	Parse            func(string) (*model.Song, error)
	SearchPlaylist   func(string) ([]model.Playlist, error)
	GetPlaylistSongs func(string) ([]model.Song, error)
	ParsePlaylist    func(string) (*model.Playlist, []model.Song, error)
	GetRecommended   func() ([]model.Playlist, error)
	GetCharts        func() ([]model.Chart, error)
	GetChartSongs    func(chartID string, limit int) ([]model.Song, error)
}

// PlatformAuth abstracts platform-specific login state queries.
type PlatformAuth interface {
	GetLoginStatus() (bool, string)
	Logout()
}

// Server holds all shared dependencies for the API handlers.
type Server struct {
	providers map[string]ProviderFuncs
	loginMgr  *login.Manager
	dlMgr     *download.Manager
	db        *gorm.DB
	netease   PlatformAuth
	qq        PlatformAuth
}

// getProvider resolves the "source" query parameter to a ProviderFuncs.
// Returns (provider, source, found).
func (s *Server) getProvider(c *gin.Context) (ProviderFuncs, string, bool) {
	source := c.Query("source")
	if source == "" {
		return ProviderFuncs{}, "", false
	}
	p, ok := s.providers[source]
	return p, source, ok
}
