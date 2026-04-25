package kugou

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/utils"
)

// Known Kugou chart rank IDs.
var kugouCharts = []model.Chart{
	{ID: "6666", Name: "飙升榜"},
	{ID: "8888", Name: "热歌榜"},
}

// GetCharts returns the list of supported Kugou charts.
func GetCharts() ([]model.Chart, error) { return defaultKugou.GetCharts() }

// GetChartSongs returns the top N songs from a Kugou chart.
func GetChartSongs(chartID string, limit int) ([]model.Song, error) {
	return defaultKugou.GetChartSongs(chartID, limit)
}

func (k *Kugou) GetCharts() ([]model.Chart, error) {
	return kugouCharts, nil
}

func (k *Kugou) GetChartSongs(chartID string, limit int) ([]model.Song, error) {
	if limit <= 0 {
		limit = 100
	}

	apiURL := fmt.Sprintf(
		"http://mobilecdnbj.kugou.com/api/v3/rank/song?rankid=%s&page=1&pagesize=%d&area_code=1",
		chartID, limit,
	)

	body, err := utils.Get(apiURL,
		utils.WithHeader("User-Agent", MobileUserAgent),
		utils.WithHeader("Cookie", k.cookie),
	)
	if err != nil {
		return nil, fmt.Errorf("kugou chart request: %w", err)
	}

	var resp struct {
		Data struct {
			Info []struct {
				Hash       string `json:"hash"`
				FileName   string `json:"filename"`
				Duration   int    `json:"duration"`
				FileSize   int64  `json:"filesize"`
				AlbumName  string `json:"album_name"`
				Remark     string `json:"remark"`
				SingerName string `json:"singername"`
				SongName   string `json:"songname"`
				TransParam struct {
					UnionCover string `json:"union_cover"`
				} `json:"trans_param"`
			} `json:"info"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kugou chart json: %w", err)
	}

	var songs []model.Song
	for _, item := range resp.Data.Info {
		name := item.SongName
		artist := item.SingerName
		if name == "" || artist == "" {
			parts := strings.Split(item.FileName, " - ")
			if len(parts) >= 2 {
				artist = strings.TrimSpace(parts[0])
				name = strings.TrimSpace(strings.Join(parts[1:], " - "))
			} else {
				name = item.FileName
			}
		}

		cover := ""
		if item.TransParam.UnionCover != "" {
			cover = strings.Replace(item.TransParam.UnionCover, "{size}", "240", 1)
		}

		album := item.AlbumName
		if album == "" {
			album = item.Remark
		}

		songs = append(songs, model.Song{
			Source:   "kugou",
			ID:       item.Hash,
			Name:     name,
			Artist:   artist,
			Album:    album,
			Duration: item.Duration,
			Size:     item.FileSize,
			Cover:    cover,
			Link:     fmt.Sprintf("https://www.kugou.com/song/#hash=%s", item.Hash),
			Extra: map[string]string{
				"hash": item.Hash,
			},
		})
	}

	return songs, nil
}
