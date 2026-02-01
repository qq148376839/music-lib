# 🎵 music-lib

music-lib 是一个 Go 音乐搜索下载库，提供统一的音乐数据接口，支持十多个主流音乐平台。

## 特性

- **后端库**: 提供函数接口，不包含 UI 逻辑，易于集成
- **多平台支持**: 支持网易云、QQ音乐、酷狗等十多个平台
- **统一数据模型**: 所有平台数据转换为 `model.Song` 结构
- **模块化设计**: 每个音乐平台都是独立的 `provider`
- **音源过滤**: 自动跳过需要 VIP 或付费的歌曲
- **高级功能**:
  - 歌词获取接口
  - 支持汽水音乐等平台的加密音频解密
  - 链接解析功能

## 支持的音乐平台

| 平台       | 模块名   | 搜索 | 下载 | 歌词 | 链接解析 | 歌单搜索 | 歌单歌曲 | 备注             |
| ---------- | -------- | ---- | ---- | ---- | -------- | -------- | -------- | ---------------- |
| 网易云音乐 | netease  | ✅   | ✅   | ✅   | ✅       | ✅       | ✅       |                  |
| QQ 音乐    | qq       | ✅   | ✅   | ✅   | ✅       | ✅       | ✅       |                  |
| 酷狗音乐   | kugou    | ✅   | ✅   | ✅   | ✅       | ✅       | ✅       |                  |
| 酷我音乐   | kuwo     | ✅   | ✅   | ✅   | ✅       | ✅       | ✅       |                  |
| 咪咕音乐   | migu     | ✅   | ✅   | ✅   | ❌       | ✅       | ❌       |                  |
| 千千音乐   | qianqian | ✅   | ✅   | ✅   | ❌       | ❌       | ✅       |                  |
| 汽水音乐   | soda     | ✅   | ✅   | ✅   | ✅       | ✅       | ✅       | 支持音频解密     |
| 5sing      | fivesing | ✅   | ✅   | ✅   | ✅       | ✅       | ✅       |                  |
| Jamendo    | jamendo  | ✅   | ✅   | ❌   | ✅       | ❌       | ❌       |                  |
| JOOX       | joox     | ✅   | ✅   | ✅   | ❌       | ✅       | ❌       |                  |
| Bilibili   | bilibili | ✅   | ✅   | ❌   | ✅       | ❌       | ❌       | 暂未实现歌单功能 |

## 快速开始

### 安装

```bash
go get github.com/guohuiyuan/music-lib
```

### 基本使用

```go
package main

import (
	"fmt"
	"log"

	"github.com/guohuiyuan/music-lib/kugou"
	"github.com/guohuiyuan/music-lib/model"
)

func main() {
	keyword := "周杰伦"

	// 搜索歌曲
	songs, err := kugou.Search(keyword)
	if err != nil {
		log.Fatalf("搜索失败: %v", err)
	}

	if len(songs) == 0 {
		fmt.Println("未找到相关歌曲")
		return
	}

	fmt.Printf("在酷狗音乐找到 %d 首歌曲:\n", len(songs))

	// 获取下载链接
	firstSong := songs[0]
	downloadURL, err := kugou.GetDownloadURL(&firstSong)
	if err != nil {
		log.Fatalf("获取下载链接失败: %v", err)
	}

	fmt.Println("下载链接:", downloadURL)

	// 获取歌词
	lyrics, err := kugou.GetLyrics(&firstSong)
	if err != nil {
		log.Printf("获取歌词失败: %v", err)
	} else {
		fmt.Println("\n歌词:")
		fmt.Println(lyrics)
	}
}
```

### 链接解析

```go
package main

import (
	"fmt"
	"log"

	"github.com/guohuiyuan/music-lib/netease"
)

func main() {
	// 解析网易云音乐分享链接
	link := "https://music.163.com/#/song?id=123456"

	song, err := netease.Parse(link)
	if err != nil {
		log.Fatalf("解析失败: %v", err)
	}

	fmt.Printf("解析成功: %s - %s\n", song.Artist, song.Name)
	fmt.Printf("下载链接: %s\n", song.URL)
}
```

## 架构

```
music-lib/
├── model/                # 数据结构
│   └── song.go          # Song 结构
├── utils/                # 工具
│   ├── file.go          # 文件处理
│   └── request.go       # HTTP 请求
├── provider/             # 接口定义
│   └── interface.go     # MusicProvider 接口
├── netease/              # 网易云音乐
├── qq/                   # QQ 音乐
├── kugou/                # 酷狗音乐
├── kuwo/                 # 酷我音乐
├── migu/                 # 咪咕音乐
├── soda/                 # 汽水音乐
├── bilibili/             # Bilibili
├── fivesing/             # 5sing
├── jamendo/              # Jamendo
├── joox/                 # JOOX
├── qianqian/             # 千千音乐
├── go.mod
└── README.md
```

### 数据模型

```go
type Song struct {
	ID       string            // 歌曲ID
	Name     string            // 歌名
	Artist   string            // 歌手
	Album    string            // 专辑
	AlbumID  string            // 专辑ID
	Duration int               // 时长（秒）
	Size     int64             // 文件大小
	Bitrate  int               // 码率
	Source   string            // 来源平台
	URL      string            // 下载链接
	Ext      string            // 文件后缀
	Cover    string            // 封面链接
	Link     string            // 原始链接
	Extra    map[string]string // 额外数据
}
```

### 接口定义

```go
type MusicProvider interface {
	// Search 搜索歌曲
	Search(keyword string) ([]model.Song, error)

	// Parse 解析分享链接
	Parse(link string) (*model.Song, error)

	// GetDownloadURL 获取下载链接
	GetDownloadURL(s *model.Song) (string, error)

	// GetLyrics 获取歌词
	GetLyrics(s *model.Song) (string, error)
}
```

## 设计

- **高内聚，低耦合**: 每个音乐平台包独立，遵循统一接口
- **单一职责**: 专注于音乐数据获取和处理
- **易于扩展**: 添加新平台只需实现接口

## 使用示例

### 多平台并发搜索

```go
package main

import (
	"fmt"
	"sync"

	"github.com/guohuiyuan/music-lib/kugou"
	"github.com/guohuiyuan/music-lib/netease"
	"github.com/guohuiyuan/music-lib/qq"
)

func main() {
	keyword := "晴天"

	var wg sync.WaitGroup
	var mu sync.Mutex
	var allSongs []model.Song

	// 并发搜索多个平台
	searchFuncs := []func(string) ([]model.Song, error){
		netease.Search,
		qq.Search,
		kugou.Search,
	}

	for _, search := range searchFuncs {
		wg.Add(1)
		go func(fn func(string) ([]model.Song, error)) {
			defer wg.Done()
			songs, err := fn(keyword)
			if err == nil && len(songs) > 0 {
				mu.Lock()
				allSongs = append(allSongs, songs...)
				mu.Unlock()
			}
		}(search)
	}

	wg.Wait()

	fmt.Printf("共找到 %d 首歌曲\n", len(allSongs))
	for _, song := range allSongs {
		fmt.Printf("- %s - %s (%s)\n", song.Artist, song.Name, song.Source)
	}
}
```

## 许可证

本项目基于 [CharlesPikachu/musicdl](https://github.com/CharlesPikachu/musicdl) 的核心设计思路开发，遵循 [PolyForm Noncommercial License 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0) 协议，禁止任何商业使用。

## 免责声明

本项目仅供个人学习和技术研究使用。在使用本库时，请遵守相关法律法规及音乐平台用户协议。通过本库获取的资源，请在 24 小时内删除。
