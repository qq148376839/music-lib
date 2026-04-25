package api

import (
	"github.com/gin-gonic/gin"
	"github.com/guohuiyuan/music-lib/download"
	"github.com/guohuiyuan/music-lib/login"
	"gorm.io/gorm"
)

// NewRouter creates and configures the Gin engine with all routes registered.
// providers maps source name to its ProviderFuncs.
// netease and qq expose login status / logout for their respective platforms.
func NewRouter(
	providers map[string]ProviderFuncs,
	loginMgr *login.Manager,
	dlMgr *download.Manager,
	db *gorm.DB,
	netease PlatformAuth,
	qq PlatformAuth,
) *gin.Engine {
	engine := gin.New()

	engine.Use(
		Recovery(),
		SlogLogger(),
		CORS(),
	)

	srv := &Server{
		providers: providers,
		loginMgr:  loginMgr,
		dlMgr:     dlMgr,
		db:        db,
		netease:   netease,
		qq:        qq,
	}

	// Health + provider info
	engine.GET("/health", srv.handleHealth)
	engine.GET("/providers", srv.handleProviders)

	// Song APIs
	engine.GET("/api/search", srv.handleSearch)
	engine.POST("/api/lyrics", srv.handleLyrics)
	engine.GET("/api/parse", srv.handleParse)

	// Playlist APIs
	engine.GET("/api/playlist/search", srv.handlePlaylistSearch)
	engine.GET("/api/playlist/songs", srv.handlePlaylistSongs)
	engine.GET("/api/playlist/parse", srv.handlePlaylistParse)
	engine.GET("/api/playlist/recommended", srv.handlePlaylistRecommended)

	// Login APIs
	engine.POST("/api/login/qr/start", srv.handleLoginQRStart)
	engine.GET("/api/login/qr/poll", srv.handleLoginQRPoll)
	engine.POST("/api/login/cookie", srv.handleLoginCookie)
	engine.GET("/api/login/status", srv.handleLoginStatus)
	engine.POST("/api/login/logout", srv.handleLoginLogout)

	// Download / NAS APIs
	engine.POST("/api/download/file", srv.handleProxyDownload)
	engine.GET("/api/nas/status", srv.handleNASStatus)
	engine.POST("/api/nas/download", srv.handleNASDownload)
	engine.POST("/api/nas/download/batch", srv.handleNASBatchDownload)
	engine.POST("/api/nas/download/upgrade", srv.handleNASUpgrade)
	engine.GET("/api/nas/tasks", srv.handleListTasks)
	engine.GET("/api/nas/task", srv.handleGetTask)
	engine.GET("/api/nas/batches", srv.handleListBatches)

	// Chart / Monitor APIs
	engine.GET("/api/charts", srv.handleGetCharts)
	engine.GET("/api/monitors", srv.handleListMonitors)
	engine.POST("/api/monitors", srv.handleCreateMonitor)
	engine.PUT("/api/monitors/:id", srv.handleUpdateMonitor)
	engine.DELETE("/api/monitors/:id", srv.handleDeleteMonitor)
	engine.GET("/api/monitors/:id/runs", srv.handleListMonitorRuns)
	engine.POST("/api/monitors/:id/trigger", srv.handleTriggerMonitor)

	// Static files (must be registered last to avoid shadowing API routes)
	registerStaticFiles(engine)

	return engine
}
