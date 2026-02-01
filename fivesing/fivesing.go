package fivesing

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/utils"
)

const (
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36"
)

type Fivesing struct {
	cookie string
}

func New(cookie string) *Fivesing {
	return &Fivesing{cookie: cookie}
}

var defaultFivesing = New("")

func Search(keyword string) ([]model.Song, error) { return defaultFivesing.Search(keyword) }
func SearchPlaylist(keyword string) ([]model.Playlist, error) {
	return defaultFivesing.SearchPlaylist(keyword)
}
func GetPlaylistSongs(id string) ([]model.Song, error) {
	return defaultFivesing.GetPlaylistSongs(id)
}
func ParsePlaylist(link string) (*model.Playlist, []model.Song, error) {
	return defaultFivesing.ParsePlaylist(link)
}
func GetDownloadURL(s *model.Song) (string, error) { return defaultFivesing.GetDownloadURL(s) }
func GetLyrics(s *model.Song) (string, error)      { return defaultFivesing.GetLyrics(s) }
func Parse(link string) (*model.Song, error)       { return defaultFivesing.Parse(link) }

// Search 搜索歌曲
func (f *Fivesing) Search(keyword string) ([]model.Song, error) {
	params := url.Values{}
	params.Set("keyword", keyword)
	params.Set("sort", "1")
	params.Set("page", "1")
	params.Set("filter", "0")
	params.Set("type", "0")

	apiURL := "http://search.5sing.kugou.com/home/json?" + params.Encode()
	body, err := utils.Get(apiURL, utils.WithHeader("User-Agent", UserAgent), utils.WithHeader("Cookie", f.cookie))
	if err != nil {
		return nil, err
	}

	var resp struct {
		List []struct {
			SongID    int64  `json:"songId"`
			SongName  string `json:"songName"`
			Singer    string `json:"singer"`
			SongSize  int64  `json:"songSize"`
			TypeEname string `json:"typeEname"`
		} `json:"list"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("fivesing json parse error: %w", err)
	}

	var songs []model.Song
	for _, item := range resp.List {
		name := removeEmTags(html.UnescapeString(item.SongName))
		artist := removeEmTags(html.UnescapeString(item.Singer))

		duration := 0
		if item.SongSize > 0 {
			duration = int((item.SongSize * 8) / 320000)
		}

		songs = append(songs, model.Song{
			Source:   "fivesing",
			ID:       fmt.Sprintf("%d|%s", item.SongID, item.TypeEname),
			Name:     name,
			Artist:   artist,
			Duration: duration,
			Size:     item.SongSize,
			Link:     fmt.Sprintf("http://5sing.kugou.com/%s/%d.html", item.TypeEname, item.SongID),
			Extra: map[string]string{
				"songid":   strconv.FormatInt(item.SongID, 10),
				"songtype": item.TypeEname,
			},
		})
	}
	return songs, nil
}

// SearchPlaylist 搜索歌单
func (f *Fivesing) SearchPlaylist(keyword string) ([]model.Playlist, error) {
	params := url.Values{}
	params.Set("keyword", keyword)
	params.Set("sort", "1")
	params.Set("page", "1")
	params.Set("filter", "0")
	params.Set("type", "1")

	apiURL := "http://search.5sing.kugou.com/home/json?" + params.Encode()
	body, err := utils.Get(apiURL, utils.WithHeader("User-Agent", UserAgent), utils.WithHeader("Cookie", f.cookie))
	if err != nil {
		return nil, err
	}

	var resp struct {
		List []struct {
			SongListId string `json:"songListId"`
			Title      string `json:"title"`
			Picture    string `json:"pictureUrl"`
			PlayCount  int    `json:"playCount"`
			UserName   string `json:"userName"`
			SongCnt    int    `json:"songCnt"`
			Content    string `json:"content"`
			UserId     string `json:"userId"`
		} `json:"list"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("fivesing playlist json parse error: %w", err)
	}

	playlists := make([]model.Playlist, len(resp.List))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // Limit concurrency

	for i, item := range resp.List {
		title := removeEmTags(html.UnescapeString(item.Title))
		desc := removeEmTags(html.UnescapeString(item.Content))
		if desc == "0" {
			desc = ""
		}

		link := fmt.Sprintf("http://5sing.kugou.com/%s/dj/%s.html", item.UserId, item.SongListId)
		creator := item.UserName
		if creator == "" {
			creator = "ID: " + item.UserId
		}

		playlists[i] = model.Playlist{
			Source:      "fivesing",
			ID:          item.SongListId,
			Name:        title,
			Cover:       item.Picture,
			TrackCount:  item.SongCnt,
			PlayCount:   item.PlayCount,
			Creator:     creator,
			Description: desc,
			Link:        link,
			Extra: map[string]string{
				"user_id": item.UserId,
			},
		}

		// Parallel fetch for creator name if missing
		if item.UserName == "" && item.SongListId != "" {
			wg.Add(1)
			go func(idx int, plID string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				// Call fetchPlaylistInfo (lightweight version of fetchPlaylistDetail)
				if name, err := f.fetchCreatorName(plID); err == nil && name != "" {
					playlists[idx].Creator = name
				}
			}(i, item.SongListId)
		}
	}
	wg.Wait()

	return playlists, nil
}

// fetchCreatorName Helper to get creator name from getsonglist API
func (f *Fivesing) fetchCreatorName(id string) (string, error) {
	infoURL := fmt.Sprintf("http://mobileapi.5sing.kugou.com/song/getsonglist?id=%s&songfields=user", id)
	infoBody, err := utils.Get(infoURL, utils.WithHeader("User-Agent", UserAgent))
	if err != nil {
		return "", err
	}
	var infoResp struct {
		Data struct {
			User struct {
				UserName string `json:"NN"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(infoBody, &infoResp); err != nil {
		return "", err
	}
	return infoResp.Data.User.UserName, nil
}

// GetPlaylistSongs 获取歌单详情 (仅返回歌曲)
func (f *Fivesing) GetPlaylistSongs(id string) ([]model.Song, error) {
	_, songs, err := f.fetchPlaylistDetail(id, "")
	return songs, err
}

// ParsePlaylist 解析歌单链接并返回详情
func (f *Fivesing) ParsePlaylist(link string) (*model.Playlist, []model.Song, error) {
	// [修正] 支持纯数字ID (\d+) 和 字母数字混合ID ([a-zA-Z0-9]+)
	// 链接格式: http://5sing.kugou.com/{userId}/dj/{songListId}.html
	re := regexp.MustCompile(`5sing\.kugou\.com/(?:(\d+)/)?dj/([a-zA-Z0-9]+)\.html`)
	matches := re.FindStringSubmatch(link)
	if len(matches) < 3 {
		return nil, nil, errors.New("invalid 5sing playlist link")
	}
	userId := matches[1] // 可能为空
	playlistId := matches[2]

	return f.fetchPlaylistDetail(playlistId, userId)
}

// fetchPlaylistDetail [内部通用] 获取歌单详情 (Metadata + Songs)
// userId 是可选的，如果为空则尝试自动获取
func (f *Fivesing) fetchPlaylistDetail(id, knownUserId string) (*model.Playlist, []model.Song, error) {
	// 1. 自动获取 UserId (如果未提供)
	var userId = knownUserId

	// 我们需要调用 getsonglist 接口，无论是否有 userId，
	// 因为这个接口返回了歌单的标题、封面等元数据
	infoURL := fmt.Sprintf("http://mobileapi.5sing.kugou.com/song/getsonglist?id=%s&songfields=ID,user", id)
	infoBody, err := utils.Get(infoURL, utils.WithHeader("User-Agent", UserAgent))
	if err != nil {
		return nil, nil, fmt.Errorf("fetch info failed: %w", err)
	}

	var infoResp struct {
		Data struct {
			Title     string `json:"T"`
			Content   string `json:"C"`
			Picture   string `json:"P"`
			Click     int    `json:"H"`
			SongCount int    `json:"E"`
			User      struct {
				ID       int64  `json:"ID"`
				UserName string `json:"NN"`
			} `json:"user"`
		} `json:"data"`
	}

	if err := json.Unmarshal(infoBody, &infoResp); err != nil {
		return nil, nil, fmt.Errorf("fetch playlist info error: %w", err)
	}

	// 更新元数据
	if infoResp.Data.User.ID != 0 {
		userId = strconv.FormatInt(infoResp.Data.User.ID, 10)
	}
	if userId == "" {
		return nil, nil, errors.New("playlist user not found")
	}

	playlist := &model.Playlist{
		Source:      "fivesing",
		ID:          id,
		Name:        infoResp.Data.Title,
		Cover:       infoResp.Data.Picture,
		TrackCount:  infoResp.Data.SongCount,
		PlayCount:   infoResp.Data.Click,
		Creator:     infoResp.Data.User.UserName,
		Description: infoResp.Data.Content,
		Link:        fmt.Sprintf("http://5sing.kugou.com/%s/dj/%s.html", userId, id),
		Extra:       map[string]string{"user_id": userId},
	}

	// 2. 解析 HTML 获取歌曲列表
	pageURL := playlist.Link
	htmlBodyBytes, err := utils.Get(pageURL,
		utils.WithHeader("User-Agent", UserAgent),
		utils.WithHeader("Cookie", f.cookie),
	)
	if err != nil {
		return playlist, nil, err // 即使解析失败也返回元数据
	}
	htmlContent := string(htmlBodyBytes)

	// 3. 解析歌曲列表 (复用原逻辑)
	blockRe := regexp.MustCompile(`<li class="p_rel">([\s\S]*?)</li>`)
	blocks := blockRe.FindAllStringSubmatch(htmlContent, -1)

	if len(blocks) == 0 {
		// 可能是空歌单或者结构变化
		return playlist, nil, nil
	}

	songRe := regexp.MustCompile(`href="http://5sing\.kugou\.com/(yc|fc|bz)/(\d+)\.html"[^>]*>([^<]+)</a>`)
	artistRe := regexp.MustCompile(`class="s_soner[^"]*".*?>([^<]+)</a>`)

	var songs []model.Song
	seen := make(map[string]bool)

	for _, match := range blocks {
		blockHTML := match[1]

		songMatch := songRe.FindStringSubmatch(blockHTML)
		if len(songMatch) < 4 {
			continue
		}
		kind := songMatch[1]
		songID := songMatch[2]
		rawName := songMatch[3]

		artist := "Unknown"
		artistMatch := artistRe.FindStringSubmatch(blockHTML)
		if len(artistMatch) >= 2 {
			artist = artistMatch[1]
		}

		uniqueKey := kind + "|" + songID
		if seen[uniqueKey] {
			continue
		}
		seen[uniqueKey] = true

		name := strings.TrimSpace(html.UnescapeString(rawName))
		artist = strings.TrimSpace(html.UnescapeString(artist))

		songs = append(songs, model.Song{
			Source: "fivesing",
			ID:     fmt.Sprintf("%s|%s", songID, kind),
			Name:   name,
			Artist: artist,
			Link:   fmt.Sprintf("http://5sing.kugou.com/%s/%s.html", kind, songID),
			Extra: map[string]string{
				"songid":   songID,
				"songtype": kind,
			},
		})
	}

	return playlist, songs, nil
}

// Parse 解析链接并获取完整信息
func (f *Fivesing) Parse(link string) (*model.Song, error) {
	re := regexp.MustCompile(`5sing\.kugou\.com/(\w+)/(\d+)\.html`)
	matches := re.FindStringSubmatch(link)
	if len(matches) < 3 {
		return nil, errors.New("invalid 5sing link")
	}
	songType := matches[1]
	songID := matches[2]

	return f.fetchSongInfo(songID, songType)
}

// GetDownloadURL 获取下载链接
func (f *Fivesing) GetDownloadURL(s *model.Song) (string, error) {
	if s.Source != "fivesing" {
		return "", errors.New("source mismatch")
	}
	if s.URL != "" {
		return s.URL, nil
	}

	var songID, songType string
	if s.Extra != nil {
		songID = s.Extra["songid"]
		songType = s.Extra["songtype"]
	}

	if songID == "" || songType == "" {
		parts := strings.Split(s.ID, "|")
		if len(parts) == 2 {
			songID = parts[0]
			songType = parts[1]
		} else {
			return "", errors.New("invalid id structure")
		}
	}

	return f.fetchAudioLink(songID, songType)
}

// fetchSongInfo 获取完整的歌曲信息（Metadata + URL）
func (f *Fivesing) fetchSongInfo(songID, songType string) (*model.Song, error) {
	audioURL, err := f.fetchAudioLink(songID, songType)
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Set("songid", songID)
	params.Set("songtype", songType)
	metaURL := "http://mobileapi.5sing.kugou.com/song/newget?" + params.Encode()

	metaBody, _ := utils.Get(metaURL, utils.WithHeader("User-Agent", UserAgent), utils.WithHeader("Cookie", f.cookie))

	var name, artist, cover string

	if metaBody != nil {
		var metaResp struct {
			Data struct {
				SongName string `json:"songName"`
				Singer   string `json:"singer"`
				Img      string `json:"img"`
			} `json:"data"`
		}
		if json.Unmarshal(metaBody, &metaResp) == nil {
			name = metaResp.Data.SongName
			artist = metaResp.Data.Singer
			cover = metaResp.Data.Img
		}
	}

	if name == "" {
		name = fmt.Sprintf("5sing_%s_%s", songType, songID)
	}

	return &model.Song{
		Source: "fivesing",
		ID:     fmt.Sprintf("%s|%s", songID, songType),
		Name:   name,
		Artist: artist,
		Cover:  cover,
		URL:    audioURL,
		Link:   fmt.Sprintf("http://5sing.kugou.com/%s/%s.html", songType, songID),
		Extra: map[string]string{
			"songid":   songID,
			"songtype": songType,
		},
	}, nil
}

// fetchAudioLink 仅获取音频链接
func (f *Fivesing) fetchAudioLink(songID, songType string) (string, error) {
	params := url.Values{}
	params.Set("songid", songID)
	params.Set("songtype", songType)

	apiURL := "http://mobileapi.5sing.kugou.com/song/getSongUrl?" + params.Encode()
	body, err := utils.Get(apiURL, utils.WithHeader("User-Agent", UserAgent), utils.WithHeader("Cookie", f.cookie))
	if err != nil {
		return "", err
	}

	var resp struct {
		Code int `json:"code"`
		Data struct {
			SQUrl       string `json:"squrl"`
			SQUrlBackup string `json:"squrl_backup"`
			HQUrl       string `json:"hqurl"`
			HQUrlBackup string `json:"hqurl_backup"`
			LQUrl       string `json:"lqurl"`
			LQUrlBackup string `json:"lqurl_backup"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("json parse error: %w", err)
	}

	if resp.Code != 1000 {
		return "", errors.New("api returned error code")
	}

	if url := getFirstValid(resp.Data.SQUrl, resp.Data.SQUrlBackup); url != "" {
		return url, nil
	}
	if url := getFirstValid(resp.Data.HQUrl, resp.Data.HQUrlBackup); url != "" {
		return url, nil
	}
	if url := getFirstValid(resp.Data.LQUrl, resp.Data.LQUrlBackup); url != "" {
		return url, nil
	}

	return "", errors.New("no valid download url found")
}

func (f *Fivesing) GetLyrics(s *model.Song) (string, error) {
	if s.Source != "fivesing" {
		return "", errors.New("source mismatch")
	}

	var songID, songType string
	if s.Extra != nil {
		songID = s.Extra["songid"]
		songType = s.Extra["songtype"]
	} else {
		parts := strings.Split(s.ID, "|")
		if len(parts) == 2 {
			songID = parts[0]
			songType = parts[1]
		}
	}

	if songID == "" {
		return "", errors.New("invalid id")
	}

	params := url.Values{}
	params.Set("songid", songID)
	params.Set("songtype", songType)
	apiURL := "http://mobileapi.5sing.kugou.com/song/newget?" + params.Encode()

	body, err := utils.Get(apiURL, utils.WithHeader("User-Agent", UserAgent), utils.WithHeader("Cookie", f.cookie))
	if err != nil {
		return "", err
	}

	var resp struct {
		Data struct {
			DynamicWords string `json:"dynamicWords"`
		} `json:"data"`
	}
	json.Unmarshal(body, &resp)
	if resp.Data.DynamicWords == "" {
		return "", errors.New("lyrics not found")
	}
	return resp.Data.DynamicWords, nil
}

func getFirstValid(urls ...string) string {
	for _, u := range urls {
		if u != "" {
			return u
		}
	}
	return ""
}

func removeEmTags(s string) string {
	s = strings.ReplaceAll(s, "<em class=\"keyword\">", "")
	s = strings.ReplaceAll(s, "</em>", "")
	return strings.TrimSpace(s)
}
