package bilibili

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/utils"
)

const (
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36 Edg/121.0.0.0"
	Referer   = "https://www.bilibili.com/"
	// 如果需要更高音质，请在此更新有效的 SESSDATA
	Cookie    = "buvid3=2E109C72-251F-3827-FA8E-921FA0D7EC5291319infoc; SESSDATA=your_sessdata;"
)

func Search(keyword string) ([]model.Song, error) {
	params := url.Values{}
	params.Set("search_type", "video")
	params.Set("keyword", keyword)
	params.Set("page", "1")
	params.Set("page_size", "10")

	searchURL := "https://api.bilibili.com/x/web-interface/search/type?" + params.Encode()
	body, err := utils.Get(searchURL, utils.WithHeader("User-Agent", UserAgent), utils.WithHeader("Referer", Referer), utils.WithHeader("Cookie", Cookie))
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
	for _, item := range searchResp.Data.Result {
		rootTitle := strings.ReplaceAll(strings.ReplaceAll(item.Title, "<em class=\"keyword\">", ""), "</em>", "")

		viewURL := fmt.Sprintf("https://api.bilibili.com/x/web-interface/view?bvid=%s", item.BVID)
		viewBody, err := utils.Get(viewURL, utils.WithHeader("User-Agent", UserAgent), utils.WithHeader("Cookie", Cookie))
		if err != nil {
			continue
		}

		var viewResp struct {
			Data struct {
				Pages []struct {
					CID      int64  `json:"cid"`
					Part     string `json:"part"`
					Duration int    `json:"duration"`
				} `json:"pages"`
			} `json:"data"`
		}
		if err := json.Unmarshal(viewBody, &viewResp); err != nil {
			continue
		}

		for _, page := range viewResp.Data.Pages {
			// [核心修改] 调用 PlayURL 获取带宽以计算 Size
			playURL := fmt.Sprintf("https://api.bilibili.com/x/player/playurl?fnval=16&bvid=%s&cid=%d", item.BVID, page.CID)
			playBody, err := utils.Get(playURL, utils.WithHeader("User-Agent", UserAgent), utils.WithHeader("Referer", Referer), utils.WithHeader("Cookie", Cookie))
			
			var estimatedSize int64 = 0
			actualDuration := page.Duration

			if err == nil {
				var playResp struct {
					Data struct {
						Dash struct {
							Audio []DashStream `json:"audio"`
							Flac  struct {
								Audio []DashStream `json:"audio"`
							} `json:"flac"`
							Duration int `json:"duration"` // DASH 提供的时长通常更准
						} `json:"dash"`
					} `json:"data"`
				}
				if json.Unmarshal(playBody, &playResp) == nil {
					if playResp.Data.Dash.Duration > 0 {
						actualDuration = playResp.Data.Dash.Duration
					}
					// 寻找最高带宽
					var bestBandwidth int = 0
					if len(playResp.Data.Dash.Flac.Audio) > 0 {
						bestBandwidth = playResp.Data.Dash.Flac.Audio[0].Bandwidth
					} else if len(playResp.Data.Dash.Audio) > 0 {
						// 已经按带宽排序取第一个
						bestBandwidth = playResp.Data.Dash.Audio[0].Bandwidth
					}
					// [计算公式] Size = Bandwidth(bps) * Duration(s) / 8
					if bestBandwidth > 0 {
						estimatedSize = int64(bestBandwidth) * int64(actualDuration) / 8
					}
				}
			}

			displayTitle := page.Part
			if len(viewResp.Data.Pages) == 1 && displayTitle == "" {
				displayTitle = rootTitle
			} else if displayTitle != rootTitle {
				displayTitle = fmt.Sprintf("%s - %s", rootTitle, displayTitle)
			}

			cover := item.Pic
			if strings.HasPrefix(cover, "//") { cover = "https:" + cover }

			songs = append(songs, model.Song{
				Source:   "bilibili",
				ID:       fmt.Sprintf("%s|%d", item.BVID, page.CID),
				Name:     displayTitle,
				Artist:   item.Author,
				Album:    item.BVID,
				Duration: actualDuration,
				Size:     estimatedSize, // 现在有了计算后的 Size
				Cover:    cover,
			})
		}
	}
	return songs, nil
}

// GetDownloadURL 保持之前的 Durl 兜底逻辑不变
func GetDownloadURL(s *model.Song) (string, error) {
	// ... (代码同上一版，包含 DASH 和 Durl 逻辑) ...
	parts := strings.Split(s.ID, "|")
	if len(parts) != 2 { return "", errors.New("invalid id") }
	apiURL := fmt.Sprintf("https://api.bilibili.com/x/player/playurl?fnval=16&bvid=%s&cid=%s", parts[0], parts[1])

	body, err := utils.Get(apiURL, utils.WithHeader("User-Agent", UserAgent), utils.WithHeader("Referer", Referer), utils.WithHeader("Cookie", Cookie))
	if err != nil { return "", err }

	var resp struct {
		Data struct {
			Durl []struct { URL string `json:"url"` } `json:"durl"`
			Dash struct {
				Audio []DashStream `json:"audio"`
				Flac  struct { Audio []DashStream `json:"audio"` } `json:"flac"`
			} `json:"dash"`
		} `json:"data"`
	}
	json.Unmarshal(body, &resp)

	// 优先 DASH
	if len(resp.Data.Dash.Flac.Audio) > 0 { return resp.Data.Dash.Flac.Audio[0].BaseURL, nil }
	if len(resp.Data.Dash.Audio) > 0 { return resp.Data.Dash.Audio[0].BaseURL, nil }
	// 兜底 Durl
	if len(resp.Data.Durl) > 0 { return resp.Data.Durl[0].URL, nil }

	return "", errors.New("no audio found")
}

type DashStream struct {
	BaseURL   string `json:"baseUrl"`
	Bandwidth int    `json:"bandwidth"`
}

// GetLyrics 获取歌词 (B站暂不支持歌词接口)
func GetLyrics(s *model.Song) (string, error) {
	if s.Source != "bilibili" {
		return "", errors.New("source mismatch")
	}
	// B站歌词接口暂未实现
	return "", nil
}
