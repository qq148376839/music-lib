package provider

import "github.com/guohuiyuan/music-lib/model"

// MusicProvider 定义了所有音乐源必须实现的方法
type MusicProvider interface {
	Search(keyword string) ([]model.Song, error)
	GetDownloadURL(s *model.Song) (string, error)
	GetLyrics(s *model.Song) (string, error)
}