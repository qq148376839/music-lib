package main

import (
	"strings"
	"testing"

	"github.com/guohuiyuan/music-lib/bilibili"
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

// PlatformTestSuite 定义每个平台的测试套件配置
type PlatformTestSuite struct {
	Name           string
	Keyword        string
	Search         func(string) ([]model.Song, error)
	GetDownloadURL func(*model.Song) (string, error)
	GetLyrics      func(*model.Song) (string, error)
	Parse          func(string) (*model.Song, error)
}

// TestPlatforms 统一测试所有平台的 Search, Download, Lyrics, Parse 流程
func TestPlatforms(t *testing.T) {
	defaultKeyword := "痛仰乐队"

	suites := []PlatformTestSuite{
		{
			Name:           "kugou",
			Keyword:        defaultKeyword,
			Search:         kugou.Search,
			GetDownloadURL: kugou.GetDownloadURL,
			GetLyrics:      kugou.GetLyrics,
			Parse:          kugou.Parse,
		},
		{
			Name:           "qq",
			Keyword:        defaultKeyword,
			Search:         qq.Search,
			GetDownloadURL: qq.GetDownloadURL,
			GetLyrics:      qq.GetLyrics,
			Parse:          qq.Parse,
		},
		{
			Name:           "netease",
			Keyword:        "再见杰克",
			Search:         netease.Search,
			GetDownloadURL: netease.GetDownloadURL,
			GetLyrics:      netease.GetLyrics,
			Parse:          netease.Parse,
		},
		{
			Name:           "kuwo",
			Keyword:        defaultKeyword,
			Search:         kuwo.Search,
			GetDownloadURL: kuwo.GetDownloadURL,
			GetLyrics:      kuwo.GetLyrics,
			Parse:          kuwo.Parse,
		},
		{
			Name:           "migu",
			Keyword:        defaultKeyword,
			Search:         migu.Search,
			GetDownloadURL: migu.GetDownloadURL,
			GetLyrics:      migu.GetLyrics,
			Parse:          nil, // Migu 未实现/导出 Parse
		},
		{
			Name:           "fivesing",
			Keyword:        "河图",
			Search:         fivesing.Search,
			GetDownloadURL: fivesing.GetDownloadURL,
			GetLyrics:      fivesing.GetLyrics,
			Parse:          fivesing.Parse,
		},
		{
			Name:           "qianqian",
			Keyword:        defaultKeyword,
			Search:         qianqian.Search,
			GetDownloadURL: qianqian.GetDownloadURL,
			GetLyrics:      qianqian.GetLyrics,
			Parse:          qianqian.Parse,
		},
		{
			Name:           "soda",
			Keyword:        defaultKeyword,
			Search:         soda.Search,
			GetDownloadURL: soda.GetDownloadURL,
			GetLyrics:      soda.GetLyrics,
			Parse:          nil, // Soda 未实现 Parse
		},
		{
			Name:           "jamendo",
			Keyword:        "acoustic",
			Search:         jamendo.Search,
			GetDownloadURL: jamendo.GetDownloadURL,
			GetLyrics:      jamendo.GetLyrics,
			Parse:          jamendo.Parse,
		},
		{
			Name:           "joox",
			Keyword:        defaultKeyword,
			Search:         joox.Search,
			GetDownloadURL: joox.GetDownloadURL,
			GetLyrics:      nil, // Joox 未实现/导出 GetLyrics
			Parse:          nil, // Joox 未实现/导出 Parse
		},
		{
			Name:           "bilibili",
			Keyword:        defaultKeyword,
			Search:         bilibili.Search,
			GetDownloadURL: bilibili.GetDownloadURL,
			GetLyrics:      nil, // Bilibili 歌词接口不稳定，暂时跳过
			Parse:          nil,
		},
	}

	for _, suite := range suites {
		suite := suite // 捕获循环变量
		t.Run(suite.Name, func(t *testing.T) {
			t.Parallel() // 启用并行测试

			// 1. Search (每个平台只调用一次)
			t.Logf("[%s] Starting Search...", suite.Name)
			songs, err := suite.Search(suite.Keyword)

			// 容错处理：对于不稳定或国外的源，允许搜索失败
			if err != nil {
				if suite.Name == "jamendo" || suite.Name == "bilibili" || suite.Name == "fivesing" {
					t.Skipf("[%s] Search failed (allowed): %v", suite.Name, err)
				}
				t.Fatalf("[%s] Search failed: %v", suite.Name, err)
			}
			if len(songs) == 0 {
				t.Skipf("[%s] Search returned no songs", suite.Name)
			}

			// 选取第一首结果进行后续测试
			song := &songs[0]
			t.Logf("[%s] Found song: %s - %s (ID: %s)", suite.Name, song.Name, song.Artist, song.ID)

			// 基础字段验证
			if song.Source != suite.Name {
				t.Errorf("Source mismatch: expected %s, got %s", suite.Name, song.Source)
			}
			if song.ID == "" {
				t.Error("ID is empty")
			}
			if song.Link == "" {
				t.Error("Link is empty")
			}
			if song.Extra == nil {
				t.Error("Extra is nil")
			}

			// 特殊验证：FiveSing ID 格式
			if suite.Name == "fivesing" && !strings.Contains(song.ID, "|") {
				t.Errorf("FiveSing ID format error: expected '|' separator, got %s", song.ID)
			}

			// 2. Test GetDownloadURL (复用 Search 结果)
			if suite.GetDownloadURL != nil {
				t.Run("GetDownloadURL", func(t *testing.T) {
					url, err := suite.GetDownloadURL(song)
					if err != nil {
						t.Logf("GetDownloadURL failed: %v", err)
						// 不做 Fatal，因为 URL 获取可能因版权/会员限制失败
					} else if url == "" {
						t.Error("Returned empty URL")
					} else {
						t.Logf("Got URL: %s...", url[:min(15, len(url))])
					}
				})
			}

			// 3. Test GetLyrics (复用 Search 结果)
			if suite.GetLyrics != nil {
				t.Run("GetLyrics", func(t *testing.T) {
					lyric, err := suite.GetLyrics(song)
					if err != nil {
						// 某些平台不支持或无歌词是正常的
						if suite.Name == "jamendo" {
							t.Logf("GetLyrics failed as expected (unsupported): %v", err)
						} else {
							t.Logf("GetLyrics failed: %v", err)
						}
					} else {
						if lyric == "" {
							t.Log("Lyrics empty")
						} else {
							t.Logf("Got lyrics (%d chars)", len(lyric))
						}
					}
				})
			}

			// 4. Test Parse (复用 Search 结果中的 Link)
			if suite.Parse != nil && song.Link != "" {
				t.Run("Parse", func(t *testing.T) {
					parsedSong, err := suite.Parse(song.Link)
					if err != nil {
						t.Errorf("Parse failed: %v", err)
					} else {
						if parsedSong.ID == "" {
							t.Error("Parsed song ID is empty")
						}
						if parsedSong.Name == "" {
							t.Error("Parsed song Name is empty")
						}
						if parsedSong.Source != suite.Name {
							t.Errorf("Parsed source mismatch: expected %s, got %s", suite.Name, parsedSong.Source)
						}
						t.Logf("Parse success: %s", parsedSong.Name)
					}
				})
			}
		})
	}
}

// TestLyricsSourceMismatch 测试歌词接口的源不匹配错误 (纯逻辑测试，无网络请求)
func TestLyricsSourceMismatch(t *testing.T) {
	wrongSong := &model.Song{
		Source: "wrong_source",
		ID:     "123",
		Name:   "Test Song",
		Artist: "Test Artist",
	}

	// 仅测试实现了 GetLyrics 的平台
	platforms := []struct {
		name      string
		getLyrics func(*model.Song) (string, error)
	}{
		{"netease", netease.GetLyrics},
		{"kuwo", kuwo.GetLyrics},
		{"soda", soda.GetLyrics},
		{"qq", qq.GetLyrics},
		{"kugou", kugou.GetLyrics},
		{"qianqian", qianqian.GetLyrics},
		{"migu", migu.GetLyrics},
		{"fivesing", fivesing.GetLyrics},
		{"jamendo", jamendo.GetLyrics},
	}

	for _, p := range platforms {
		t.Run(p.name, func(t *testing.T) {
			_, err := p.getLyrics(wrongSong)
			if err == nil {
				t.Errorf("%s GetLyrics should return error for source mismatch", p.name)
			} else if !strings.Contains(err.Error(), "source mismatch") {
				t.Errorf("%s GetLyrics error should contain 'source mismatch', got: %v", p.name, err)
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}