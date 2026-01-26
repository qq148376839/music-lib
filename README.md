# Music Library

一个Go语言编写的音乐搜索和下载库，设计简洁，可以作为第三方库在其他项目中使用。

## 主要功能

- **纯函数式接口**：输入参数，返回数据，不包含UI逻辑
- **模块化设计**：每个音乐源独立为一个包，方便维护和扩展
- **标准化接口**：所有音乐源都使用统一的`model.Song`结构体
- **易于测试**：没有控制台输出和用户交互，方便编写单元测试
- **VIP过滤**：自动过滤VIP和付费歌曲，只返回免费可下载的
- **统一歌词接口**：所有平台都实现了`GetLyrics`方法，支持歌词获取
- **音频解密**：支持汽水音乐加密音频的解密
- **问题修复**：解决了酷我音乐多次下载出现示例音乐的问题

## 当前已知问题

1. QQ音乐源有些歌曲下载不了
2. 汽水音乐部分音频文不对版（非VIP用户只能得到音频切片）
3. Bilibili下载需要配置Cookie才能获取高音质音频

## 已解决的问题

- 酷我音乐多次下载出现示例音乐的问题 ✓
- 咪咕音乐展示问题 ✓
- Jamendo下载问题 ✓
- Bilibili下载问题 ✓
- 添加了歌词接口支持（酷我音乐源）✓
- 添加了汽水音乐加密音频解密功能 ✓
- 统一了所有平台的歌词接口 ✓

## 支持的音乐平台

- **网易云音乐**：国内主流平台，曲库丰富，有很多原创和独立音乐人作品
- **QQ音乐**：腾讯旗下，华语流行音乐版权多
- **酷狗音乐**：老牌平台，曲库量大，K歌功能强
- **酷我音乐**：提供高品质音乐，支持无损格式
- **咪咕音乐**：中国移动旗下，正版资源多
- **5sing原创音乐**：专注原创音乐和翻唱，适合找独立音乐人作品
- **Jamendo**：国际免费音乐平台，所有音乐都可免费下载使用
- **JOOX音乐**：腾讯国际版，主要面向东南亚
- **千千音乐**：百度旗下，整合了百度音乐资源
- **汽水音乐**：字节跳动旗下，个性化推荐做得好
- **Bilibili音频**：从B站视频提取音频，二次创作和同人音乐多

## 项目结构

```
music-lib/
├── go.mod
├── model/                # 通用数据结构
│   └── song.go
├── utils/                # 基础工具
│   └── request.go
├── bilibili/             # Bilibili音频源
│   └── bilibili.go
├── fivesing/             # 5sing原创音乐源
│   └── fivesing.go
├── jamendo/              # Jamendo免费音乐源
│   └── jamendo.go
├── joox/                 # JOOX音乐源
│   └── joox.go
├── kugou/                # 酷狗音乐源
│   └── kugou.go
├── kuwo/                 # 酷我音乐源
│   └── kuwo.go
├── migu/                 # 咪咕音乐源
│   └── migu.go
├── netease/              # 网易云音乐源
│   ├── crypto.go         # 加密算法
│   └── netease.go
├── qianqian/             # 千千音乐源
│   └── qianqian.go
├── qq/                   # QQ音乐源
│   └── qq.go
└── soda/                 # 汽水音乐源
    └── soda.go
```

## 安装

```bash
go get github.com/guohuiyuan/music-lib
```

## 使用示例

### 基本使用

```go
package main

import (
	"fmt"
	"log"
	
	"github.com/guohuiyuan/music-lib/kugou"
)

func main() {
	keyword := "周杰伦"
	
	// 搜索歌曲
	songs, err := kugou.Search(keyword)
	if err != nil {
		log.Fatalf("搜索失败: %v", err)
	}

	fmt.Printf("找到 %d 首歌\n", len(songs))
	
	// 显示前3条结果
	for i, song := range songs {
		if i >= 3 {
			break
		}
		fmt.Printf("%d. %s - %s (时长: %d秒)\n", i+1, song.Name, song.Artist, song.Duration)
	}

	// 获取下载链接
	if len(songs) > 0 {
		target := &songs[0]
		url, err := kugou.GetDownloadURL(target)
		if err != nil {
			log.Printf("获取链接失败: %v", err)
		} else {
			fmt.Println("下载地址:", url)
		}
	}
}
```

### 在其他项目中引用

```go
// 使用者的 go.mod
module my-app
go 1.25

require github.com/guohuiyuan/music-lib v1.0.1
```

## API 文档（部分）

### kugou 包

**搜索歌曲**
```go
func Search(keyword string) ([]model.Song, error)
```
使用移动端API搜索酷狗音乐，自动选择最佳音质。

**获取下载链接**
```go
func GetDownloadURL(s *model.Song) (string, error)
```
获取单首歌曲的下载地址，需要正确的User-Agent和Referer。

**获取歌词**
```go
func GetLyrics(s *model.Song) (string, error)
```
返回LRC格式的歌词字符串。

### qq 包

**搜索歌曲**
```go
func Search(keyword string) ([]model.Song, error)
```
使用QQ音乐API搜索，自动过滤VIP和付费歌曲。

**获取下载链接**
```go
func GetDownloadURL(s *model.Song) (string, error)
```
获取下载地址，支持128k MP3和M4A音质。

### netease 包

**搜索歌曲**
```go
func Search(keyword string) ([]model.Song, error)
```
使用网易云音乐Linux API搜索，参数经过AES-ECB加密。

**获取下载链接**
```go
func GetDownloadURL(s *model.Song) (string, error)
```
使用WeApi获取下载地址，支持320kbps码率。

### soda 包

**搜索歌曲**
```go
func Search(keyword string) ([]model.Song, error)
```
搜索汽水音乐，返回完整的歌曲元数据。

**解密音频**
```go
func DecryptAudio(fileData []byte, playAuth string) ([]byte, error)
```
解密汽水音乐下载的加密音频数据，使用AES-CTR算法。

## 设计原则

1. **高内聚**：每个包只负责一个明确的功能
2. **低耦合**：包之间通过标准接口通信，减少依赖
3. **可测试性**：纯函数设计，方便单元测试
4. **可扩展性**：新的音乐源只需实现相同的接口就能集成

## 注意事项

- 有些歌曲可能需要VIP权限才能下载
- API可能会变更，需要定期维护
- 仅供学习和研究使用

## 与 go-music-dl 集成

这个库是[go-music-dl](https://github.com/guohuiyuan/go-music-dl)项目的核心依赖。go-music-dl是一个完整的Go项目，把命令行和Web服务合在一起。

### 主要功能
- **双模式运行**：支持命令行交互和Web服务
- **现代化界面**：
  - 命令行：用Bubble Tea做交互式表格
  - Web：用Gin + Tailwind CSS做网页，还有Live2D看板娘
- **完整元数据**：显示歌曲时长、大小、专辑、封面等信息
- **统一文件命名**：下载文件自动命名为`歌手 - 歌名.mp3`
- **歌词支持**：内置音乐播放器，支持在线试听和歌词同步
- **音频解密**：支持汽水音乐加密音频的解密和播放
- **封面下载**：可以同时下载歌曲封面图片

### 快速开始
```bash
# 安装 go-music-dl
git clone https://github.com/guohuiyuan/go-music-dl.git
cd go-music-dl
go build -o music-dl ./cmd/music-dl

# 命令行模式
./music-dl -k "周杰伦"

# Web模式
./music-dl web
```

## 许可证

GNU Affero General Public License v3.0