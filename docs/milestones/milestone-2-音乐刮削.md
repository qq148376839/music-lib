# Milestone 2 — 音乐刮削

> 状态：待开始
> 前置条件：Milestone 1 完成（Gin + SQLite + Docker）
> 预计工作量：中高（核心新功能）

---

## 这个 Milestone 解决什么问题

用户下载的音乐文件是"哑文件"——文件名可能是乱码，在 Plex/Navidrome/本地播放器里显示为"未知艺术家 - 未知专辑"。没有封面、没有歌词嵌入、没有任何元数据。

这个 Milestone 完成后：任意下载一首歌，文件里自动有：
- 正确的 ID3 标签（标题、艺术家、专辑、年份、流派）
- 专辑封面图（嵌入文件内）
- 歌词（LRC 格式嵌入 + 同目录 .lrc 文件）

---

## 交付范围

### 1. 刮削触发机制

下载完成后自动触发刮削，作为下载管道的最后一步：

```
下载文件 → 写入磁盘 → [刮削] → 更新任务状态
                        ↑
                   scraper.Scrape(song, filePath)
```

刮削失败不影响下载任务本身的成功状态，但记录 scrape_error 字段。

### 2. ID3 标签写入

使用 `github.com/bogem/id3v2`（纯 Go，支持 ID3v2.4）：

写入字段：
- TIT2 - 歌曲标题
- TPE1 - 艺术家
- TALB - 专辑
- TDRC - 年份
- TCON - 流派（若平台提供）
- APIC - 专辑封面（二进制嵌入）
- USLT - 歌词文本（非 LRC 同步歌词）

数据来源策略：
- **优先**：下载来源平台的元数据（Song 结构体里已有的字段）
- **补充**：若 Artist/Album 为空，用 Song.Name 拆分或留空，不跨平台再搜索

支持格式：mp3（ID3v2）、flac（Vorbis Comment，使用 `github.com/mewkiz/flac`）

> 注：aac/m4a 使用 MP4 box 写入（待 M2 阶段调研确认库选择）

### 3. 封面下载

下载流程：
1. 读取 Song.CoverURL
2. HTTP GET 封面图（带超时 10s，User-Agent 模拟浏览器）
3. 图片格式检测（JPEG/PNG），JPEG 优先
4. 嵌入 ID3 APIC frame

封面存储：
- 嵌入音频文件（主要）
- 同时写入 `{albumDir}/cover.jpg`（供 Plex/Navidrome 的目录扫描）

失败处理：封面下载失败 = 记录日志，跳过，不阻断整个刮削

### 4. 歌词处理

调用现有 `GetLyrics` 接口获取歌词，然后：

1. LRC 格式检测（是否含时间轴 `[mm:ss.xx]`）
2. 写入同目录 `.lrc` 文件（`songTitle - artist.lrc`）
3. 同时嵌入 ID3 USLT（纯文本，去掉时间轴）

平台支持现状（从现有代码推断）：
- netease、qq、kugou、kuwo 的 GetLyrics 已实现
- migu、qianqian、soda 需确认是否返回 LRC 格式

### 5. 刮削配置

新增环境变量：

```
SCRAPE_ENABLED=true      # 是否启用刮削（默认 true）
SCRAPE_COVER=true        # 是否下载封面（默认 true）
SCRAPE_LYRICS=true       # 是否获取歌词（默认 true）
```

### 6. DownloadTask 表扩展

在 M1 的 `DownloadTask` 基础上增加：

```go
ScrapeStatus string // pending/done/failed/skipped
ScrapeError  string
ScrapedAt    *time.Time
```

---

## 验收条件

以下全部满足才算完成：

1. 下载网易云一首有封面的 MP3 歌曲 → 在 macOS 访达预览中看到专辑封面
2. 同一首歌在 VLC/系统播放器中，"歌手"和"专辑"字段显示正确（非"未知"）
3. 同目录下存在对应的 `.lrc` 文件（若平台支持歌词）
4. 刮削失败（如 CoverURL 为空）不导致下载任务标记失败
5. `SCRAPE_ENABLED=false` 时跳过刮削，文件仍正常保存

---

## 不在此 Milestone 范围内

- 跨平台多源打分刮削（在 M3 之后的 P1 迭代考虑）
- 已存在文件的批量补刮削（独立功能，后续再做）
- Acoustid 音频指纹（明确删除，见 ROADMAP）
- 非 mp3/flac 格式的完整支持（m4a/aac 留 TODO）

---

## 关键技术决策

| 决策点 | 选择 | 原因 |
|--------|------|------|
| ID3 库 | `github.com/bogem/id3v2` | 纯 Go，支持写入，活跃维护 |
| FLAC 标签 | `github.com/mewkiz/flac` 或调研替代 | 执行前 WebSearch 确认最新 Go FLAC 写入支持 |
| 刮削时机 | 下载完成后同步执行 | 异步队列过度设计，刮削耗时 < 2s |
| 封面分辨率 | 接受任意尺寸，不强制压缩 | 避免依赖图像处理库 |
