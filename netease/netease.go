package netease

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/utils"
)

const (
	Referer                = "http://music.163.com/"
	SearchAPI              = "http://music.163.com/api/linux/forward"
	DownloadAPI            = "http://music.163.com/weapi/song/enhance/player/url"
	DetailAPI              = "https://music.163.com/weapi/v3/song/detail"
	PlaylistAPI            = "https://music.163.com/weapi/v3/playlist/detail"
	RecommendedPlaylistAPI = "https://music.163.com/weapi/personalized/playlist" // 新增：推荐歌单API
)

type Netease struct {
	cookie string
}

func New(cookie string) *Netease { return &Netease{cookie: cookie} }

var defaultNetease = New("")

func Search(keyword string) ([]model.Song, error) { return defaultNetease.Search(keyword) }
func SearchPlaylist(keyword string) ([]model.Playlist, error) {
	return defaultNetease.SearchPlaylist(keyword)
}
func GetPlaylistSongs(playlistID string) ([]model.Song, error) {
	return defaultNetease.GetPlaylistSongs(playlistID)
}
func ParsePlaylist(link string) (*model.Playlist, []model.Song, error) {
	return defaultNetease.ParsePlaylist(link)
}
func GetDownloadURL(s *model.Song) (string, error) { return defaultNetease.GetDownloadURL(s) }
func GetLyrics(s *model.Song) (string, error)      { return defaultNetease.GetLyrics(s) }
func Parse(link string) (*model.Song, error)       { return defaultNetease.Parse(link) }

// GetRecommendedPlaylists 新增：获取推荐歌单（无需登录）
func GetRecommendedPlaylists() ([]model.Playlist, error) {
	return defaultNetease.GetRecommendedPlaylists()
}

// Search 搜索歌曲
func (n *Netease) Search(keyword string) ([]model.Song, error) {
	eparams := map[string]interface{}{
		"method": "POST",
		"url":    "http://music.163.com/api/cloudsearch/pc",
		"params": map[string]interface{}{"s": keyword, "type": 1, "offset": 0, "limit": 10},
	}
	eparamsJSON, _ := json.Marshal(eparams)
	encryptedParam := EncryptLinux(string(eparamsJSON))
	form := url.Values{}
	form.Set("eparams", encryptedParam)

	headers := []utils.RequestOption{
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Content-Type", "application/x-www-form-urlencoded"),
		utils.WithHeader("Cookie", n.cookie),
	}

	body, err := utils.Post(SearchAPI, strings.NewReader(form.Encode()), headers...)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Result struct {
			Songs []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
				Ar   []struct {
					Name string `json:"name"`
				} `json:"ar"`
				Al struct {
					Name   string `json:"name"`
					PicURL string `json:"picUrl"`
				} `json:"al"`
				Dt        int `json:"dt"`
				Privilege struct {
					Fl int `json:"fl"`
					Pl int `json:"pl"`
				} `json:"privilege"`
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

	var songs []model.Song
	for _, item := range resp.Result.Songs {
		// 简单过滤无版权或收费歌曲 (Privilege.Fl == 0)
		if item.Privilege.Fl == 0 {
			continue
		}

		var size int64
		// 优先选择高音质大小
		if item.Privilege.Fl >= 320000 && item.H.Size > 0 {
			size = item.H.Size
		} else if item.Privilege.Fl >= 192000 && item.M.Size > 0 {
			size = item.M.Size
		} else {
			size = item.L.Size
		}

		duration := item.Dt / 1000
		bitrate := 128
		if duration > 0 && size > 0 {
			bitrate = int(size * 8 / 1000 / int64(duration))
		}

		var artistNames []string
		for _, ar := range item.Ar {
			artistNames = append(artistNames, ar.Name)
		}

		songs = append(songs, model.Song{
			Source:   "netease",
			ID:       strconv.Itoa(item.ID),
			Name:     item.Name,
			Artist:   strings.Join(artistNames, "、"),
			Album:    item.Al.Name,
			Duration: duration,
			Size:     size,
			Bitrate:  bitrate,
			Cover:    item.Al.PicURL,
			Link:     fmt.Sprintf("https://music.163.com/#/song?id=%d", item.ID),
			Extra: map[string]string{
				"song_id": strconv.Itoa(item.ID),
			},
		})
	}
	return songs, nil
}

// SearchPlaylist 搜索歌单
func (n *Netease) SearchPlaylist(keyword string) ([]model.Playlist, error) {
	eparams := map[string]interface{}{
		"method": "POST",
		"url":    "http://music.163.com/api/cloudsearch/pc",
		"params": map[string]interface{}{"s": keyword, "type": 1000, "offset": 0, "limit": 10},
	}
	eparamsJSON, _ := json.Marshal(eparams)
	encryptedParam := EncryptLinux(string(eparamsJSON))
	form := url.Values{}
	form.Set("eparams", encryptedParam)

	headers := []utils.RequestOption{
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Content-Type", "application/x-www-form-urlencoded"),
		utils.WithHeader("Cookie", n.cookie),
	}

	body, err := utils.Post(SearchAPI, strings.NewReader(form.Encode()), headers...)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Result struct {
			Playlists []struct {
				ID          int    `json:"id"`
				Name        string `json:"name"`
				CoverImgURL string `json:"coverImgUrl"`
				Creator     struct {
					Nickname string `json:"nickname"`
				} `json:"creator"`
				TrackCount  int    `json:"trackCount"`
				PlayCount   int    `json:"playCount"`
				Description string `json:"description"`
			} `json:"playlists"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("netease playlist json parse error: %w", err)
	}

	var playlists []model.Playlist
	for _, item := range resp.Result.Playlists {
		playlists = append(playlists, model.Playlist{
			Source:      "netease",
			ID:          strconv.Itoa(item.ID),
			Name:        item.Name,
			Cover:       item.CoverImgURL,
			TrackCount:  item.TrackCount,
			PlayCount:   item.PlayCount,
			Creator:     item.Creator.Nickname,
			Description: item.Description,
			Link:        fmt.Sprintf("https://music.163.com/#/playlist?id=%d", item.ID),
		})
	}
	return playlists, nil
}

// GetPlaylistSongs 获取歌单详情（仅返回歌曲列表）
func (n *Netease) GetPlaylistSongs(playlistID string) ([]model.Song, error) {
	_, songs, err := n.fetchPlaylistDetail(playlistID)
	return songs, err
}

// ParsePlaylist 解析歌单链接
func (n *Netease) ParsePlaylist(link string) (*model.Playlist, []model.Song, error) {
	re := regexp.MustCompile(`playlist\?id=(\d+)`)
	matches := re.FindStringSubmatch(link)
	if len(matches) < 2 {
		return nil, nil, errors.New("invalid netease playlist link")
	}
	playlistID := matches[1]
	return n.fetchPlaylistDetail(playlistID)
}

// fetchPlaylistDetail 获取歌单详情 (核心逻辑：使用 trackIds 全量获取)
func (n *Netease) fetchPlaylistDetail(playlistID string) (*model.Playlist, []model.Song, error) {
	reqData := map[string]interface{}{
		"id":         playlistID,
		"n":          0, // 0表示不直接返回详情，我们只需要ID列表
		"csrf_token": "",
	}
	reqJSON, _ := json.Marshal(reqData)
	params, encSecKey := EncryptWeApi(string(reqJSON))
	form := url.Values{}
	form.Set("params", params)
	form.Set("encSecKey", encSecKey)

	headers := []utils.RequestOption{
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Content-Type", "application/x-www-form-urlencoded"),
		utils.WithHeader("Cookie", n.cookie),
	}

	body, err := utils.Post(PlaylistAPI, strings.NewReader(form.Encode()), headers...)
	if err != nil {
		return nil, nil, err
	}

	var resp struct {
		Code     int `json:"code"`
		Playlist struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			CoverImgURL string `json:"coverImgUrl"`
			Description string `json:"description"`
			PlayCount   int    `json:"playCount"`
			TrackCount  int    `json:"trackCount"`
			Creator     struct {
				Nickname string `json:"nickname"`
			} `json:"creator"`
			// 使用 TrackIds 获取完整列表，解决数量不一致问题
			TrackIds []struct {
				ID int `json:"id"`
			} `json:"trackIds"`
		} `json:"playlist"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, nil, fmt.Errorf("netease playlist detail json parse error: %w", err)
	}
	if resp.Code != 200 {
		return nil, nil, fmt.Errorf("netease api error code: %d", resp.Code)
	}

	// 构造 Playlist 元数据
	playlist := &model.Playlist{
		Source:      "netease",
		ID:          strconv.Itoa(resp.Playlist.ID),
		Name:        resp.Playlist.Name,
		Cover:       resp.Playlist.CoverImgURL,
		TrackCount:  resp.Playlist.TrackCount,
		PlayCount:   resp.Playlist.PlayCount,
		Creator:     resp.Playlist.Creator.Nickname,
		Description: resp.Playlist.Description,
		Link:        fmt.Sprintf("https://music.163.com/#/playlist?id=%d", resp.Playlist.ID),
	}

	// 提取所有 ID
	var allIDs []string
	for _, tid := range resp.Playlist.TrackIds {
		allIDs = append(allIDs, strconv.Itoa(tid.ID))
	}

	// 分批获取歌曲详情 (Detail API 支持批量，每次 500-1000 首没问题，这里保守用 500)
	var allSongs []model.Song
	batchSize := 500
	for i := 0; i < len(allIDs); i += batchSize {
		end := i + batchSize
		if end > len(allIDs) {
			end = len(allIDs)
		}

		batchIDs := allIDs[i:end]
		batchSongs, err := n.fetchSongsBatch(batchIDs)
		if err == nil {
			allSongs = append(allSongs, batchSongs...)
		}
	}

	return playlist, allSongs, nil
}

// fetchSongsBatch 批量获取歌曲详情 (利用 Detail 接口的批量特性，速度极快)
func (n *Netease) fetchSongsBatch(songIDs []string) ([]model.Song, error) {
	if len(songIDs) == 0 {
		return nil, nil
	}

	// 构造 c 参数: [{"id":123},{"id":456},...]
	var cList []map[string]interface{}
	for _, id := range songIDs {
		cList = append(cList, map[string]interface{}{"id": id})
	}
	cJSON, _ := json.Marshal(cList)
	idsJSON, _ := json.Marshal(songIDs)

	reqData := map[string]interface{}{
		"c":   string(cJSON),
		"ids": string(idsJSON),
	}
	reqJSON, _ := json.Marshal(reqData)
	params, encSecKey := EncryptWeApi(string(reqJSON))

	form := url.Values{}
	form.Set("params", params)
	form.Set("encSecKey", encSecKey)

	headers := []utils.RequestOption{
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Content-Type", "application/x-www-form-urlencoded"),
		utils.WithHeader("Cookie", n.cookie),
	}

	body, err := utils.Post(DetailAPI, strings.NewReader(form.Encode()), headers...)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Songs []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
			Ar   []struct {
				Name string `json:"name"`
			} `json:"ar"`
			Al struct {
				Name   string `json:"name"`
				PicURL string `json:"picUrl"`
			} `json:"al"`
			Dt int `json:"dt"`
		} `json:"songs"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	var songs []model.Song
	for _, item := range resp.Songs {
		var artistNames []string
		for _, ar := range item.Ar {
			artistNames = append(artistNames, ar.Name)
		}

		songs = append(songs, model.Song{
			Source:   "netease",
			ID:       strconv.Itoa(item.ID),
			Name:     item.Name,
			Artist:   strings.Join(artistNames, "、"),
			Album:    item.Al.Name,
			Duration: item.Dt / 1000,
			Cover:    item.Al.PicURL,
			Link:     fmt.Sprintf("https://music.163.com/#/song?id=%d", item.ID),
			Extra: map[string]string{
				"song_id": strconv.Itoa(item.ID),
			},
		})
	}
	return songs, nil
}

// Parse 解析单曲链接
func (n *Netease) Parse(link string) (*model.Song, error) {
	re := regexp.MustCompile(`id=(\d+)`)
	matches := re.FindStringSubmatch(link)
	if len(matches) < 2 {
		return nil, errors.New("invalid netease link")
	}
	songID := matches[1]

	songs, err := n.fetchSongsBatch([]string{songID})
	if err != nil || len(songs) == 0 {
		return nil, fmt.Errorf("fetch song detail failed: %v", err)
	}
	song := &songs[0]

	downloadURL, err := n.GetDownloadURL(song)
	if err == nil {
		song.URL = downloadURL
	}

	return song, nil
}

// GetDownloadURL 获取下载链接
func (n *Netease) GetDownloadURL(s *model.Song) (string, error) {
	if s.Source != "netease" {
		return "", errors.New("source mismatch")
	}

	songID := s.ID
	if s.Extra != nil && s.Extra["song_id"] != "" {
		songID = s.Extra["song_id"]
	}

	reqData := map[string]interface{}{
		"ids": []string{songID},
		"br":  320000,
	}
	reqJSON, _ := json.Marshal(reqData)
	params, encSecKey := EncryptWeApi(string(reqJSON))
	form := url.Values{}
	form.Set("params", params)
	form.Set("encSecKey", encSecKey)

	headers := []utils.RequestOption{
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Content-Type", "application/x-www-form-urlencoded"),
		utils.WithHeader("Cookie", n.cookie),
	}

	body, err := utils.Post(DownloadAPI, strings.NewReader(form.Encode()), headers...)
	if err != nil {
		return "", err
	}

	var resp struct {
		Data []struct {
			URL  string `json:"url"`
			Code int    `json:"code"`
			Br   int    `json:"br"`
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

	songID := s.ID
	if s.Extra != nil && s.Extra["song_id"] != "" {
		songID = s.Extra["song_id"]
	}

	reqData := map[string]interface{}{
		"csrf_token": "",
		"id":         songID,
		"lv":         -1,
		"tv":         -1,
	}
	reqJSON, _ := json.Marshal(reqData)
	params, encSecKey := EncryptWeApi(string(reqJSON))
	form := url.Values{}
	form.Set("params", params)
	form.Set("encSecKey", encSecKey)

	headers := []utils.RequestOption{
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Content-Type", "application/x-www-form-urlencoded"),
		utils.WithHeader("Cookie", n.cookie),
	}

	lyricAPI := "https://music.163.com/weapi/song/lyric"
	body, err := utils.Post(lyricAPI, strings.NewReader(form.Encode()), headers...)
	if err != nil {
		return "", err
	}

	var resp struct {
		Code int `json:"code"`
		Lrc  struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("json parse error: %w", err)
	}
	if resp.Code != 200 {
		return "", fmt.Errorf("netease api error code: %d", resp.Code)
	}
	return resp.Lrc.Lyric, nil
}

// GetRecommendedPlaylists 获取推荐歌单 (无需登录，即首页推荐歌单)
func (n *Netease) GetRecommendedPlaylists() ([]model.Playlist, error) {
	reqData := map[string]interface{}{
		"limit": 30, // 默认返回30个
		"total": true,
		"n":     1000,
	}
	reqJSON, _ := json.Marshal(reqData)
	params, encSecKey := EncryptWeApi(string(reqJSON))
	form := url.Values{}
	form.Set("params", params)
	form.Set("encSecKey", encSecKey)

	headers := []utils.RequestOption{
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("Content-Type", "application/x-www-form-urlencoded"),
		// 此接口不需要 Cookie
	}

	body, err := utils.Post(RecommendedPlaylistAPI, strings.NewReader(form.Encode()), headers...)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Code   int `json:"code"`
		Result []struct {
			ID         int     `json:"id"`
			Name       string  `json:"name"`
			PicURL     string  `json:"picUrl"`
			PlayCount  float64 `json:"playCount"`
			TrackCount int     `json:"trackCount"`
			Copywriter string  `json:"copywriter"` // 推荐语
			Alg        string  `json:"alg"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("netease recommended playlist json parse error: %w", err)
	}
	if resp.Code != 200 {
		return nil, fmt.Errorf("netease api error code: %d", resp.Code)
	}

	var playlists []model.Playlist
	for _, item := range resp.Result {
		pl := model.Playlist{
			Source:      "netease",
			ID:          strconv.Itoa(item.ID),
			Name:        item.Name,
			Cover:       item.PicURL,
			PlayCount:   int(item.PlayCount),
			TrackCount:  item.TrackCount, // 注意：此接口返回的 trackCount 可能为 0
			Description: item.Copywriter, // 将推荐语作为描述
			Link:        fmt.Sprintf("https://music.163.com/#/playlist?id=%d", item.ID),
			Extra:       map[string]string{},
		}

		// 如果有算法标签，可以保存
		if item.Alg != "" {
			pl.Extra["alg"] = item.Alg
		}

		playlists = append(playlists, pl)
	}

	return playlists, nil
}
