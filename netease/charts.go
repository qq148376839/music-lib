package netease

import "github.com/guohuiyuan/music-lib/model"

// Known chart playlist IDs in Netease's system.
var neteaseCharts = []model.Chart{
	{ID: "19723756", Name: "飙升榜"},
	{ID: "3778678", Name: "热歌榜"},
	{ID: "3779629", Name: "新歌榜"},
}

// GetCharts returns the list of supported Netease charts.
func GetCharts() ([]model.Chart, error) { return getDefault().GetCharts() }

// GetChartSongs returns the top N songs from a Netease chart.
func GetChartSongs(chartID string, limit int) ([]model.Song, error) {
	return getDefault().GetChartSongs(chartID, limit)
}

func (n *Netease) GetCharts() ([]model.Chart, error) {
	return neteaseCharts, nil
}

func (n *Netease) GetChartSongs(chartID string, limit int) ([]model.Song, error) {
	// Netease charts are just playlists with fixed IDs.
	songs, err := n.GetPlaylistSongs(chartID)
	if err != nil {
		return nil, err
	}
	if limit > 0 && limit < len(songs) {
		songs = songs[:limit]
	}
	return songs, nil
}
