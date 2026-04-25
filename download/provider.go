package download

import "github.com/guohuiyuan/music-lib/model"

// ProviderFuncs holds the callbacks needed for downloading from a specific source.
type ProviderFuncs struct {
	Search         func(string) ([]model.Song, error)
	GetDownloadURL func(*model.Song) (string, error)
	GetLyrics      func(*model.Song) (string, error)
}
