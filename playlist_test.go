package main

import (
	"testing"
	"time"

	"github.com/guohuiyuan/music-lib/bilibili"
	"github.com/guohuiyuan/music-lib/fivesing"
	"github.com/guohuiyuan/music-lib/kugou"
	"github.com/guohuiyuan/music-lib/kuwo"
	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/netease"
	"github.com/guohuiyuan/music-lib/qq"
	"github.com/guohuiyuan/music-lib/soda"
)

// PlaylistTestSuite 定义歌单平台的测试套件配置
type PlaylistTestSuite struct {
	Name             string
	Keyword          string
	
	// 核心通用接口 (必须有)
	SearchPlaylist   func(string) ([]model.Playlist, error)
	GetPlaylistSongs func(string) ([]model.Song, error)
	ParsePlaylist    func(string) (*model.Playlist, []model.Song, error)

	// 可选特性接口 (某些源可能没有，允许为 nil)
	GetRecommended   func() ([]model.Playlist, error)
}

func TestPlaylistPlatforms(t *testing.T) {
	suites := []PlaylistTestSuite{
		{
			Name:             "netease",
			Keyword:          "热歌",
			SearchPlaylist:   netease.SearchPlaylist,
			GetPlaylistSongs: netease.GetPlaylistSongs,
			ParsePlaylist:    netease.ParsePlaylist,
			GetRecommended:   netease.GetRecommendedPlaylists, // 已配置推荐歌单接口
		},
		{
			Name:             "qq",
			Keyword:          "流行",
			SearchPlaylist:   qq.SearchPlaylist,
			GetPlaylistSongs: qq.GetPlaylistSongs,
			ParsePlaylist:    qq.ParsePlaylist,
		},
		{
			Name:             "kugou",
			Keyword:          "经典",
			SearchPlaylist:   kugou.SearchPlaylist,
			GetPlaylistSongs: kugou.GetPlaylistSongs,
			ParsePlaylist:    kugou.ParsePlaylist,
		},
		{
			Name:             "kuwo",
			Keyword:          "抖音",
			SearchPlaylist:   kuwo.SearchPlaylist,
			GetPlaylistSongs: kuwo.GetPlaylistSongs,
			ParsePlaylist:    kuwo.ParsePlaylist,
		},
		{
			Name:             "soda",
			Keyword:          "抖音",
			SearchPlaylist:   soda.SearchPlaylist,
			GetPlaylistSongs: soda.GetPlaylistSongs,
			ParsePlaylist:    soda.ParsePlaylist,
		},
		{
			Name:             "fivesing",
			Keyword:          "古风",
			SearchPlaylist:   fivesing.SearchPlaylist,
			GetPlaylistSongs: fivesing.GetPlaylistSongs,
			ParsePlaylist:    fivesing.ParsePlaylist,
		},
		{
			Name:             "bilibili",
			Keyword:          "合集",
			SearchPlaylist:   bilibili.SearchPlaylist,
			GetPlaylistSongs: bilibili.GetPlaylistSongs,
			ParsePlaylist:    bilibili.ParsePlaylist,
		},
	}

	for _, suite := range suites {
		suite := suite // 捕获变量
		t.Run(suite.Name, func(t *testing.T) {
			t.Parallel() // 并行测试以加快速度

			// -------------------------------------------------------
			// 1. 测试歌单搜索 (SearchPlaylist)
			// -------------------------------------------------------
			t.Logf("=== [%s] 1. Testing SearchPlaylist (Keyword: %s) ===", suite.Name, suite.Keyword)
			playlists, err := suite.SearchPlaylist(suite.Keyword)
			if err != nil {
				t.Logf("SearchPlaylist error (might be network issue): %v", err)
				return // 网络错误跳过后续步骤
			}
			if len(playlists) == 0 {
				t.Skip("No playlists found, skipping detail tests")
			}

			var first model.Playlist
			found := false
			for _, p := range playlists {
				if p.TrackCount > 0 {
					first = p
					found = true
					break
				}
			}
			if !found {
				first = playlists[0]
				t.Log("Warning: All playlists have 0 tracks, using the first one anyway")
			}

			t.Logf("Found Playlist: %s (ID: %s, Tracks: %d)", first.Name, first.ID, first.TrackCount)
			
			if first.ID == "" {
				t.Error("Playlist ID should not be empty")
			}

			// -------------------------------------------------------
			// 2. 测试获取歌单详情 (GetPlaylistSongs)
			// -------------------------------------------------------
			t.Logf("=== [%s] 2. Testing GetPlaylistSongs (ID: %s) ===", suite.Name, first.ID)
			songs, err := suite.GetPlaylistSongs(first.ID)
			if err != nil {
				t.Logf("GetPlaylistSongs failed: %v", err)
			} else {
				if len(songs) == 0 {
					t.Log("GetPlaylistSongs returned 0 songs")
				} else {
					t.Logf("Success! Retrieved %d songs.", len(songs))
					t.Logf("Sample Song: %s - %s (ID: %s)", songs[0].Name, songs[0].Artist, songs[0].ID)
				}
			}

			// -------------------------------------------------------
			// 3. 测试解析歌单链接 (ParsePlaylist)
			// -------------------------------------------------------
			if first.Link != "" && suite.ParsePlaylist != nil {
				time.Sleep(1 * time.Second) // 避免请求过快
				t.Logf("=== [%s] 3. Testing ParsePlaylist (Link: %s) ===", suite.Name, first.Link)
				parsedMeta, parsedSongs, err := suite.ParsePlaylist(first.Link)
				if err != nil {
					t.Logf("ParsePlaylist failed: %v", err)
				} else {
					if parsedMeta == nil {
						t.Error("ParsePlaylist returned nil metadata")
					} else {
						t.Logf("Parsed Meta: %s (ID: %s)", parsedMeta.Name, parsedMeta.ID)
					}
					t.Logf("Parsed Songs: %d", len(parsedSongs))
				}
			} else {
				t.Log("Link empty or ParsePlaylist func nil, skipping step 3")
			}

			// -------------------------------------------------------
			// 4. [新增] 测试推荐歌单 (可选特性)
			// -------------------------------------------------------
			if suite.GetRecommended != nil {
				t.Logf("=== [%s] 4. Testing GetRecommended (Optional Feature) ===", suite.Name)
				
				// 稍微延时避免并发过高被封
				time.Sleep(500 * time.Millisecond)

				recPlaylists, err := suite.GetRecommended()
				if err != nil {
					t.Errorf("GetRecommended failed: %v", err)
				} else {
					if len(recPlaylists) == 0 {
						t.Error("GetRecommended returned 0 playlists")
					} else {
						t.Logf("Success! Got %d recommended playlists", len(recPlaylists))
						
						// 验证第一个推荐歌单是否有效（尝试获取其中的歌曲）
						if len(recPlaylists) > 0 {
							firstRec := recPlaylists[0]
							t.Logf("Verifying first recommended: %s (ID: %s)", firstRec.Name, firstRec.ID)
							
							// 复用 GetPlaylistSongs 验证其有效性
							recSongs, err := suite.GetPlaylistSongs(firstRec.ID)
							if err != nil {
								t.Logf("Warning: Failed to fetch songs for recommended playlist: %v", err)
							} else {
								t.Logf("Verified! Contains %d songs.", len(recSongs))
							}
						}
					}
				}
			} else {
				t.Logf("=== [%s] 4. GetRecommended not configured, skipping ===", suite.Name)
			}
		})
	}
}