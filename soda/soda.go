package soda

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/utils"
)

const (
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36"
)

// Search 搜索歌曲
// 对应 Python: _search 方法前半部分
func Search(keyword string) ([]model.Song, error) {
	// 1. 构造搜索参数
	params := url.Values{}
	params.Set("q", keyword)
	params.Set("cursor", "0")
	params.Set("search_method", "input")
	params.Set("aid", "386088") // 汽水音乐 Web AppID
	params.Set("device_platform", "web")
	params.Set("channel", "pc_web")

	apiURL := "https://api.qishui.com/luna/pc/search/track?" + params.Encode()

	// 2. 发送请求
	body, err := utils.Get(apiURL,
		utils.WithHeader("User-Agent", UserAgent),
	)
	if err != nil {
		return nil, err
	}
	fmt.Println(string(body))
	// 3. 解析 JSON
	var resp struct {
		ResultGroups []struct {
			Data []struct {
				Entity struct {
					Track struct {
						ID       string `json:"id"`
						Name     string `json:"name"`
						Duration int    `json:"duration"` // [新增] 时长(ms)
						Artists  []struct {
							Name string `json:"name"`
						} `json:"artists"`
						Album struct {
							Name     string `json:"name"`
							UrlCover struct {
								Urls []string `json:"urls"`
							} `json:"url_cover"` // [新增] 封面
						} `json:"album"`
						
						// [新增] 核心过滤字段：版权/VIP信息
						LabelInfo struct {
							OnlyVipDownload bool `json:"only_vip_download"`
						} `json:"label_info"`

						// [新增] 码率列表，用于计算大小
						BitRates []struct {
							Size int64 `json:"size"`
						} `json:"bit_rates"`
					} `json:"track"`
				} `json:"entity"`
			} `json:"data"`
		} `json:"result_groups"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("soda search json parse error: %w", err)
	}

	if len(resp.ResultGroups) == 0 {
		return nil, nil
	}

	// 4. 转换模型
	var songs []model.Song
	for _, item := range resp.ResultGroups[0].Data {
		track := item.Entity.Track

		// 拼接歌手
		var artistNames []string
		for _, ar := range track.Artists {
			artistNames = append(artistNames, ar.Name)
		}

		// [新增] 获取封面 (取第一个 URL)
		var cover string
		if len(track.Album.UrlCover.Urls) > 0 {
			cover = track.Album.UrlCover.Urls[0]
		}

		// [新增] 获取最大文件大小
		var maxSize int64
		for _, br := range track.BitRates {
			if br.Size > maxSize {
				maxSize = br.Size
			}
		}

		songs = append(songs, model.Song{
			Source:   "soda",
			ID:       track.ID,
			Name:     track.Name,
			Artist:   strings.Join(artistNames, "、"),
			Album:    track.Album.Name,
			Duration: track.Duration / 1000, // ms -> s
			Size:     maxSize,               // 填充文件大小
			Cover:    cover,                 // 填充封面
		})
	}

	return songs, nil
}

// DownloadInfo 包含下载所需的 URL 和 解密 Key
type DownloadInfo struct {
	URL      string // 加密的音频链接
	PlayAuth string // 解密 Key (Base64)
	Format   string // 文件格式 (m4a)
	Size     int64  // 文件大小
}

// GetDownloadInfo 获取下载信息 (URL + Auth)
func GetDownloadInfo(s *model.Song) (*DownloadInfo, error) {
	if s.Source != "soda" {
		return nil, errors.New("source mismatch")
	}

	// 1. 请求 Track V2 接口
	params := url.Values{}
	params.Set("track_id", s.ID)
	params.Set("media_type", "track")
	
	v2URL := "https://api.qishui.com/luna/pc/track_v2?" + params.Encode()
	
	v2Body, err := utils.Get(v2URL, utils.WithHeader("User-Agent", UserAgent))
	if err != nil {
		return nil, err
	}

	var v2Resp struct {
		TrackPlayer struct {
			URLPlayerInfo string `json:"url_player_info"`
		} `json:"track_player"`
	}
	
	if err := json.Unmarshal(v2Body, &v2Resp); err != nil {
		return nil, err
	}

	if v2Resp.TrackPlayer.URLPlayerInfo == "" {
		return nil, errors.New("player info url not found")
	}

	// 2. 请求 Player Info (获取实际播放地址)
	infoBody, err := utils.Get(v2Resp.TrackPlayer.URLPlayerInfo, utils.WithHeader("User-Agent", UserAgent))
	if err != nil {
		return nil, err
	}

	var infoResp struct {
		Result struct {
			Data struct {
				PlayInfoList []struct {
					MainPlayUrl   string `json:"MainPlayUrl"`
					BackupPlayUrl string `json:"BackupPlayUrl"`
					PlayAuth      string `json:"PlayAuth"`
					Size          int64  `json:"Size"`
					Bitrate       int    `json:"Bitrate"`
					Format        string `json:"Format"`
				} `json:"PlayInfoList"`
			} `json:"Data"`
		} `json:"Result"`
	}

	if err := json.Unmarshal(infoBody, &infoResp); err != nil {
		return nil, err
	}

	list := infoResp.Result.Data.PlayInfoList
	if len(list) == 0 {
		return nil, errors.New("no audio stream found")
	}

	// 3. 排序取最高音质
	sort.Slice(list, func(i, j int) bool {
		if list[i].Size != list[j].Size {
			return list[i].Size > list[j].Size
		}
		return list[i].Bitrate > list[j].Bitrate
	})

	best := list[0]
	downloadURL := best.MainPlayUrl
	if downloadURL == "" {
		downloadURL = best.BackupPlayUrl
	}

	if downloadURL == "" {
		return nil, errors.New("invalid download url")
	}

	return &DownloadInfo{
		URL:      downloadURL,
		PlayAuth: best.PlayAuth,
		Format:   best.Format,
		Size:     best.Size,
	}, nil
}

// GetDownloadURL 为了兼容 model.Song 接口，仅返回 URL
// 注意：这个 URL 下载的文件是加密的，需要配合 PlayAuth 解密
func GetDownloadURL(s *model.Song) (string, error) {
	info, err := GetDownloadInfo(s)
	if err != nil {
		return "", err
	}
	// 将 PlayAuth 附带在 URL fragment 中返回，供调用者解析（Hack 方式）
	return info.URL + "#auth=" + url.QueryEscape(info.PlayAuth), nil
}