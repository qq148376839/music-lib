# music-lib

music-lib 是个用 Go 写的音乐库。

它没有 UI，主要帮你解决各个音乐平台的数据接口问题——不管是搜索、解析还是下载。如果你想自己写个音乐下载器或者播放器，用它正好。

同时提供 HTTP API 服务，Docker 部署后即可直接通过接口调用。

## 主要功能

支持网易云、QQ、酷狗、酷我这些主流平台，也能搞定汽水音乐、5sing 这些。具体支持情况如下：

| 平台       | 包名         | 搜索 | 下载 | 歌词 | 歌曲解析 | 歌单搜索 | 歌单推荐 | 歌单歌曲 | 歌单链接解析 | 备注     |
| :--------- | :----------- | :--: | :--: | :--: | :------: | :------: | :------: | :------: | :----------: | :------- |
| 网易云音乐 | `netease`  |  O  |  O  |  O  |    O    |    O    |    O    |    O    |      O      |          |
| QQ 音乐    | `qq`       |  O  |  O  |  O  |    O    |    O    |    O    |    O    |      O      |          |
| 酷狗音乐   | `kugou`    |  O  |  O  |  O  |    O    |    O    |    O    |    O    |      O      |          |
| 酷我音乐   | `kuwo`     |  O  |  O  |  O  |    O    |    O    |    O    |    O    |      O      |          |
| 咪咕音乐   | `migu`     |  O  |  O  |  O  |    O    |    O    |    X    |    O    |      X      |          |
| 千千音乐   | `qianqian` |  O  |  O  |  O  |    O    |    O    |    X    |    O    |      X      |          |
| 汽水音乐   | `soda`     |  O  |  O  |  O  |    O    |    O    |    X    |    O    |      O      | 音频解密 |
| 5sing      | `fivesing` |  O  |  O  |  O  |    O    |    O    |    X    |    O    |      O      |          |
| Jamendo    | `jamendo`  |  O  |  O  |  X  |    O    |    O    |    X    |    O    |      X      |          |
| JOOX       | `joox`     |  O  |  O  |  O  |    O    |    O    |    X    |    O    |      X      |          |
| Bilibili   | `bilibili` |  O  |  O  |  X  |    O    |    O    |    X    |    O    |      O      |          |

## Docker 部署

### 构建镜像

```bash
docker build -t music-lib .
```

### 运行容器

```bash
docker run -d -p 35280:35280 --name music-lib music-lib
```

自定义端口：

```bash
docker run -d -p 3000:3000 -e PORT=3000 --name music-lib music-lib
```

### 启用 NAS 下载

设置 `MUSIC_DIR` 环境变量后，Web UI 中会出现"下载到 NAS"选项：

```bash
docker run -d -p 35280:35280 \
  -e MUSIC_DIR=/music \
  -v /your/nas/music:/music \
  --name music-lib music-lib
```

可选环境变量：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PORT` | `35280` | 服务端口 |
| `MUSIC_DIR` | 未设置（NAS 禁用） | 音乐文件存储目录 |
| `DOWNLOAD_CONCURRENCY` | `3` | NAS 并发下载数 |
| `WEB_DIR` | `web` | 前端静态文件目录 |

### 跨平台回退下载

NAS 批量下载时，部分歌曲可能因 VIP/版权限制无法从原始平台获取下载链接。系统会自动在其他平台搜索同名歌曲并尝试下载，无需手动干预。

回退搜索按以下优先级遍历平台：酷狗 → 酷我 → 咪咕 → QQ → 千千 → 汽水 → 5sing → JOOX → Bilibili → Jamendo

匹配规则：
- 歌名归一化后完全相等，且歌手名存在包含关系
- 歌名有一方包含另一方，且歌手名完全匹配（处理 "Song (Remastered)" vs "Song"）

回退成功后文件仍以原始歌曲元数据（歌手/歌名/专辑）命名保存，保持歌单结构一致。前端任务列表中回退下载的歌曲会显示来源标签，如 `网易云 → 酷狗`。

### 验证服务

```bash
curl http://localhost:35280/health
```

## HTTP API

所有接口统一返回格式：

```json
{
  "code": 0,
  "data": ...
}
```

错误时：

```json
{
  "code": -1,
  "message": "错误信息"
}
```

### 基础接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/health` | 健康检查 |
| GET | `/providers` | 列出所有支持的平台及其功能 |

### 歌曲接口

| 方法 | 路径 | 参数 | 说明 |
|------|------|------|------|
| GET | `/api/search` | `source`, `keyword` | 搜索歌曲 |
| POST | `/api/lyrics` | `source` + Body(Song JSON) | 获取歌词 |
| GET | `/api/parse` | `source`, `link` | 解析歌曲链接 |

### 歌单接口

| 方法 | 路径 | 参数 | 说明 |
|------|------|------|------|
| GET | `/api/playlist/search` | `source`, `keyword` | 搜索歌单 |
| GET | `/api/playlist/songs` | `source`, `id` | 获取歌单内歌曲列表 |
| GET | `/api/playlist/parse` | `source`, `link` | 解析歌单链接 |
| GET | `/api/playlist/recommended` | `source` | 获取推荐歌单 |

### 下载接口

| 方法 | 路径 | 参数 | 说明 |
|------|------|------|------|
| POST | `/api/download/file` | `source` + Body(Song JSON) | 代理下载歌曲文件（浏览器下载） |
| GET | `/api/nas/status` | — | 查询 NAS 下载功能是否启用 |
| POST | `/api/nas/download` | `source` + Body(Song JSON) | 单曲下载到 NAS |
| POST | `/api/nas/download/batch` | `source` + Body(playlist JSON) | 批量下载歌单到 NAS |
| GET | `/api/nas/tasks` | — | 列出所有 NAS 下载任务 |
| GET | `/api/nas/task` | `id` | 查询单个任务状态 |
| GET | `/api/nas/batches` | — | 列出批量下载批次汇总 |

### 调用示例

**搜索歌曲：**

```bash
curl "http://localhost:35280/api/search?source=netease&keyword=周杰伦"
```

**浏览器下载歌曲文件：**

```bash
curl -X POST "http://localhost:35280/api/download/file?source=kugou" \
  -H "Content-Type: application/json" \
  -d '{"id":"hash_value","source":"kugou","extra":{"hash":"hash_value"}}' \
  -o song.mp3
```

**解析歌曲链接：**

```bash
curl "http://localhost:35280/api/parse?source=netease&link=https://music.163.com/song?id=123456"
```

**获取推荐歌单：**

```bash
curl "http://localhost:35280/api/playlist/recommended?source=kuwo"
```

**搜索歌单：**

```bash
curl "http://localhost:35280/api/playlist/search?source=qq&keyword=流行"
```

**获取歌单歌曲：**

```bash
curl "http://localhost:35280/api/playlist/songs?source=netease&id=123456"
```

## 作为 Go 库使用

直接 `go get`：

```bash
go get github.com/guohuiyuan/music-lib
```

### 1. 搜歌 + 下载

```go
package main

import (
	"fmt"
	"log"

	"github.com/guohuiyuan/music-lib/kugou"
)

func main() {
	songs, err := kugou.Search("周杰伦")
	if err != nil {
		log.Fatal(err)
	}
	if len(songs) == 0 {
		fmt.Println("没找到相关歌曲")
		return
	}

	// 拿第一首的下载地址
	url, err := kugou.GetDownloadURL(&songs[0])
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("下载地址:", url)
}
```

### 2. 获取推荐歌单

```go
package main

import (
	"fmt"
	"log"
	"github.com/guohuiyuan/music-lib/netease"
)

func main() {
	playlists, err := netease.GetRecommendedPlaylists()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("拿到 %d 个推荐歌单：\n", len(playlists))
	for _, p := range playlists {
		fmt.Printf("- %s (ID: %s)\n", p.Name, p.ID)
	}
}
```

### 3. 解析歌单链接

```go
package main

import (
	"fmt"
	"log"
	"github.com/guohuiyuan/music-lib/netease"
)

func main() {
	link := "https://music.163.com/#/playlist?id=123456"
	playlist, songs, err := netease.ParsePlaylist(link)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s 共有 %d 首歌\n", playlist.Name, len(songs))
}
```

## 设计思路

- **独立性**：你可以只引 `netease` 包，别的包不会进去污染你的依赖。
- **统一性**：不管用哪个包，返回的 `Song` 和 `Playlist` 结构都是一样的，切换源的时候不用改业务逻辑。
- **扩展性**：如果要加新平台，照着 `provider` 接口实现一遍就行。

## 目录结构

```
music-lib/
├── cmd/server/  # HTTP API 服务入口
├── model/       # 通用数据结构
├── download/    # NAS 下载管理（任务队列、跨平台回退）
├── provider/    # 接口定义
├── utils/       # 公共工具函数
├── netease/     # 各个平台的实现
├── qq/
├── kugou/
├── kuwo/
├── migu/
├── qianqian/
├── soda/
├── fivesing/
├── jamendo/
├── joox/
├── bilibili/
├── web/         # 前端静态文件
├── Dockerfile   # Docker 构建文件
└── README.md
```

## 许可证

本项目遵循 GNU Affero General Public License v3.0（AGPL-3.0）。详情见 [LICENSE](LICENSE)。

## 免责声明

这个库就是写着玩、学技术的。大家用的时候遵守一下法律法规，不要拿去商用。下载的资源 24 小时内删掉。
