package main

import (
	"github.com/guohuiyuan/music-lib/bilibili"
	"github.com/guohuiyuan/music-lib/fivesing"
	"github.com/guohuiyuan/music-lib/internal/api"
	"github.com/guohuiyuan/music-lib/jamendo"
	"github.com/guohuiyuan/music-lib/joox"
	"github.com/guohuiyuan/music-lib/kugou"
	"github.com/guohuiyuan/music-lib/kuwo"
	"github.com/guohuiyuan/music-lib/migu"
	"github.com/guohuiyuan/music-lib/netease"
	"github.com/guohuiyuan/music-lib/qianqian"
	"github.com/guohuiyuan/music-lib/qq"
	"github.com/guohuiyuan/music-lib/soda"
)

// buildProviders returns all registered music providers.
func buildProviders() map[string]api.ProviderFuncs {
	return map[string]api.ProviderFuncs{
		"netease": {
			Search:           netease.Search,
			GetDownloadURL:   netease.GetDownloadURL,
			GetLyrics:        netease.GetLyrics,
			Parse:            netease.Parse,
			SearchPlaylist:   netease.SearchPlaylist,
			GetPlaylistSongs: netease.GetPlaylistSongs,
			ParsePlaylist:    netease.ParsePlaylist,
			GetRecommended:   netease.GetRecommendedPlaylists,
			GetCharts:        netease.GetCharts,
			GetChartSongs:    netease.GetChartSongs,
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
			GetCharts:        qq.GetCharts,
			GetChartSongs:    qq.GetChartSongs,
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
			GetCharts:        kugou.GetCharts,
			GetChartSongs:    kugou.GetChartSongs,
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
