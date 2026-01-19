package kuwo

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/utils"
)

const (
	// 对应 Python default_search_headers
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36"
)

// Search 搜索歌曲
// 对应 Python: _search 方法
func Search(keyword string) ([]model.Song, error) {
	// 1. 构造参数
	params := url.Values{}
	params.Set("vipver", "1")
	params.Set("client", "kt")
	params.Set("ft", "music")
	params.Set("cluster", "0")
	params.Set("strategy", "2012")
	params.Set("encoding", "utf8")
	params.Set("rformat", "json")
	params.Set("mobi", "1")
	params.Set("issubtitle", "1")
	params.Set("show_copyright_off", "1")
	params.Set("pn", "0")  // 页码
	params.Set("rn", "10") // 每页数量
	params.Set("all", keyword)

	apiURL := "http://www.kuwo.cn/search/searchMusicBykeyWord?" + params.Encode()

	// 2. 发送请求
	body, err := utils.Get(apiURL,
		utils.WithHeader("User-Agent", UserAgent),
	)
	if err != nil {
		return nil, err
	}

	// 3. 解析 JSON
	var resp struct {
		AbsList []struct {
			MusicRID string `json:"MUSICRID"`
			SongName string `json:"SONGNAME"`
			Artist   string `json:"ARTIST"`
			Album    string `json:"ALBUM"`
			Duration string `json:"DURATION"`
			HtsMVPic string `json:"hts_MVPIC"`
			MInfo    string `json:"MINFO"` 
			PayInfo  string `json:"PAY"`
			
			// [新增] 关键字段：资源开关
			BitSwitch int `json:"bitSwitch"` 
		} `json:"abslist"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kuwo json parse error: %w", err)
	}

	// 4. 转换模型
	var songs []model.Song
	for _, item := range resp.AbsList {
		// --- 核心过滤逻辑 ---
		
		// [新增] 过滤 BitSwitch 为 0 的无效歌曲
		if item.BitSwitch == 0 {
			continue
		}

		cleanID := strings.TrimPrefix(item.MusicRID, "MUSIC_")
		duration, _ := strconv.Atoi(item.Duration)

		// 解析文件大小 (使用上一轮的逻辑，匹配下载优先级)
		size := parseSizeFromMInfo(item.MInfo)

		songs = append(songs, model.Song{
			Source:   "kuwo",
			ID:       cleanID,
			Name:     item.SongName,
			Artist:   item.Artist,
			Album:    item.Album,
			Duration: duration,
			Size:     size,
			Cover:    item.HtsMVPic,
		})
	}

	return songs, nil
}

// 辅助函数：根据下载优先级解析大小 (保持不变)
func parseSizeFromMInfo(minfo string) int64 {
	if minfo == "" {
		return 0
	}

	type FormatInfo struct {
		Format  string
		Bitrate string
		Size    int64
	}
	var formats []FormatInfo

	parts := strings.Split(minfo, ";")
	for _, part := range parts {
		kv := make(map[string]string)
		attrs := strings.Split(part, ",")
		for _, attr := range attrs {
			pair := strings.Split(attr, ":")
			if len(pair) == 2 {
				kv[pair[0]] = pair[1]
			}
		}

		sizeStr := kv["size"]
		if sizeStr == "" {
			continue
		}
		sizeStr = strings.TrimSuffix(strings.ToLower(sizeStr), "mb")
		sizeMb, _ := strconv.ParseFloat(sizeStr, 64)
		sizeBytes := int64(sizeMb * 1024 * 1024)

		formats = append(formats, FormatInfo{
			Format:  kv["format"],
			Bitrate: kv["bitrate"],
			Size:    sizeBytes,
		})
	}

	// 优先级: 2000kflac > flac > 320kmp3 > 128kmp3
	for _, f := range formats {
		if f.Format == "flac" && f.Bitrate == "2000" { return f.Size }
	}
	for _, f := range formats {
		if f.Format == "flac" { return f.Size }
	}
	for _, f := range formats {
		if f.Format == "mp3" && f.Bitrate == "320" { return f.Size }
	}
	for _, f := range formats {
		if f.Format == "mp3" && f.Bitrate == "128" { return f.Size }
	}

	var maxSize int64
	for _, f := range formats {
		if f.Size > maxSize { maxSize = f.Size }
	}
	return maxSize
}

// GetDownloadURL 获取下载链接 (保持不变)
func GetDownloadURL(s *model.Song) (string, error) {
	if s.Source != "kuwo" {
		return "", errors.New("source mismatch")
	}

	qualities := []string{
		"2000kflac", 
		"flac",      
		"320kmp3",   
		"128kmp3",   
	}

	for _, br := range qualities {
		params := url.Values{}
		params.Set("f", "web")
		params.Set("source", "kwplayercar_ar_6.0.0.9_B_jiakong_vh.apk")
		params.Set("from", "PC")
		params.Set("type", "convert_url_with_sign")
		params.Set("br", br)
		params.Set("rid", s.ID)
		params.Set("user", "C_APK_guanwang_12609069939969033731")

		apiURL := "https://mobi.kuwo.cn/mobi.s?" + params.Encode()

		body, err := utils.Get(apiURL,
			utils.WithHeader("User-Agent", UserAgent),
		)
		if err != nil {
			continue
		}

		var resp struct {
			Data struct {
				URL     string `json:"url"`
				Bitrate int    `json:"bitrate"`
				Format  string `json:"format"`
			} `json:"data"`
		}

		if err := json.Unmarshal(body, &resp); err != nil {
			continue
		}

		if resp.Data.URL != "" {
			return resp.Data.URL, nil
		}
	}

	return "", errors.New("download url not found (copyright restricted)")
}