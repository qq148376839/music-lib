package jamendo

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"regexp"
	"strconv"

	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/utils"
)

const (
	UserAgent         = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36"
	Referer           = "https://www.jamendo.com/search?q=musicdl"
	XJamVersion       = "4gvfvv"
	SearchAPI         = "https://www.jamendo.com/api/search"
	SearchApiPath     = "/api/search" 
	TrackAPI          = "https://www.jamendo.com/api/tracks" 
	TrackApiPath      = "/api/tracks" 
	PlaylistAPI       = "https://www.jamendo.com/api/playlists"
	PlaylistApiPath   = "/api/playlists"
	PlaylistTracksAPI = "https://www.jamendo.com/api/playlists/tracks"
	PlaylistTracksPath= "/api/playlists/tracks"
	ClientID = "9873ff31"
)

type Jamendo struct {
	cookie string
}

func New(cookie string) *Jamendo {
	return &Jamendo{cookie: cookie}
}

var defaultJamendo = New("")

func Search(keyword string) ([]model.Song, error) { return defaultJamendo.Search(keyword) }
func SearchPlaylist(keyword string) ([]model.Playlist, error) { return defaultJamendo.SearchPlaylist(keyword) } // [新增]
func GetPlaylistSongs(id string) ([]model.Song, error) { return defaultJamendo.GetPlaylistSongs(id) }       // [新增]
func GetDownloadURL(s *model.Song) (string, error) { return defaultJamendo.GetDownloadURL(s) }
func GetLyrics(s *model.Song) (string, error) { return defaultJamendo.GetLyrics(s) }
func Parse(link string) (*model.Song, error) { return defaultJamendo.Parse(link) }

// Search 搜索歌曲
func (j *Jamendo) Search(keyword string) ([]model.Song, error) {
	params := url.Values{}
	params.Set("query", keyword)
	params.Set("type", "track")
	params.Set("limit", "20")
	params.Set("identities", "www")
	apiURL := SearchAPI + "?" + params.Encode()
	xJamCall := makeXJamCall(SearchApiPath)

	body, err := utils.Get(apiURL,
		utils.WithHeader("User-Agent", UserAgent),
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("x-jam-call", xJamCall),
		utils.WithHeader("x-jam-version", XJamVersion),
		utils.WithHeader("x-requested-with", "XMLHttpRequest"),
		utils.WithHeader("Cookie", j.cookie),
	)
	if err != nil {
		return nil, err
	}

	var results []struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		Duration int    `json:"duration"`
		Artist   struct { Name string `json:"name"` } `json:"artist"`
		Album    struct { Name string `json:"name"` } `json:"album"`
		Cover    struct { Big struct { Size300 string `json:"size300"` } `json:"big"` } `json:"cover"`
		Download map[string]string `json:"download"`
		Stream   map[string]string `json:"stream"`
	}
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("jamendo json parse error: %w", err)
	}

	var songs []model.Song
	for _, item := range results {
		streams := item.Download
		if len(streams) == 0 { streams = item.Stream }
		if len(streams) == 0 { continue }

		downloadURL, ext := pickBestQuality(streams)
		if downloadURL == "" { continue }

		songs = append(songs, model.Song{
			Source:   "jamendo",
			ID:       fmt.Sprintf("%d", item.ID),
			Name:     item.Name,
			Artist:   item.Artist.Name,
			Album:    item.Album.Name,
			Duration: item.Duration,
			Ext:      ext,
			Cover:    item.Cover.Big.Size300,
			URL:      downloadURL, // Search 结果也可以直接带上 URL
			Link:     fmt.Sprintf("https://www.jamendo.com/track/%d", item.ID),
			Extra: map[string]string{
				"track_id": strconv.Itoa(item.ID),
			},
		})
	}
	return songs, nil
}
// SearchPlaylist 搜索歌单
func (j *Jamendo) SearchPlaylist(keyword string) ([]model.Playlist, error) {
	params := url.Values{}
	params.Set("namesearch", keyword)
	params.Set("v", "3.0")
	params.Set("format", "json")
	params.Set("client_id", ClientID)
	params.Set("limit", "10")
	params.Set("order", "creationdate_desc")

	// 使用 v3.0 公共 API
	apiURL := "https://api.jamendo.com/v3.0/playlists/?" + params.Encode()

	body, err := utils.Get(apiURL, utils.WithHeader("User-Agent", UserAgent))
	if err != nil {
		return nil, err
	}

	var resp struct {
		Headers struct {
			Status string `json:"status"`
			Code   int    `json:"code"`
		} `json:"headers"`
		Results []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Creation  string `json:"creationdate"`
			UserID    string `json:"user_id"`
			UserName  string `json:"user_name"`
			ShareURL  string `json:"shareurl"`
			// Jamendo 歌单搜索列表不直接返回封面，需后续处理或用默认图
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("jamendo playlist json parse error: %w", err)
	}

	if resp.Headers.Status != "success" {
		return nil, fmt.Errorf("jamendo api error: %s", resp.Headers.Status)
	}

	var playlists []model.Playlist
	for _, item := range resp.Results {
		playlists = append(playlists, model.Playlist{
			Source:      "jamendo",
			ID:          item.ID,
			Name:        item.Name,
			Creator:     item.UserName,
			Description: fmt.Sprintf("Created: %s", item.Creation),
			// Jamendo 歌单列表 API 不返回封面，留空或使用占位
			Cover:       "", 
			TrackCount:  0, // 列表接口未返回歌曲数
		})
	}
	return playlists, nil
}

// GetPlaylistSongs 获取歌单详情
func (j *Jamendo) GetPlaylistSongs(id string) ([]model.Song, error) {
	params := url.Values{}
	params.Set("id", id)
	params.Set("v", "3.0")
	params.Set("format", "json")
	params.Set("client_id", ClientID)
	params.Set("limit", "100") // 获取前100首

	// 注意：这里用的是 /playlists/tracks 接口
	apiURL := "https://api.jamendo.com/v3.0/playlists/tracks?" + params.Encode()

	body, err := utils.Get(apiURL, utils.WithHeader("User-Agent", UserAgent))
	if err != nil {
		return nil, err
	}

	var resp struct {
		Headers struct {
			Status string `json:"status"`
		} `json:"headers"`
		Results []struct {
			ID     string `json:"id"` // 歌单 ID
			Tracks []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Duration string `json:"duration"` // Jamendo v3 返回的是字符串秒数
				Artist   string `json:"artist_name"`
				Album    string `json:"album_name"`
				Image    string `json:"image"`    // 封面
				Audio    string `json:"audio"`    // 试听链接 (通常也就是下载链接)
				Audiodl  string `json:"audiodl"`  // 强制下载链接
			} `json:"tracks"`
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("jamendo playlist tracks json error: %w", err)
	}

	if len(resp.Results) == 0 || len(resp.Results[0].Tracks) == 0 {
		return nil, errors.New("playlist is empty or invalid")
	}

	var songs []model.Song
	for _, item := range resp.Results[0].Tracks {
		dur, _ := strconv.Atoi(item.Duration)
		
		// 优先使用 audio 字段 (mp3流)
		url := item.Audio
		if url == "" {
			url = item.Audiodl
		}

		songs = append(songs, model.Song{
			Source:   "jamendo",
			ID:       item.ID,
			Name:     item.Name,
			Artist:   item.Artist,
			Album:    item.Album,
			Duration: dur,
			Cover:    item.Image,
			URL:      url, // Jamendo 接口直接返回播放链接，可直接赋值
			Link:     fmt.Sprintf("https://www.jamendo.com/track/%s", item.ID),
			Extra: map[string]string{
				"track_id": item.ID,
			},
		})
	}
	return songs, nil
}

// Parse 解析链接并获取完整 Song 详情
func (j *Jamendo) Parse(link string) (*model.Song, error) {
	re := regexp.MustCompile(`jamendo\.com/track/(\d+)`)
	matches := re.FindStringSubmatch(link)
	if len(matches) < 2 {
		return nil, errors.New("invalid jamendo link")
	}
	trackID := matches[1]

	// 直接调用底层获取详情
	return j.getTrackByID(trackID)
}

// GetDownloadURL 获取下载链接
func (j *Jamendo) GetDownloadURL(s *model.Song) (string, error) {
	if s.Source != "jamendo" {
		return "", errors.New("source mismatch")
	}
	if s.URL != "" {
		return s.URL, nil
	}

	trackID := s.ID
	if s.Extra != nil && s.Extra["track_id"] != "" {
		trackID = s.Extra["track_id"]
	}
	if trackID == "" {
		return "", errors.New("id missing")
	}

	// 复用底层逻辑
	info, err := j.getTrackByID(trackID)
	if err != nil {
		return "", err
	}
	return info.URL, nil
}

// getTrackByID 底层核心逻辑：通过 ID 获取完整 Song 对象
func (j *Jamendo) getTrackByID(id string) (*model.Song, error) {
	params := url.Values{}
	params.Set("id", id)

	apiURL := TrackAPI + "?" + params.Encode()
	xJamCall := makeXJamCall(TrackApiPath)

	body, err := utils.Get(apiURL,
		utils.WithHeader("User-Agent", UserAgent),
		utils.WithHeader("Referer", Referer),
		utils.WithHeader("x-jam-call", xJamCall),
		utils.WithHeader("x-jam-version", XJamVersion),
		utils.WithHeader("x-requested-with", "XMLHttpRequest"),
		utils.WithHeader("Cookie", j.cookie),
	)
	if err != nil {
		return nil, err
	}

	var results []struct {
		ID       int               `json:"id"`
		Name     string            `json:"name"`
		Duration int               `json:"duration"`
		Artist   struct{ Name string `json:"name"` } `json:"artist"`
		Album    struct{ Name string `json:"name"` } `json:"album"`
		Cover    struct{ Big struct{ Size300 string `json:"size300"` } `json:"big"` } `json:"cover"`
		Download map[string]string `json:"download"`
		Stream   map[string]string `json:"stream"`
	}
	
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("jamendo track json error: %w", err)
	}
	if len(results) == 0 {
		return nil, errors.New("track not found")
	}

	item := results[0]
	streams := item.Download
	if len(streams) == 0 {
		streams = item.Stream
	}

	downloadURL, ext := pickBestQuality(streams)
	if downloadURL == "" {
		return nil, errors.New("no valid stream found")
	}

	return &model.Song{
		Source:   "jamendo",
		ID:       strconv.Itoa(item.ID),
		Name:     item.Name,
		Artist:   item.Artist.Name,
		Album:    item.Album.Name,
		Duration: item.Duration,
		Ext:      ext,
		Cover:    item.Cover.Big.Size300,
		URL:      downloadURL, // 已填充
		Link:     fmt.Sprintf("https://www.jamendo.com/track/%d", item.ID),
		Extra: map[string]string{
			"track_id": strconv.Itoa(item.ID),
		},
	}, nil
}

func pickBestQuality(streams map[string]string) (string, string) {
	if url := streams["flac"]; url != "" { return url, "flac" }
	if url := streams["mp3"]; url != "" { return url, "mp3" }
	if url := streams["ogg"]; url != "" { return url, "ogg" }
	return "", ""
}

func makeXJamCall(path string) string {
	r := rand.Float64()
	randStr := fmt.Sprintf("%v", r)
	data := path + randStr
	hash := sha1.Sum([]byte(data))
	digest := hex.EncodeToString(hash[:])
	return fmt.Sprintf("$%s*%s~", digest, randStr)
}

func (j *Jamendo) GetLyrics(s *model.Song) (string, error) {
	if s.Source != "jamendo" {
		return "", errors.New("source mismatch")
	}
	return "", nil
}