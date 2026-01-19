package bilibili

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
	// 对应 Python default_search_headers
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36 Edg/121.0.0.0"
	Referer   = "https://www.bilibili.com/"
	Origin    = "https://www.bilibili.com"
	Accept    = "application/json, text/plain, */*"

	// 仿浏览器指纹 Header
	SecChUa         = "\"Not A(Brand\";v=\"99\", \"Microsoft Edge\";v=\"121\", \"Chromium\";v=\"121\""
	SecChUaMobile   = "?0"
	SecChUaPlatform = "\"Windows\""
	SecFetchDest    = "empty"
	SecFetchMode    = "cors"
	SecFetchSite    = "same-site"

	// Cookie (SESSDATA 是关键)
	Cookie = "buvid3=2E109C72-251F-3827-FA8E-921FA0D7EC5291319infoc; b_nut=1676213591; i-wanna-go-back=-1; _uuid=2B2D7A6C-8310C-1167-F548-2F1095A6E93F290252infoc; buvid4=31696B5F-BB23-8F2B-3310-8B3C55FB49D491966-023021222-WcoPnBbwgLUAZ6TJuAUN8Q%3D%3D; CURRENT_FNVAL=4048; DedeUserID=520271156; DedeUserID__ckMd5=66450f2302095cc5; nostalgia_conf=-1; rpdid=|(JY))RmR~|u0J'uY~YkuJ~Ru; buvid_fp_plain=undefined; b_ut=5; hit-dyn-v2=1; LIVE_BUVID=AUTO8716766313471956; hit-new-style-dyn=1; CURRENT_PID=418c8490-cadb-11ed-b23b-dd640f2e1c14; FEED_LIVE_VERSION=V8; header_theme_version=CLOSE; CURRENT_QUALITY=80; enable_web_push=DISABLE; buvid_fp=52ad4773acad74caefdb23875d5217cd; PVID=1; home_feed_column=5; SESSDATA=8036f42c%2C1719895843%2C19675%2A12CjATThdxG8TyQ2panBpBQcmT0gDKjexwc-zXNGiMnIQ2I9oLVmOiE9YkLao2_aawEhoSVlhGY05PVjVkZWM0T042Z2hZRXBOdElYWXhJa3RpVmZ0M3NvcWw1N0tPcGRVSmRoOVNQZnNHT1JHS05yR1Y1MUFLX3RXeXVJa3NjbEVBQkUxRVN6RFRRIIEC; bili_jct=4c583b61b86b16d812a7804078828688; sid=8dt1ioao; bili_ticket=eyJhbGciOiJIUzI1NiIsImtpZCI6InMwMyIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3MDQ2MjUzNjAsImlhdCI6MTcwNDM2NjEwMCwicGx0IjotMX0.4E-V4K2y452cy6eexwY2x_q3-xgcNF2qtugddiuF8d4; bili_ticket_expires=1704625300; fingerprint=847f1839b443252d91ff0df7465fa8d9; browser_resolution=1912-924; bp_video_offset_520271156=883089613008142344"
)

// Search 搜索 B 站视频并展开为分 P 音频
func Search(keyword string) ([]model.Song, error) {
	// 1. 构造搜索请求
	params := url.Values{}
	params.Set("search_type", "video")
	params.Set("keyword", keyword)
	params.Set("page", "1")
	params.Set("page_size", "10")

	searchURL := "https://api.bilibili.com/x/web-interface/search/type?" + params.Encode()

	// 2. 发送搜索请求
	body, err := utils.Get(searchURL,
		utils.WithHeader("User-Agent", UserAgent),
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Cookie", Cookie),
		utils.WithHeader("Sec-Ch-Ua", SecChUa),
		utils.WithHeader("Sec-Ch-Ua-Mobile", SecChUaMobile),
		utils.WithHeader("Sec-Ch-Ua-Platform", SecChUaPlatform),
		utils.WithHeader("Sec-Fetch-Dest", SecFetchDest),
		utils.WithHeader("Sec-Fetch-Mode", SecFetchMode),
		utils.WithHeader("Sec-Fetch-Site", SecFetchSite),
	)
	if err != nil {
		return nil, err
	}

	var searchResp struct {
		Data struct {
			Result []struct {
				BVID   string `json:"bvid"`
				Title  string `json:"title"`
				Author string `json:"author"`
				Pic    string `json:"pic"`
			} `json:"result"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("bilibili search json error: %w", err)
	}

	var songs []model.Song

	// 4. 遍历每个视频，获取分 P 详情 (View API)
	for _, item := range searchResp.Data.Result {
		rootTitle := strings.ReplaceAll(strings.ReplaceAll(item.Title, "<em class=\"keyword\">", ""), "</em>", "")

		// View API: 获取分P信息
		viewURL := fmt.Sprintf("https://api.bilibili.com/x/web-interface/view?bvid=%s", item.BVID)
		viewBody, err := utils.Get(viewURL,
			utils.WithHeader("User-Agent", UserAgent),
			utils.WithHeader("Cookie", Cookie),
		)
		if err != nil {
			continue
		}

		var viewResp struct {
			Data struct {
				Pages []struct {
					CID      int64  `json:"cid"`
					Part     string `json:"part"`
					Duration int    `json:"duration"` // 秒
				} `json:"pages"`
			} `json:"data"`
		}
		
		if err := json.Unmarshal(viewBody, &viewResp); err != nil {
			continue
		}

		// 5. 展开分 P 并获取大小 (PlayURL API)
		for _, page := range viewResp.Data.Pages {
			// [新增] 每一P都调用 PlayURL 获取真实流信息 (为了拿到 Size)
			playURL := fmt.Sprintf("https://api.bilibili.com/x/player/playurl?fnval=16&bvid=%s&cid=%d", item.BVID, page.CID)
			playBody, err := utils.Get(playURL,
				utils.WithHeader("User-Agent", UserAgent),
				utils.WithHeader("Referer", Referer),
				utils.WithHeader("Cookie", Cookie),
			)
			
			var size int64 = 0
			var duration int = page.Duration

			if err == nil {
				var playResp struct {
					Data struct {
						Dash struct {
							Audio []DashStream `json:"audio"`
							Flac  struct {
								Audio []DashStream `json:"audio"`
							} `json:"flac"`
							Dolby struct {
								Audio []DashStream `json:"audio"`
							} `json:"dolby"`
							Duration int `json:"duration"` // DASH 里的时长可能更精确
						} `json:"dash"`
					} `json:"data"`
				}
				
				if json.Unmarshal(playBody, &playResp) == nil {
					// 汇总所有音频流
					var candidates []DashStream
					if len(playResp.Data.Dash.Flac.Audio) > 0 {
						candidates = append(candidates, playResp.Data.Dash.Flac.Audio...)
					}
					if len(playResp.Data.Dash.Dolby.Audio) > 0 {
						candidates = append(candidates, playResp.Data.Dash.Dolby.Audio...)
					}
					if len(playResp.Data.Dash.Audio) > 0 {
						candidates = append(candidates, playResp.Data.Dash.Audio...)
					}

					// 选取最佳音质 (Bandwidth 最大)
					if len(candidates) > 0 {
						sort.Slice(candidates, func(i, j int) bool {
							return candidates[i].Bandwidth > candidates[j].Bandwidth
						})
						best := candidates[0]
						
						// [核心] 估算大小: Bandwidth (bps) * Duration (s) / 8
						// 注意：Bandwidth 是 bit per second
						if best.Bandwidth > 0 && page.Duration > 0 {
							size = int64(best.Bandwidth) * int64(page.Duration) / 8
						}
					}
					// 如果 DASH 返回了时长，也可以用它
					if playResp.Data.Dash.Duration > 0 {
						duration = playResp.Data.Dash.Duration
					}
				}
			}

			// 构造复合ID
			compoundID := fmt.Sprintf("%s|%d", item.BVID, page.CID)

			// 标题处理
			displayTitle := page.Part
			if len(viewResp.Data.Pages) == 1 && displayTitle == "" {
				displayTitle = rootTitle
			} else if displayTitle != rootTitle {
				displayTitle = fmt.Sprintf("%s - %s", rootTitle, displayTitle)
			}

			// 封面处理 (补全 https)
			cover := item.Pic
			if strings.HasPrefix(cover, "//") {
				cover = "https:" + cover
			}

			songs = append(songs, model.Song{
				Source:   "bilibili",
				ID:       compoundID,
				Name:     displayTitle,
				Artist:   item.Author,
				Album:    item.BVID,
				Duration: duration,
				Size:     size,  // [新增] 填充估算大小
				Cover:    cover, // [确认] 填充封面
			})
		}
	}

	return songs, nil
}

// GetDownloadURL 获取音频流链接
func GetDownloadURL(s *model.Song) (string, error) {
	if s.Source != "bilibili" {
		return "", errors.New("source mismatch")
	}

	parts := strings.Split(s.ID, "|")
	if len(parts) != 2 {
		return "", errors.New("invalid bilibili id format")
	}
	bvid := parts[0]
	cid := parts[1]

	// 构造 PlayURL 请求 (fnval=16 请求 DASH)
	apiURL := fmt.Sprintf("https://api.bilibili.com/x/player/playurl?fnval=16&bvid=%s&cid=%s", bvid, cid)

	// 发送请求 (必须带 Cookie 才能获取高音质)
	body, err := utils.Get(apiURL,
		utils.WithHeader("User-Agent", UserAgent),
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Cookie", Cookie), 
		utils.WithHeader("Sec-Ch-Ua", SecChUa),
		utils.WithHeader("Sec-Ch-Ua-Mobile", SecChUaMobile),
		utils.WithHeader("Sec-Ch-Ua-Platform", SecChUaPlatform),
	)
	if err != nil {
		return "", err
	}

	var resp struct {
		Data struct {
			Dash struct {
				Audio []DashStream `json:"audio"`
				Flac struct {
					Audio []DashStream `json:"audio"`
				} `json:"flac"`
				Dolby struct {
					Audio []DashStream `json:"audio"`
				} `json:"dolby"`
			} `json:"dash"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("playurl json error: %w", err)
	}

	// 优先级选择 (Flac > Dolby > Audio)
	var candidates []DashStream
	
	if len(resp.Data.Dash.Flac.Audio) > 0 {
		candidates = append(candidates, resp.Data.Dash.Flac.Audio...)
	}
	if len(resp.Data.Dash.Dolby.Audio) > 0 {
		candidates = append(candidates, resp.Data.Dash.Dolby.Audio...)
	}
	if len(resp.Data.Dash.Audio) > 0 {
		candidates = append(candidates, resp.Data.Dash.Audio...)
	}

	if len(candidates) == 0 {
		return "", errors.New("no audio stream found")
	}

	// 排序取最佳 (按 bandwidth 降序)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Bandwidth > candidates[j].Bandwidth
	})

	bestStream := candidates[0]
	
	if bestStream.BaseURL != "" {
		return bestStream.BaseURL, nil
	}
	if len(bestStream.BackupURL) > 0 {
		return bestStream.BackupURL[0], nil
	}

	return "", errors.New("no valid url in stream")
}

// DashStream 辅助结构体
type DashStream struct {
	ID        int      `json:"id"`
	BaseURL   string   `json:"baseUrl"`
	BackupURL []string `json:"backupUrl"`
	Bandwidth int      `json:"bandwidth"`
	MimeType  string   `json:"mimeType"`
}