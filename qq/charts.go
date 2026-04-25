package qq

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/utils"
)

// Known QQ Music chart top IDs.
var qqCharts = []model.Chart{
	{ID: "26", Name: "热歌榜"},
	{ID: "27", Name: "新歌榜"},
	{ID: "4", Name: "巅峰榜·流行指数"},
}

// GetCharts returns the list of supported QQ charts.
func GetCharts() ([]model.Chart, error) { return getDefault().GetCharts() }

// GetChartSongs returns the top N songs from a QQ chart.
func GetChartSongs(chartID string, limit int) ([]model.Song, error) {
	return getDefault().GetChartSongs(chartID, limit)
}

func (q *QQ) GetCharts() ([]model.Chart, error) {
	return qqCharts, nil
}

func (q *QQ) GetChartSongs(chartID string, limit int) ([]model.Song, error) {
	if limit <= 0 {
		limit = 100
	}

	params := url.Values{}
	params.Set("topid", chartID)
	params.Set("song_begin", "0")
	params.Set("song_num", fmt.Sprintf("%d", limit))
	params.Set("type", "top")
	params.Set("page", "detail")
	params.Set("tpl", "3")
	params.Set("format", "json")
	params.Set("g_tk", "5381")
	params.Set("loginUin", "0")
	params.Set("hostUin", "0")
	params.Set("inCharset", "utf8")
	params.Set("outCharset", "utf-8")
	params.Set("notice", "0")
	params.Set("platform", "yqq")
	params.Set("needNewCode", "0")

	apiURL := "https://c.y.qq.com/v8/fcg-bin/fcg_v8_toplist_cp.fcg?" + params.Encode()

	body, err := utils.Get(apiURL,
		utils.WithHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"),
		utils.WithHeader("Referer", "https://y.qq.com/"),
		utils.WithHeader("Cookie", q.cookie),
	)
	if err != nil {
		return nil, fmt.Errorf("qq chart request: %w", err)
	}

	// Handle JSONP wrapper.
	sBody := string(body)
	if idx := strings.Index(sBody, "("); idx >= 0 && strings.HasSuffix(strings.TrimSpace(sBody), ")") {
		sBody = sBody[idx+1 : len(sBody)-1]
		body = []byte(sBody)
	}

	var resp struct {
		Songlist []struct {
			Data struct {
				SongID    int64  `json:"songid"`
				SongName  string `json:"songname"`
				SongMID   string `json:"songmid"`
				AlbumName string `json:"albumname"`
				AlbumMID  string `json:"albummid"`
				Interval  int    `json:"interval"`
				Size128   int64  `json:"size128"`
				Size320   int64  `json:"size320"`
				SizeFlac  int64  `json:"sizeflac"`
				Singer    []struct {
					Name string `json:"name"`
				} `json:"singer"`
			} `json:"data"`
		} `json:"songlist"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("qq chart json: %w", err)
	}

	var songs []model.Song
	for _, item := range resp.Songlist {
		d := item.Data

		var artists []string
		for _, s := range d.Singer {
			artists = append(artists, s.Name)
		}

		var cover string
		if d.AlbumMID != "" {
			cover = fmt.Sprintf("https://y.gtimg.cn/music/photo_new/T002R300x300M000%s.jpg", d.AlbumMID)
		}

		fileSize := d.Size128
		bitrate := 128
		if d.SizeFlac > 0 {
			fileSize = d.SizeFlac
			if d.Interval > 0 {
				bitrate = int(fileSize * 8 / 1000 / int64(d.Interval))
			} else {
				bitrate = 800
			}
		} else if d.Size320 > 0 {
			fileSize = d.Size320
			bitrate = 320
		}

		songs = append(songs, model.Song{
			Source:   "qq",
			ID:       d.SongMID,
			Name:     d.SongName,
			Artist:   strings.Join(artists, "、"),
			Album:    d.AlbumName,
			Duration: d.Interval,
			Size:     fileSize,
			Bitrate:  bitrate,
			Cover:    cover,
			Link:     fmt.Sprintf("https://y.qq.com/n/ryqq/songDetail/%s", d.SongMID),
			Extra: map[string]string{
				"songmid": d.SongMID,
			},
		})
	}

	return songs, nil
}
