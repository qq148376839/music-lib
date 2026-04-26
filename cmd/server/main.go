package main

import (
	"log/slog"
	"os"
	"strconv"

	"github.com/guohuiyuan/music-lib/download"
	"github.com/guohuiyuan/music-lib/internal/api"
	"github.com/guohuiyuan/music-lib/internal/monitor"
	"github.com/guohuiyuan/music-lib/internal/store"
	"github.com/guohuiyuan/music-lib/login"
	"github.com/guohuiyuan/music-lib/netease"
	"github.com/guohuiyuan/music-lib/qq"
)

func main() {
	// 1. Read environment variables.
	port := envOr("PORT", "35280")
	musicDir := os.Getenv("MUSIC_DIR")
	dataDir := envOr("DATA_DIR", "data")
	concurrency := envInt("DOWNLOAD_CONCURRENCY", 3)
	maxRetries := envInt("DOWNLOAD_MAX_RETRIES", 3)
	retryBackoff := envInt("DOWNLOAD_RETRY_BACKOFF", 2)
	scrapeEnabled := envBool("SCRAPE_ENABLED", true)
	scrapeCover := envBool("SCRAPE_COVER", true)
	scrapeLyrics := envBool("SCRAPE_LYRICS", true)
	cfgDir := envOr("CONFIG_DIR", dataDir)

	// 2. Initialize slog (JSON handler, level from LOG_LEVEL).
	initSlog(os.Getenv("LOG_LEVEL"))

	// 3. Init DB.
	db, err := store.Init(dataDir)
	if err != nil {
		slog.Error("failed to init database", "error", err)
		os.Exit(1)
	}

	// 4. Mark interrupted tasks as failed.
	if err := store.MarkRunningAsFailed(db); err != nil {
		slog.Warn("mark running as failed", "error", err)
	}

	// 5. Load existing tasks.
	existingTasks, err := store.ListAllTasks(db)
	if err != nil {
		slog.Warn("load existing tasks", "error", err)
	}

	// 6. Load cookies.
	netease.SetConfigDir(cfgDir)
	netease.LoadCookieFromDisk()
	qq.SetConfigDir(cfgDir)
	qq.LoadCookieFromDisk()

	// 6b. Log data restoration summary.
	neteaseOK, _ := netease.GetLoginStatus()
	qqOK, _ := qq.GetLoginStatus()
	slog.Info("data restored",
		"tasks", len(existingTasks),
		"netease_cookie", neteaseOK,
		"qq_cookie", qqOK,
	)

	// 7. Login manager.
	qrProviders := map[string]login.QRProvider{
		"netease": netease.NewQRProvider(),
		"qq":      qq.NewQRProvider(),
	}
	loginMgr := login.NewManager(qrProviders, func(platform, cookies, nickname string) {
		switch platform {
		case "netease":
			netease.SetCookie(cookies)
			if err := netease.SaveCookieToDisk(cookies, nickname); err != nil {
				slog.Warn("save netease cookie", "error", err)
			}
			slog.Info("login.success", "platform", platform, "nickname", nickname)
		case "qq":
			qq.SetCookie(cookies)
			if err := qq.SaveCookieToDisk(cookies, nickname); err != nil {
				slog.Warn("save qq cookie", "error", err)
			}
			slog.Info("login.success", "platform", platform, "nickname", nickname)
		}
	})

	// 8. Build provider funcs map.
	providers := buildProviders()

	// 9. Build download provider funcs.
	dlProviders := make(map[string]download.ProviderFuncs)
	for name, p := range providers {
		dlProviders[name] = download.ProviderFuncs{
			Search:         p.Search,
			GetDownloadURL: p.GetDownloadURL,
			GetLyrics:      p.GetLyrics,
		}
	}

	// 10. Create download manager.
	dlCfg := download.Config{
		MusicDir:      musicDir,
		Concurrency:   concurrency,
		MaxRetries:    maxRetries,
		RetryBackoff:  retryBackoff,
		ScrapeEnabled: scrapeEnabled,
		ScrapeCover:   scrapeCover,
		ScrapeLyrics:  scrapeLyrics,
	}
	var dlMgr *download.Manager
	if musicDir != "" {
		if err := os.MkdirAll(musicDir, 0755); err != nil {
			slog.Error("MUSIC_DIR not usable", "dir", musicDir, "error", err)
			os.Exit(1)
		}
		dlMgr = download.NewManager(dlCfg, dlProviders)
		slog.Info("NAS download enabled", "dir", musicDir, "concurrency", concurrency)
	} else {
		slog.Warn("NAS download disabled (MUSIC_DIR not set)")
	}

	// 11. Wire persistence callback.
	if dlMgr != nil {
		dlMgr.SetOnTaskUpdate(func(t *download.Task) {
			if err := store.SaveTask(db, t); err != nil {
				slog.Warn("save task", "task_id", t.ID, "error", err)
			}
		})
		// 12. Restore history.
		dlMgr.LoadTasks(existingTasks)

		// 13. Restore batch names from DB.
		if batchNames, err := store.ListBatchNames(db); err != nil {
			slog.Warn("load batch names", "error", err)
		} else {
			dlMgr.LoadBatchNames(batchNames)
		}
	}

	// 14. Start chart monitor scheduler.
	if dlMgr != nil {
		chartProviders := make(map[string]monitor.ChartProvider)
		for name, p := range providers {
			if p.GetChartSongs != nil || p.GetPlaylistSongs != nil {
				chartProviders[name] = monitor.ChartProvider{
					GetChartSongs:    p.GetChartSongs,
					GetPlaylistSongs: p.GetPlaylistSongs,
					GetDownloadURL:   p.GetDownloadURL,
					GetLyrics:        p.GetLyrics,
				}
			}
		}
		scheduler := monitor.NewScheduler(db, dlMgr, chartProviders)
		scheduler.Start()
		api.SetMonitorScheduler(scheduler)
		monitorCount := 0
		if monitors, err := store.ListMonitors(db); err == nil {
			monitorCount = len(monitors)
		}
		slog.Info("chart monitor enabled", "platforms", len(chartProviders), "monitors", monitorCount)
	}

	// 15. Create router.
	router := api.NewRouter(
		providers,
		loginMgr,
		dlMgr,
		db,
		neteaseAuth{},
		qqAuth{},
	)

	slog.Info("server starting", "port", port, "music_dir", musicDir, "data_dir", dataDir)

	// 16. Run.
	if err := router.Run(":" + port); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// --- helpers ---

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v == "true" || v == "1"
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func initSlog(level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(h))
}

// neteaseAuth wraps the netease package-level functions as a PlatformAuth.
type neteaseAuth struct{}

func (neteaseAuth) GetLoginStatus() (bool, string) { return netease.GetLoginStatus() }
func (neteaseAuth) Logout()                         { netease.Logout() }

// qqAuth wraps the qq package-level functions as a PlatformAuth.
type qqAuth struct{}

func (qqAuth) GetLoginStatus() (bool, string) { return qq.GetLoginStatus() }
func (qqAuth) Logout()                         { qq.Logout() }
