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
docker run -d -p 8080:8080 --name music-lib music-lib
```

自定义端口：

```bash
docker run -d -p 3000:3000 -e PORT=3000 --name music-lib music-lib
```

### 验证服务

```bash
curl http://localhost:8080/health
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
| POST | `/api/download` | `source` + Body(Song JSON) | 获取下载链接 |
| POST | `/api/lyrics` | `source` + Body(Song JSON) | 获取歌词 |
| GET | `/api/parse` | `source`, `link` | 解析歌曲链接 |

### 歌单接口

| 方法 | 路径 | 参数 | 说明 |
|------|------|------|------|
| GET | `/api/playlist/search` | `source`, `keyword` | 搜索歌单 |
| GET | `/api/playlist/songs` | `source`, `id` | 获取歌单内歌曲列表 |
| GET | `/api/playlist/parse` | `source`, `link` | 解析歌单链接 |
| GET | `/api/playlist/recommended` | `source` | 获取推荐歌单 |

### 调用示例

**搜索歌曲：**

```bash
curl "http://localhost:8080/api/search?source=netease&keyword=周杰伦"
```

**获取下载链接：**

```bash
curl -X POST "http://localhost:8080/api/download?source=kugou" \
  -H "Content-Type: application/json" \
  -d '{"id":"hash_value","source":"kugou","extra":{"hash":"hash_value"}}'
```

**解析歌曲链接：**

```bash
curl "http://localhost:8080/api/parse?source=netease&link=https://music.163.com/song?id=123456"
```

**获取推荐歌单：**

```bash
curl "http://localhost:8080/api/playlist/recommended?source=kuwo"
```

**搜索歌单：**

```bash
curl "http://localhost:8080/api/playlist/search?source=qq&keyword=流行"
```

**获取歌单歌曲：**

```bash
curl "http://localhost:8080/api/playlist/songs?source=netease&id=123456"
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
├── Dockerfile   # Docker 构建文件
└── README.md
```

## 许可证

本项目遵循 GNU Affero General Public License v3.0（AGPL-3.0）。详情见 [LICENSE](LICENSE)。

## 免责声明

这个库就是写着玩、学技术的。大家用的时候遵守一下法律法规，不要拿去商用。下载的资源 24 小时内删掉。
