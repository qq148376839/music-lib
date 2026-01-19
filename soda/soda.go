package soda

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/utils"
)

const (
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36"
)

// Search 搜索歌曲
func Search(keyword string) ([]model.Song, error) {
	params := url.Values{}
	params.Set("q", keyword)
	params.Set("cursor", "0")
	params.Set("search_method", "input")
	params.Set("aid", "386088")
	params.Set("device_platform", "web")
	params.Set("channel", "pc_web")

	apiURL := "https://api.qishui.com/luna/pc/search/track?" + params.Encode()

	body, err := utils.Get(apiURL, utils.WithHeader("User-Agent", UserAgent))
	if err != nil {
		return nil, err
	}

	var resp struct {
		ResultGroups []struct {
			Data []struct {
				Entity struct {
					Track struct {
						ID       string `json:"id"`
						Name     string `json:"name"`
						Duration int    `json:"duration"`
						Artists  []struct {
							Name string `json:"name"`
						} `json:"artists"`
						Album struct {
							Name     string `json:"name"`
							UrlCover struct {
								Urls []string `json:"urls"`
							} `json:"url_cover"`
						} `json:"album"`
						LabelInfo struct {
							OnlyVipDownload bool `json:"only_vip_download"`
						} `json:"label_info"`
						BitRates []struct {
							Size int64 `json:"size"`
						} `json:"bit_rates"`
					} `json:"track"`
				} `json:"entity"`
			} `json:"data"`
		} `json:"result_groups"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("soda search json parse error: %w", err)
	}

	if len(resp.ResultGroups) == 0 {
		return nil, nil
	}

	var songs []model.Song
	for _, item := range resp.ResultGroups[0].Data {
		track := item.Entity.Track
		if track.ID == "" {
			continue
		}

		// 过滤 VIP 下载 (可选，参考 soda.py 逻辑可能不需要过滤，但这里保留以防万一)
		// if track.LabelInfo.OnlyVipDownload { continue }

		var artistNames []string
		for _, ar := range track.Artists {
			artistNames = append(artistNames, ar.Name)
		}

		var cover string
		if len(track.Album.UrlCover.Urls) > 0 {
			cover = track.Album.UrlCover.Urls[0] + "~c5_375x375.jpg"
		}

		var maxSize int64
		for _, br := range track.BitRates {
			if br.Size > maxSize {
				maxSize = br.Size
			}
		}

		songs = append(songs, model.Song{
			Source:   "soda",
			ID:       track.ID,
			Name:     track.Name,
			Artist:   strings.Join(artistNames, "、"),
			Album:    track.Album.Name,
			Duration: track.Duration / 1000,
			Size:     maxSize,
			Cover:    cover,
		})
	}

	return songs, nil
}

// DownloadInfo 下载信息
type DownloadInfo struct {
	URL      string
	PlayAuth string
	Format   string
	Size     int64
}

// GetDownloadInfo 获取下载信息
func GetDownloadInfo(s *model.Song) (*DownloadInfo, error) {
	if s.Source != "soda" {
		return nil, errors.New("source mismatch")
	}

	params := url.Values{}
	params.Set("track_id", s.ID)
	params.Set("media_type", "track")

	v2URL := "https://api.qishui.com/luna/pc/track_v2?" + params.Encode()
	v2Body, err := utils.Get(v2URL, utils.WithHeader("User-Agent", UserAgent))
	if err != nil {
		return nil, err
	}

	var v2Resp struct {
		TrackPlayer struct {
			URLPlayerInfo string `json:"url_player_info"`
		} `json:"track_player"`
	}
	if err := json.Unmarshal(v2Body, &v2Resp); err != nil {
		return nil, fmt.Errorf("parse track_v2 response error: %w", err)
	}

	if v2Resp.TrackPlayer.URLPlayerInfo == "" {
		return nil, errors.New("player info url not found")
	}

	infoBody, err := utils.Get(v2Resp.TrackPlayer.URLPlayerInfo, utils.WithHeader("User-Agent", UserAgent))
	if err != nil {
		return nil, err
	}

	var infoResp struct {
		Result struct {
			Data struct {
				PlayInfoList []struct {
					MainPlayUrl   string `json:"MainPlayUrl"`
					BackupPlayUrl string `json:"BackupPlayUrl"`
					PlayAuth      string `json:"PlayAuth"`
					Size          int64  `json:"Size"`
					Bitrate       int    `json:"Bitrate"`
					Format        string `json:"Format"`
				} `json:"PlayInfoList"`
			} `json:"Data"`
		} `json:"Result"`
	}

	if err := json.Unmarshal(infoBody, &infoResp); err != nil {
		return nil, fmt.Errorf("parse play info response error: %w", err)
	}

	list := infoResp.Result.Data.PlayInfoList
	if len(list) == 0 {
		return nil, errors.New("no audio stream found")
	}

	// 按 Size 和 Bitrate 降序排序，和 Python 逻辑对齐
	sort.Slice(list, func(i, j int) bool {
		if list[i].Size != list[j].Size {
			return list[i].Size > list[j].Size
		}
		return list[i].Bitrate > list[j].Bitrate
	})

	best := list[0]
	downloadURL := best.MainPlayUrl
	if downloadURL == "" {
		downloadURL = best.BackupPlayUrl
	}

	if downloadURL == "" {
		return nil, errors.New("invalid download url")
	}

	return &DownloadInfo{
		URL:      downloadURL,
		PlayAuth: best.PlayAuth,
		Format:   best.Format,
		Size:     best.Size,
	}, nil
}

// GetDownloadURL 返回下载链接
// 注意：汽水音乐的链接需要下载后解密，如果直接在浏览器播放会失败
// 建议使用 Download 函数下载并解密到本地
func GetDownloadURL(s *model.Song) (string, error) {
	info, err := GetDownloadInfo(s)
	if err != nil {
		return "", err
	}
	// 返回带 Auth 的链接，方便调试，但普通播放器无法播放
	return info.URL + "#auth=" + url.QueryEscape(info.PlayAuth), nil
}

// Download 下载并解密歌曲 (专供 Core 调用)
func Download(s *model.Song, outputPath string) error {
	// 1. 获取下载信息
	info, err := GetDownloadInfo(s)
	if err != nil {
		return fmt.Errorf("get download info failed: %w", err)
	}

	// 2. 下载原始加密文件
	client := &http.Client{}
	req, err := http.NewRequest("GET", info.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download status: %d", resp.StatusCode)
	}

	encryptedData, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// 3. 解密
	decryptedData, err := DecryptAudio(encryptedData, info.PlayAuth)
	if err != nil {
		return fmt.Errorf("decrypt failed: %w", err)
	}

	// 4. 保存
	err = os.WriteFile(outputPath, decryptedData, 0644)
	if err != nil {
		return err
	}

	return nil
}

// DecryptAudio 解密汽水音乐下载的加密音频数据
// fileData: 下载的原始文件字节数据
// playAuth: GetDownloadInfo 返回的 PlayAuth 字符串
func DecryptAudio(fileData []byte, playAuth string) ([]byte, error) {
	// 1. 提取 AES Key
	hexKey, err := extractKey(playAuth)
	if err != nil {
		return nil, fmt.Errorf("failed to extract key: %w", err)
	}
	if hexKey == "" {
		return nil, errors.New("extract key return empty")
	}
	keyBytes, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("invalid hex key: %w", err)
	}

	// 2. 查找 moov box
	moov, err := findBox(fileData, "moov", 0, len(fileData))
	if err != nil {
		return nil, errors.New("moov box not found")
	}

	// 3. 查找 trak -> mdia -> minf -> stbl 链路
	trak, err := findBox(fileData, "trak", moov.offset+8, moov.offset+moov.size)
	if err != nil {
		return nil, errors.New("trak box not found")
	}
	mdia, err := findBox(fileData, "mdia", trak.offset+8, trak.offset+trak.size)
	if err != nil {
		return nil, errors.New("mdia box not found")
	}
	minf, err := findBox(fileData, "minf", mdia.offset+8, mdia.offset+mdia.size)
	if err != nil {
		return nil, errors.New("minf box not found")
	}
	stbl, err := findBox(fileData, "stbl", minf.offset+8, minf.offset+minf.size)
	if err != nil {
		return nil, errors.New("stbl box not found")
	}

	// 4. 解析 stsz（Sample Sizes）
	stsz, err := findBox(fileData, "stsz", stbl.offset+8, stbl.offset+stbl.size)
	if err != nil {
		return nil, errors.New("stsz box not found")
	}
	sampleSizes := parseStsz(stsz.data)
	if len(sampleSizes) == 0 {
		return nil, errors.New("parse stsz empty")
	}

	// 5. 查找 senc box（先 moov 后 stbl，对齐 Python）
	senc, err := findBox(fileData, "senc", moov.offset+8, moov.offset+moov.size)
	if err != nil {
		senc, err = findBox(fileData, "senc", stbl.offset+8, stbl.offset+stbl.size)
	}
	if err != nil {
		return nil, errors.New("senc box not found")
	}
	ivs := parseSenc(senc.data)
	if len(ivs) == 0 {
		return nil, errors.New("parse senc ivs empty")
	}

	// 6. 查找 mdat box
	mdat, err := findBox(fileData, "mdat", 0, len(fileData))
	if err != nil {
		return nil, errors.New("mdat box not found")
	}

	// 7. AES-CTR 解密
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher failed: %w", err)
	}

	// 复制原始数据，避免修改原切片
	decryptedData := make([]byte, len(fileData))
	copy(decryptedData, fileData)

	readPtr := mdat.offset + 8
	decryptedMdat := make([]byte, 0, mdat.size-8)

	for i := 0; i < len(sampleSizes); i++ {
		size := int(sampleSizes[i])
		if readPtr+size > len(decryptedData) {
			break
		}
		chunk := decryptedData[readPtr : readPtr+size]

		if i < len(ivs) {
			// IV 填充：8字节 + 8个\x00（和 Python 一致）
			iv := ivs[i]
			if len(iv) < 16 {
				paddedIV := make([]byte, 16)
				copy(paddedIV, iv)
				iv = paddedIV
			}
			stream := cipher.NewCTR(block, iv)
			dst := make([]byte, size)
			stream.XORKeyStream(dst, chunk)
			decryptedMdat = append(decryptedMdat, dst...)
		} else {
			decryptedMdat = append(decryptedMdat, chunk...)
		}
		readPtr += size
	}

	// 8. 替换 mdat 数据
	if len(decryptedMdat) == int(mdat.size)-8 {
		copy(decryptedData[mdat.offset+8:], decryptedMdat)
	} else {
		return nil, errors.New("decrypted size mismatch")
	}

	// 9. 替换 stsd 中的 enca -> mp4a
	stsd, err := findBox(fileData, "stsd", stbl.offset+8, stbl.offset+stbl.size)
	if err == nil {
		stsdOffset := stsd.offset
		stsdData := decryptedData[stsdOffset : stsdOffset+stsd.size]
		if idx := bytes.Index(stsdData, []byte("enca")); idx != -1 {
			copy(stsdData[idx:], []byte("mp4a"))
			copy(decryptedData[stsdOffset:], stsdData)
		}
	}

	return decryptedData, nil
}

// --- 工具函数（严格对齐 Python 实现）---

type mp4Box struct {
	offset int
	size   int
	data   []byte
}

func findBox(data []byte, boxType string, start, end int) (*mp4Box, error) {
	if end > len(data) {
		end = len(data)
	}
	pos := start
	target := []byte(boxType)
	for pos+8 <= end {
		size := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		if size < 8 {
			break
		}
		currentType := data[pos+4 : pos+8]
		if bytes.Equal(currentType, target) {
			return &mp4Box{
				offset: pos,
				size:   size,
				data:   data[pos+8 : pos+size],
			}, nil
		}
		pos += size
	}
	return nil, errors.New("box not found")
}

func parseStsz(data []byte) []uint32 {
	if len(data) < 12 {
		return nil
	}
	sampleSizeFixed := binary.BigEndian.Uint32(data[4:8])
	sampleCount := int(binary.BigEndian.Uint32(data[8:12]))
	sizes := make([]uint32, sampleCount)

	if sampleSizeFixed != 0 {
		for i := 0; i < sampleCount; i++ {
			sizes[i] = sampleSizeFixed
		}
	} else {
		for i := 0; i < sampleCount; i++ {
			offset := 12 + i*4
			if offset+4 > len(data) {
				break
			}
			sizes[i] = binary.BigEndian.Uint32(data[offset : offset+4])
		}
	}
	return sizes
}

func parseSenc(data []byte) [][]byte {
	if len(data) < 8 {
		return nil
	}
	flags := binary.BigEndian.Uint32(data[0:4]) & 0x00FFFFFF
	sampleCount := int(binary.BigEndian.Uint32(data[4:8]))
	ivs := make([][]byte, 0, sampleCount)
	ptr := 8
	hasSubsamples := (flags & 0x02) != 0

	for i := 0; i < sampleCount; i++ {
		if ptr+8 > len(data) {
			break
		}
		// 提取 8 字节 IV
		rawIV := data[ptr : ptr+8]
		ivs = append(ivs, rawIV)
		ptr += 8

		if hasSubsamples {
			if ptr+2 > len(data) {
				break
			}
			subCount := int(binary.BigEndian.Uint16(data[ptr : ptr+2]))
			ptr += 2 + (subCount * 6)
		}
	}
	return ivs
}

// bitcount 完全移植 Python 的 bitcount 逻辑
// 【关键修复】Go 中 & 优先级高于 +，必须加括号以匹配 Python 逻辑
func bitcount(n int) int {
	u := uint32(n)
	u = u & 0xFFFFFFFF
	u = u - ((u >> 1) & 0x55555555)
	u = (u & 0x33333333) + ((u >> 2) & 0x33333333)
	// Python: ((n + (n >> 4) & 0xF0F0F0F) * 0x1010101) >> 24
	// Go (Fix): (((u + (u >> 4)) & 0xF0F0F0F) * 0x1010101) >> 24
	return int((((u + (u >> 4)) & 0xF0F0F0F) * 0x1010101) >> 24)
}

func decodeBase36(c byte) int {
	if c >= '0' && c <= '9' {
		return int(c - '0')
	}
	if c >= 'a' && c <= 'z' {
		return int(c - 'a' + 10)
	}
	return 0xFF
}

func decryptSpadeInner(keyBytes []byte) []byte {
	result := make([]byte, len(keyBytes))
	buff := append([]byte{0xFA, 0x55}, keyBytes...)
	for i := 0; i < len(result); i++ {
		// Python: v = ...; while v < 0: v += 255
		v := int(keyBytes[i]^buff[i]) - bitcount(i) - 21
		for v < 0 {
			v += 255
		}
		result[i] = byte(v)
	}
	return result
}

func extractKey(playAuth string) (string, error) {
	binaryStr, err := base64.StdEncoding.DecodeString(playAuth)
	if err != nil {
		return "", err
	}
	bytesData := []byte(binaryStr)
	if len(bytesData) < 3 {
		return "", errors.New("auth data too short")
	}

	paddingLen := int((bytesData[0] ^ bytesData[1] ^ bytesData[2]) - 48)
	if len(bytesData) < paddingLen+2 {
		return "", errors.New("invalid padding length")
	}

	innerInput := bytesData[1 : len(bytesData)-paddingLen]
	tmpBuff := decryptSpadeInner(innerInput)
	if len(tmpBuff) == 0 {
		return "", errors.New("decryption failed")
	}

	skipBytes := decodeBase36(tmpBuff[0])
	decodedLen := len(bytesData) - paddingLen - 2
	endIndex := 1 + decodedLen - skipBytes

	if endIndex > len(tmpBuff) || endIndex < 1 {
		return "", errors.New("index out of bounds")
	}

	finalBytes := tmpBuff[1:endIndex]
	return string(finalBytes), nil
}
