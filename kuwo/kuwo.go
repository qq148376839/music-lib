package kuwo

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/utils"
)

const (
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36"
)

type Kuwo struct {
	cookie string
}

func New(cookie string) *Kuwo { return &Kuwo{cookie: cookie} }

var defaultKuwo = New("")

func Search(keyword string) ([]model.Song, error) { return defaultKuwo.Search(keyword) }
func SearchPlaylist(keyword string) ([]model.Playlist, error) {
	return defaultKuwo.SearchPlaylist(keyword)
}                                                      // [新增]
func GetPlaylistSongs(id string) ([]model.Song, error) { return defaultKuwo.GetPlaylistSongs(id) } // [新增]
func GetDownloadURL(s *model.Song) (string, error)     { return defaultKuwo.GetDownloadURL(s) }
func GetLyrics(s *model.Song) (string, error)          { return defaultKuwo.GetLyrics(s) }
func Parse(link string) (*model.Song, error)           { return defaultKuwo.Parse(link) }

// Search 搜索歌曲
func (k *Kuwo) Search(keyword string) ([]model.Song, error) {
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
		utils.WithHeader("Cookie", k.cookie),
	)
	if err != nil {
		return nil, err
	}

	var resp struct {
		AbsList []struct {
			MusicRID  string `json:"MUSICRID"`
			SongName  string `json:"SONGNAME"`
			Artist    string `json:"ARTIST"`
			Album     string `json:"ALBUM"`
			Duration  string `json:"DURATION"`
			HtsMVPic  string `json:"hts_MVPIC"`
			MInfo     string `json:"MINFO"`
			PayInfo   string `json:"PAY"`
			BitSwitch int    `json:"bitSwitch"`
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
		bitrate := parseBitrateFromMInfo(item.MInfo)

		songs = append(songs, model.Song{
			Source:   "kuwo",
			ID:       cleanID,
			Name:     item.SongName,
			Artist:   item.Artist,
			Album:    item.Album,
			Duration: duration,
			Size:     size,
			Bitrate:  bitrate,
			Cover:    item.HtsMVPic,
			Link:     fmt.Sprintf("http://www.kuwo.cn/play_detail/%s", cleanID),
			Extra: map[string]string{
				"rid": cleanID,
			},
		})
	}

	return songs, nil
}

// SearchPlaylist 搜索歌单
func (k *Kuwo) SearchPlaylist(keyword string) ([]model.Playlist, error) {
	params := url.Values{}
	params.Set("all", keyword)
	params.Set("ft", "playlist")
	params.Set("itemset", "web_2013")
	params.Set("client", "kt")
	params.Set("pcmp4", "1")
	params.Set("geo", "c")
	params.Set("vipver", "1")
	params.Set("pn", "0")
	params.Set("rn", "10")
	params.Set("rformat", "json")
	params.Set("encoding", "utf8")

	// 使用 search.kuwo.cn 接口
	apiURL := "http://search.kuwo.cn/r.s?" + params.Encode()

	body, err := utils.Get(apiURL,
		utils.WithHeader("User-Agent", UserAgent),
		utils.WithHeader("Cookie", k.cookie),
	)
	if err != nil {
		return nil, err
	}

	// [关键修复] 酷我旧接口返回的是 {'key':'value'} 格式（单引号），非标准JSON
	// 我们需要手动替换为双引号才能被 Go 解析
	jsonStr := string(body)
	jsonStr = strings.ReplaceAll(jsonStr, "'", "\"")

	// 定义与清洗后 JSON 匹配的结构体
	var resp struct {
		AbsList []struct {
			PlaylistID string `json:"playlistid"`
			Name       string `json:"name"`
			Pic        string `json:"pic"`
			SongNum    string `json:"songnum"`
			Intro      string `json:"intro"`
			NickName   string `json:"nickname"` // 之前是 UNAME，实际是 nickname
		} `json:"abslist"`
	}

	// 解析清洗后的字符串
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		// 如果替换后仍然解析失败，打印原始数据以便调试
		// fmt.Printf("DEBUG: Original Kuwo Body: %s\n", string(body))
		return nil, fmt.Errorf("kuwo playlist json parse error: %w", err)
	}

	var playlists []model.Playlist
	for _, item := range resp.AbsList {
		count, _ := strconv.Atoi(item.SongNum)
		// 图片链接修复：有时候不带协议头
		cover := item.Pic
		if cover != "" {
			// 修复可能出现的图片尺寸后缀，例如 _150.jpg -> _700.jpg 获取大图
			// 酷我图片通常格式: .../123456_150.jpg
			cover = strings.Replace(cover, "_150.", "_700.", 1)
			if !strings.HasPrefix(cover, "http") {
				cover = "http://" + cover
			}
		}

		playlists = append(playlists, model.Playlist{
			Source:      "kuwo",
			ID:          item.PlaylistID,
			Name:        item.Name,
			Cover:       cover,
			TrackCount:  count,
			Creator:     item.NickName,
			Description: item.Intro,
		})
	}
	return playlists, nil
}

// GetPlaylistSongs 获取歌单详情（解析歌曲列表）
func (k *Kuwo) GetPlaylistSongs(id string) ([]model.Song, error) {
	params := url.Values{}
	params.Set("op", "getlistinfo")
	params.Set("pid", id)
	params.Set("pn", "0")
	params.Set("rn", "100")
	params.Set("encode", "utf8")
	params.Set("keyset", "pl2012")
	params.Set("identity", "kuwo")
	params.Set("pcmp4", "1")
	params.Set("vipver", "1")
	params.Set("newver", "1")

	apiURL := "http://nplserver.kuwo.cn/pl.svc?" + params.Encode()

	body, err := utils.Get(apiURL,
		utils.WithHeader("User-Agent", UserAgent),
		utils.WithHeader("Cookie", k.cookie),
	)
	if err != nil {
		return nil, err
	}

	// 定义与实际返回 JSON 匹配的结构体
	var resp struct {
		MusicList []struct {
			Id     string `json:"id"`
			Name   string `json:"name"`
			Artist string `json:"artist"`
			Album  string `json:"album"`

			// [修复] 实际返回的图片字段是 albumpic
			AlbumPic string `json:"albumpic"`

			// [修复] 甚至 ID 也是字符串，duration 也是字符串
			// 使用 interface{} 兼容 string 或 int
			Duration interface{} `json:"duration"`

			// 备用字段
			SongName   string `json:"song_name"`
			ArtistName string `json:"artist_name"`
		} `json:"musiclist"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kuwo playlist detail json error: %w", err)
	}

	if len(resp.MusicList) == 0 {
		return nil, errors.New("playlist is empty or id is invalid")
	}

	var songs []model.Song
	for _, item := range resp.MusicList {
		// 1. 字段容错
		name := item.Name
		if name == "" {
			name = item.SongName
		}
		artist := item.Artist
		if artist == "" {
			artist = item.ArtistName
		}

		// 2. 解析 Duration (可能是 string 也可能是 float/int)
		var duration int
		switch v := item.Duration.(type) {
		case string:
			// 如果是字符串 "154"
			d, _ := strconv.Atoi(v)
			duration = d
		case float64:
			// 如果是数字 154
			duration = int(v)
		}

		// 3. 图片链接修复
		cover := item.AlbumPic
		if cover != "" {
			// 补全协议头
			if !strings.HasPrefix(cover, "http") {
				cover = "http://" + cover
			}
			// 尝试获取大图：将 _100.jpg / _150.jpg 替换为 _500.jpg
			// 酷我图片 URL 规律通常包含尺寸信息
			if strings.Contains(cover, "_100.") {
				cover = strings.Replace(cover, "_100.", "_500.", 1)
			} else if strings.Contains(cover, "_150.") {
				cover = strings.Replace(cover, "_150.", "_500.", 1)
			} else if strings.Contains(cover, "_120.") { // 有时是 120
				cover = strings.Replace(cover, "_120.", "_500.", 1)
			}
		}

		songs = append(songs, model.Song{
			Source:   "kuwo",
			ID:       item.Id,
			Name:     name,
			Artist:   artist,
			Album:    item.Album,
			Duration: duration,
			Cover:    cover,
			Link:     fmt.Sprintf("http://www.kuwo.cn/play_detail/%s", item.Id),
			Extra: map[string]string{
				"rid": item.Id,
			},
		})
	}
	return songs, nil
}

// Parse 解析链接并获取完整信息
func (k *Kuwo) Parse(link string) (*model.Song, error) {
	// 1. 提取 RID
	// 支持格式: http://www.kuwo.cn/play_detail/123456
	re := regexp.MustCompile(`play_detail/(\d+)`)
	matches := re.FindStringSubmatch(link)
	if len(matches) < 2 {
		return nil, errors.New("invalid kuwo link, rid not found")
	}
	rid := matches[1]

	// 2. 结合元数据 API 和 下载链接 API
	return k.fetchFullSongInfo(rid)
}

// GetDownloadURL 获取下载链接
func (k *Kuwo) GetDownloadURL(s *model.Song) (string, error) {
	if s.Source != "kuwo" {
		return "", errors.New("source mismatch")
	}
	if s.URL != "" {
		return s.URL, nil
	}

	rid := s.ID
	if s.Extra != nil && s.Extra["rid"] != "" {
		rid = s.Extra["rid"]
	}

	return k.fetchAudioURL(rid)
}

// fetchFullSongInfo 内部聚合：同时获取元数据和下载链接
func (k *Kuwo) fetchFullSongInfo(rid string) (*model.Song, error) {
	// A. 尝试获取元数据 (复用 GetLyrics 调用的接口，该接口包含 SongInfo)
	params := url.Values{}
	params.Set("musicId", rid)
	params.Set("httpsStatus", "1")
	metaURL := "http://m.kuwo.cn/newh5/singles/songinfoandlrc?" + params.Encode()

	var name, artist, cover string
	metaBody, err := utils.Get(metaURL, utils.WithHeader("User-Agent", UserAgent), utils.WithHeader("Cookie", k.cookie))

	if err == nil {
		var metaResp struct {
			Data struct {
				SongInfo struct {
					SongName string `json:"songName"`
					Artist   string `json:"artist"`
					Pic      string `json:"pic"`
				} `json:"songinfo"`
			} `json:"data"`
		}
		if json.Unmarshal(metaBody, &metaResp) == nil {
			name = metaResp.Data.SongInfo.SongName
			artist = metaResp.Data.SongInfo.Artist
			cover = metaResp.Data.SongInfo.Pic
		}
	}

	// 兜底 Name
	if name == "" {
		name = fmt.Sprintf("Kuwo_Song_%s", rid)
	}

	// B. 获取下载链接
	audioURL, err := k.fetchAudioURL(rid)
	if err != nil {
		// 即使没有音频，也返回元数据，但在实际使用中可能希望报错
		// 这里选择报错，保证 Parse 的结果是可用下载的
		return nil, err
	}

	return &model.Song{
		Source: "kuwo",
		ID:     rid,
		Name:   name,
		Artist: artist,
		Cover:  cover,
		URL:    audioURL,
		Link:   fmt.Sprintf("http://www.kuwo.cn/play_detail/%s", rid),
		Extra: map[string]string{
			"rid": rid,
		},
	}, nil
}

// fetchAudioURL 内部核心：仅获取下载链接
func (k *Kuwo) fetchAudioURL(rid string) (string, error) {
	qualities := []string{"128kmp3", "320kmp3", "flac", "2000kflac"}
	randomID := fmt.Sprintf("C_APK_guanwang_%d%d", time.Now().UnixNano(), rand.Intn(1000000))

	for _, br := range qualities {
		params := url.Values{}
		params.Set("f", "web")
		params.Set("source", "kwplayercar_ar_6.0.0.9_B_jiakong_vh.apk")
		params.Set("from", "PC")
		params.Set("type", "convert_url_with_sign")
		params.Set("br", br)
		params.Set("rid", rid)
		params.Set("user", randomID)

		apiURL := "https://mobi.kuwo.cn/mobi.s?" + params.Encode()

		body, err := utils.Get(apiURL,
			utils.WithHeader("User-Agent", UserAgent),
			utils.WithHeader("Cookie", k.cookie),
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

// GetLyrics 获取歌词
func (k *Kuwo) GetLyrics(s *model.Song) (string, error) {
	if s.Source != "kuwo" {
		return "", errors.New("source mismatch")
	}

	rid := s.ID
	if s.Extra != nil && s.Extra["rid"] != "" {
		rid = s.Extra["rid"]
	}

	params := url.Values{}
	params.Set("musicId", rid)
	params.Set("httpsStatus", "1")

	apiURL := "http://m.kuwo.cn/newh5/singles/songinfoandlrc?" + params.Encode()
	body, err := utils.Get(apiURL,
		utils.WithHeader("User-Agent", UserAgent),
		utils.WithHeader("Cookie", k.cookie),
	)
	if err != nil {
		return "", fmt.Errorf("failed to fetch kuwo lyric API: %w", err)
	}

	var resp struct {
		Data struct {
			Lrclist []struct {
				Time      string `json:"time"`
				LineLyric string `json:"lineLyric"`
			} `json:"lrclist"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to parse kuwo lyric JSON: %w", err)
	}

	if len(resp.Data.Lrclist) == 0 {
		return "", nil
	}

	var sb strings.Builder
	for _, line := range resp.Data.Lrclist {
		secs, _ := strconv.ParseFloat(line.Time, 64)
		m := int(secs) / 60
		s := int(secs) % 60
		ms := int((secs - float64(int(secs))) * 100)
		sb.WriteString(fmt.Sprintf("[%02d:%02d.%02d]%s\n", m, s, ms, line.LineLyric))
	}
	return sb.String(), nil
}

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
		formats = append(formats, FormatInfo{Format: kv["format"], Bitrate: kv["bitrate"], Size: sizeBytes})
	}
	for _, f := range formats {
		if f.Format == "mp3" && f.Bitrate == "128" {
			return f.Size
		}
	}
	for _, f := range formats {
		if f.Format == "mp3" && f.Bitrate == "320" {
			return f.Size
		}
	}
	for _, f := range formats {
		if f.Format == "flac" {
			return f.Size
		}
	}
	for _, f := range formats {
		if f.Format == "flac" && f.Bitrate == "2000" {
			return f.Size
		}
	}
	var maxSize int64
	for _, f := range formats {
		if f.Size > maxSize {
			maxSize = f.Size
		}
	}
	return maxSize
}

func parseBitrateFromMInfo(minfo string) int {
	if minfo == "" {
		return 128
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
		formats = append(formats, FormatInfo{Format: kv["format"], Bitrate: kv["bitrate"], Size: sizeBytes})
	}
	toInt := func(s string) int { v, _ := strconv.Atoi(s); return v }
	for _, f := range formats {
		if f.Format == "mp3" && f.Bitrate == "128" {
			return 128
		}
	}
	for _, f := range formats {
		if f.Format == "mp3" && f.Bitrate == "320" {
			return 320
		}
	}
	for _, f := range formats {
		if f.Format == "flac" && f.Bitrate == "2000" {
			if val := toInt(f.Bitrate); val > 0 {
				return val
			}
			return 2000
		}
	}
	for _, f := range formats {
		if f.Format == "flac" {
			if val := toInt(f.Bitrate); val > 0 {
				return val
			}
			return 800
		}
	}
	return 128
}
