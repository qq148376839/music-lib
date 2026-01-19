package qq

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"time"

	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/utils"
)

const (
	// 对应 Python config.get("ios_useragent")
	UserAgent = "Mozilla/5.0 (iPhone; CPU iPhone OS 9_1 like Mac OS X) AppleWebKit/601.1.46 (KHTML, like Gecko) Version/9.0 Mobile/13B143 Safari/601.1"
	
	// 搜索 Referer
	SearchReferer = "http://m.y.qq.com"
	// 下载 Referer
	DownloadReferer = "http://y.qq.com"
)

// Search 搜索歌曲
func Search(keyword string) ([]model.Song, error) {
	// 1. 构造参数
	params := url.Values{}
	params.Set("w", keyword)
	params.Set("format", "json")
	params.Set("p", "1")
	params.Set("n", "10")

	apiURL := "http://c.y.qq.com/soso/fcgi-bin/search_for_qq_cp?" + params.Encode()

	// 2. 发送请求
	body, err := utils.Get(apiURL,
		utils.WithHeader("User-Agent", UserAgent),
		utils.WithHeader("Referer", SearchReferer),
	)
	if err != nil {
		return nil, err
	}

	// 3. 解析 JSON (根据提供的 JSON 重新映射字段)
	var resp struct {
		Data struct {
			Song struct {
				List []struct {
					SongID    int64  `json:"songid"`
					SongName  string `json:"songname"`
					SongMID   string `json:"songmid"`
					AlbumName string `json:"albumname"`
					AlbumMID  string `json:"albummid"`
					Interval  int    `json:"interval"` // 时长(秒)
					
					// 不同音质的大小
					Size128  int64 `json:"size128"`
					Size320  int64 `json:"size320"`
					SizeFlac int64 `json:"sizeflac"`
					
					Singer []struct {
						Name string `json:"name"`
					} `json:"singer"`

					// 支付/权限信息 (更新字段名)
					Pay struct {
						PayDownload   int `json:"paydownload"`   // 1 表示需要付费下载
						PayPlay       int `json:"payplay"`       // 1 表示需要付费播放
						PayTrackPrice int `json:"paytrackprice"` // 单曲价格
					} `json:"pay"`
				} `json:"list"`
			} `json:"song"`
		} `json:"data"`
	}

	// 调试时可以打印 body 查看真实返回
	fmt.Println(string(body))
	if err := json.Unmarshal(body, &resp); err != nil {

		
		return nil, fmt.Errorf("qq json parse error: %w", err)
	}
			
	// 4. 转换模型
	var songs []model.Song
	fmt.Println(resp.Data.Song.List)
	for _, item := range resp.Data.Song.List {
		// --- 核心过滤逻辑 ---
		// 过滤 VIP 和 付费歌曲 (payplay==1 表示需要付费播放)
		if item.Pay.PayPlay == 1 {
			continue
		}

		// 拼接歌手名
		var artistNames []string
		for _, s := range item.Singer {
			artistNames = append(artistNames, s.Name)
		}

		// 构造封面 URL
		var coverURL string
		if item.AlbumMID != "" {
			coverURL = fmt.Sprintf("https://y.gtimg.cn/music/photo_new/T002R300x300M000%s.jpg", item.AlbumMID)
		}

		// 下载逻辑目前主要尝试 128k (M500)，所以展示 size128 可能更准确
		fileSize := item.Size128
		// 如果需要下载 FLAC，可以用 item.SizeFlac

		songs = append(songs, model.Song{
			Source:   "qq",
			ID:       item.SongMID,
			Name:     item.SongName,
			Artist:   strings.Join(artistNames, "、"),
			Album:    item.AlbumName,
			Duration: item.Interval,
			Size:     fileSize,
			Cover:    coverURL,
		})
	}

	return songs, nil
}

// GetDownloadURL 获取下载链接
func GetDownloadURL(s *model.Song) (string, error) {
	if s.Source != "qq" {
		return "", errors.New("source mismatch")
	}

	// 1. 生成随机 GUID
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	guid := fmt.Sprintf("%d", r.Int63n(9000000000)+1000000000)

	// 2. 定义音质列表
	// 优先尝试 128k MP3，如果失败尝试 M4A
	type Rate struct {
		Prefix string
		Ext    string
	}
	rates := []Rate{
		{"M500", "mp3"}, // 128kbps
		{"C400", "m4a"}, // m4a
	}

	var lastErr string

	// 3. 循环尝试获取播放地址
	for _, rate := range rates {
		// 构造文件名: 前缀 + SongMID + SongMID + 后缀
		filename := fmt.Sprintf("%s%s%s.%s", rate.Prefix, s.ID, s.ID, rate.Ext)

		reqData := map[string]interface{}{
			"comm": map[string]interface{}{
				"cv":                4747474,
				"ct":                24,
				"format":            "json",
				"inCharset":         "utf-8",
				"outCharset":        "utf-8",
				"notice":            0,
				"platform":          "yqq.json",
				"needNewCode":       1,
				"uin":               0,
				"g_tk_new_20200303": 5381,
				"g_tk":              5381,
			},
			"req_1": map[string]interface{}{
				"module": "music.vkey.GetVkey",
				"method": "UrlGetVkey",
				"param": map[string]interface{}{
					"guid":      guid,
					"songmid":   []string{s.ID},
					"songtype":  []int{0},
					"uin":       "0",
					"loginflag": 1,
					"platform":  "20",
					"filename":  []string{filename},
				},
			},
		}

		jsonData, err := json.Marshal(reqData)
		if err != nil {
			continue
		}

		headers := []utils.RequestOption{
			utils.WithHeader("User-Agent", UserAgent),
			utils.WithHeader("Referer", DownloadReferer),
			utils.WithHeader("Content-Type", "application/json"),
		}
		
		body, err := utils.Post("https://u.y.qq.com/cgi-bin/musicu.fcg", bytes.NewReader(jsonData), headers...)
		if err != nil {
			lastErr = err.Error()
			continue
		}

		var result struct {
			Req1 struct {
				Data struct {
					MidUrlInfo []struct {
						Purl    string `json:"purl"`
						WifiUrl string `json:"wifiurl"`
						Result  int    `json:"result"`
						ErrMsg  string `json:"errtype"`
					} `json:"midurlinfo"`
				} `json:"data"`
			} `json:"req_1"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			lastErr = "json parse error"
			continue
		}

		if len(result.Req1.Data.MidUrlInfo) > 0 {
			info := result.Req1.Data.MidUrlInfo[0]
			
			// 核心修正：如果 purl 为空，不要立即返回错误，而是记录原因并 continue 尝试下一种音质
			if info.Purl == "" {
				// 记录错误信息用于最后兜底返回，如 "result: 104003"
				lastErr = fmt.Sprintf("empty purl (result code: %d)", info.Result)
				continue 
			}

			// 成功获取
			return "http://ws.stream.qqmusic.qq.com/" + info.Purl, nil
		}
	}

	// 所有音质都尝试失败
	return "", fmt.Errorf("download url not found: %s", lastErr)
}