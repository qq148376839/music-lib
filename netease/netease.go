package netease

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
	Referer = "http://music.163.com/"
	// 搜索接口 (通过 linux forward 转发)
	SearchAPI = "http://music.163.com/api/linux/forward"
	// 下载接口 (WeApi)
	DownloadAPI = "http://music.163.com/weapi/song/enhance/player/url"
)

// Netease 结构体
type Netease struct {
	cookie string
}

// New 初始化函数
func New(cookie string) *Netease {
	return &Netease{
		cookie: cookie,
	}
}

// 全局默认实例（向后兼容）
var defaultNetease = New("")

// Search 搜索歌曲（向后兼容）
func Search(keyword string) ([]model.Song, error) {
	return defaultNetease.Search(keyword)
}

// GetDownloadURL 获取下载链接（向后兼容）
func GetDownloadURL(s *model.Song) (string, error) {
	return defaultNetease.GetDownloadURL(s)
}

// GetLyrics 获取歌词（向后兼容）
func GetLyrics(s *model.Song) (string, error) {
	return defaultNetease.GetLyrics(s)
}

// Search 搜索歌曲
// Python: netease_search
func (n *Netease) Search(keyword string) ([]model.Song, error) {
	// 1. 构造内部 eparams (将被 AES-ECB 加密)
	eparams := map[string]interface{}{
		"method": "POST",
		"url":    "http://music.163.com/api/cloudsearch/pc",
		"params": map[string]interface{}{
			"s":      keyword,
			"type":   1,
			"offset": 0,
			"limit":  10, // 默认 10 条
		},
	}
	eparamsJSON, err := json.Marshal(eparams)
	if err != nil {
		return nil, fmt.Errorf("json marshal error: %w", err)
	}

	// 2. 加密参数
	encryptedParam := EncryptLinux(string(eparamsJSON))

	// 3. 构造 POST 表单数据
	form := url.Values{}
	form.Set("eparams", encryptedParam)

	// 4. 发送请求
	headers := []utils.RequestOption{
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Content-Type", "application/x-www-form-urlencoded"),
		utils.WithHeader("Cookie", n.cookie),
	}

	body, err := utils.Post(SearchAPI, strings.NewReader(form.Encode()), headers...)
	if err != nil {
		return nil, err
	}

	// 5. 解析 JSON
	var resp struct {
		Result struct {
			Songs []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
				Ar   []struct {
					Name string `json:"name"`
				} `json:"ar"` // 歌手
				Al struct {
					Name   string `json:"name"`
					PicURL string `json:"picUrl"`
				} `json:"al"` // 专辑
				Dt        int `json:"dt"` // 时长 (ms)
				Privilege struct {
					Fl int `json:"fl"` // 版权标记
					Pl int `json:"pl"` // 播放等级
				} `json:"privilege"`
				// 不同音质信息，用于计算大小
				H struct {
					Size int64 `json:"size"`
				} `json:"h"`
				M struct {
					Size int64 `json:"size"`
				} `json:"m"`
				L struct {
					Size int64 `json:"size"`
				} `json:"l"`
			} `json:"songs"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("netease json parse error: %w", err)
	}

	// 6. 转换模型
	var songs []model.Song
	for _, item := range resp.Result.Songs {
		// --- 核心过滤逻辑 (参考 Python 代码) ---
		
		// 1. 过滤无版权 (fl == 0 通常表示无版权)
		if item.Privilege.Fl == 0 {
			continue
		}

		// 2. 计算文件大小 (模拟 Python 逻辑)
		// 优先获取高品质大小
		var size int64
		if item.Privilege.Fl >= 320000 && item.H.Size > 0 {
			size = item.H.Size
		} else if item.Privilege.Fl >= 192000 && item.M.Size > 0 {
			size = item.M.Size
		} else {
			size = item.L.Size
		}

		var artistNames []string
		for _, ar := range item.Ar {
			artistNames = append(artistNames, ar.Name)
		}

		songs = append(songs, model.Song{
			Source:   "netease",
			ID:       fmt.Sprintf("%d", item.ID),
			Name:     item.Name,
			Artist:   strings.Join(artistNames, "、"),
			Album:    item.Al.Name,
			Duration: item.Dt / 1000,
			Size:     size,
			Cover:    item.Al.PicURL,
		})
	}

	return songs, nil
}

// GetDownloadURL 获取下载链接
// Python: NeteaseSong.download
func (n *Netease) GetDownloadURL(s *model.Song) (string, error) {
	if s.Source != "netease" {
		return "", errors.New("source mismatch")
	}

	// 1. 构造原始参数
	reqData := map[string]interface{}{
		"ids": []string{s.ID}, // 注意 ID 要放在数组里
		"br":  320000,         // 320k 码率
	}
	reqJSON, err := json.Marshal(reqData)
	if err != nil {
		return "", fmt.Errorf("json marshal error: %w", err)
	}

	// 2. WeApi 加密 (AES-CBC + RSA)
	params, encSecKey := EncryptWeApi(string(reqJSON))

	// 3. 构造 POST 表单
	form := url.Values{}
	form.Set("params", params)
	form.Set("encSecKey", encSecKey)

	// 4. 发送请求
	headers := []utils.RequestOption{
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Content-Type", "application/x-www-form-urlencoded"),
		utils.WithHeader("Cookie", n.cookie),
	}

	body, err := utils.Post(DownloadAPI, strings.NewReader(form.Encode()), headers...)
	if err != nil {
		return "", err
	}

	// 5. 解析响应
	var resp struct {
		Data []struct {
			URL  string `json:"url"`
			Code int    `json:"code"`
			Br   int    `json:"br"` // 实际码率
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("json parse error: %w", err)
	}

	if len(resp.Data) == 0 || resp.Data[0].URL == "" {
		return "", errors.New("download url not found (might be vip or copyright restricted)")
	}

	return resp.Data[0].URL, nil
}

// GetLyrics 获取歌词
func (n *Netease) GetLyrics(s *model.Song) (string, error) {
	if s.Source != "netease" {
		return "", errors.New("source mismatch")
	}

	// 1. 构造参数
	reqData := map[string]interface{}{
		"csrf_token": "",
		"id":         s.ID, // model 中 ID 为 string，网易云接口通常兼容
		"lv":         -1,   // 版本号，-1 表示获取最新
		"tv":         -1,   // 翻译版本号
	}

	reqJSON, err := json.Marshal(reqData)
	if err != nil {
		return "", fmt.Errorf("json marshal error: %w", err)
	}

	// 2. WeApi 加密
	params, encSecKey := EncryptWeApi(string(reqJSON))

	// 3. 构造 POST 表单
	form := url.Values{}
	form.Set("params", params)
	form.Set("encSecKey", encSecKey)

	// 4. 发送请求
	headers := []utils.RequestOption{
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Content-Type", "application/x-www-form-urlencoded"),
		utils.WithHeader("Cookie", n.cookie),
	}

	// 接口地址
	lyricAPI := "https://music.163.com/weapi/song/lyric"

	body, err := utils.Post(lyricAPI, strings.NewReader(form.Encode()), headers...)
	if err != nil {
		return "", err
	}

	// 5. 解析响应
	var resp struct {
		Code int `json:"code"`
		Lrc  struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
		// 如果需要翻译歌词，可以解析 tlyric 字段
		// Tlyric struct {
		// 	Lyric string `json:"lyric"`
		// } `json:"tlyric"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("json parse error: %w", err)
	}

	if resp.Code != 200 {
		return "", fmt.Errorf("netease api error code: %d", resp.Code)
	}

	return resp.Lrc.Lyric, nil
}
