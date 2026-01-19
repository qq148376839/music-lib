package kuwo

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/utils"
)

const (
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36"
)

func init() {
	// 初始化随机数种子
	rand.Seed(time.Now().UnixNano())
}

// Search 搜索歌曲 (逻辑保持不变: 优先展示 128k 大小)
func Search(keyword string) ([]model.Song, error) {
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
	params.Set("pn", "0")
	params.Set("rn", "10")
	params.Set("all", keyword)

	apiURL := "http://www.kuwo.cn/search/searchMusicBykeyWord?" + params.Encode()

	body, err := utils.Get(apiURL,
		utils.WithHeader("User-Agent", UserAgent),
	)
	if err != nil {
		return nil, err
	}

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
			BitSwitch int `json:"bitSwitch"` 
		} `json:"abslist"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kuwo json parse error: %w", err)
	}

	var songs []model.Song
	for _, item := range resp.AbsList {
		if item.BitSwitch == 0 {
			continue
		}

		cleanID := strings.TrimPrefix(item.MusicRID, "MUSIC_")
		duration, _ := strconv.Atoi(item.Duration)
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

// 辅助函数 (保持不变)
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

	// 优先级: 128kmp3 > 320kmp3 > flac > 2000kflac
	for _, f := range formats {
		if f.Format == "mp3" && f.Bitrate == "128" { return f.Size }
	}
	for _, f := range formats {
		if f.Format == "mp3" && f.Bitrate == "320" { return f.Size }
	}
	for _, f := range formats {
		if f.Format == "flac" { return f.Size }
	}
	for _, f := range formats {
		if f.Format == "flac" && f.Bitrate == "2000" { return f.Size }
	}

	var maxSize int64
	for _, f := range formats {
		if f.Size > maxSize { maxSize = f.Size }
	}
	return maxSize
}

// GetDownloadURL 获取下载链接
func GetDownloadURL(s *model.Song) (string, error) {
	if s.Source != "kuwo" {
		return "", errors.New("source mismatch")
	}

	// 优先级: 优先尝试 128kmp3 (最稳定，无VIP限制)
	qualities := []string{
		"128kmp3",   
		"320kmp3",   
		"flac",      
		"2000kflac", 
	}

	// [新增] 生成随机 DeviceID / UserID
	// 格式模仿: C_APK_guanwang_ + 19位随机数字
	randomID := fmt.Sprintf("C_APK_guanwang_%d%d", time.Now().UnixNano(), rand.Intn(1000000))

	for _, br := range qualities {
		params := url.Values{}
		params.Set("f", "web")
		params.Set("source", "kwplayercar_ar_6.0.0.9_B_jiakong_vh.apk")
		params.Set("from", "PC")
		params.Set("type", "convert_url_with_sign")
		params.Set("br", br)
		params.Set("rid", s.ID)
		
		// [修改] 使用随机生成的 User ID，避免被服务器判定为"同一用户多下载"
		params.Set("user", randomID)

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

// GetLyric 获取歌词
func GetLyric(songID string) (string, error) {
	params := url.Values{}
	params.Set("musicId", songID)
	params.Set("httpsStatus", "1")

	// 酷我歌词接口
	apiURL := "http://m.kuwo.cn/newh5/singles/songinfoandlrc?" + params.Encode()
	body, err := utils.Get(apiURL, utils.WithHeader("User-Agent", UserAgent))
	if err != nil {
		return "", fmt.Errorf("failed to fetch kuwo lyric API: %w", err)
	}

	var resp struct {
		Data struct {
			Lrclist []struct {
				Time      string `json:"time"`      // 例如 "10.55" (秒)
				LineLyric string `json:"lineLyric"` // 歌词文本
			} `json:"lrclist"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to parse kuwo lyric JSON: %w", err)
	}

	if len(resp.Data.Lrclist) == 0 {
		return "", nil // 返回空字符串表示没有歌词
	}

	// 拼接成标准 LRC 格式: [mm:ss.xx]歌词
	var sb strings.Builder
	for _, line := range resp.Data.Lrclist {
		secs, _ := strconv.ParseFloat(line.Time, 64)
		m := int(secs) / 60
		s := int(secs) % 60
		ms := int((secs - float64(int(secs))) * 100)
		
		// 格式化为 [00:00.00]
		sb.WriteString(fmt.Sprintf("[%02d:%02d.%02d]%s\n", m, s, ms, line.LineLyric))
	}

	return sb.String(), nil
}
