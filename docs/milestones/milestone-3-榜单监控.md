# Milestone 3 — 榜单监控

> 状态：待开始
> 前置条件：Milestone 1 完成（SQLite 是去重的基础）；Milestone 2 可并行
> 预计工作量：中高（新功能，含调度器）

---

## 这个 Milestone 解决什么问题

用户想让 NAS 自动跟上榜单更新，不需要每天手动刷榜。

现在的状态：用户要发现新歌只能自己搜。这个 Milestone 完成后：配置一张榜单，系统每 6/12/24 小时自动抓取 Top N，下载没有的歌，已有的直接跳过。

---

## 平台范围（明确限制）

**只做 3 个平台，共 8 张榜单：**

| 平台 | 榜单 | API 来源 |
|------|------|---------|
| 网易云 | 飙升榜、热歌榜、新歌榜 | `playlist/detail?id=...`（公开 playlist ID）|
| QQ 音乐 | 热歌榜、新歌榜、巅峰榜 | `/top/detail`（现有 GetRecommendedPlaylists 扩展）|
| 酷狗 | 飙升榜、热歌榜 | 现有 kugou provider 扩展 |

其他 8 个平台（Bilibili/5sing/Jamendo 等）：**明确不做**，无公开榜单 API。

---

## 交付范围

### 1. 数据模型

```go
// 监控规则
type Monitor struct {
    ID       uint   `gorm:"primaryKey"`
    Name     string // 用户自定义名称，如"网易热歌榜"
    Platform string // netease/qq/kugou
    ChartID  string // 平台内部榜单 ID
    TopN     int    // 下载 Top N，默认 20，最大 100
    Interval int    // 间隔小时数：6/12/24
    Enabled  bool   `gorm:"default:true"`
    LastRunAt  *time.Time
    NextRunAt  time.Time
    CreatedAt  time.Time
    UpdatedAt  time.Time
}

// 监控执行历史
type MonitorRun struct {
    ID          uint `gorm:"primaryKey"`
    MonitorID   uint
    StartedAt   time.Time
    FinishedAt  *time.Time
    TotalFetched int  // 抓取到的榜单歌曲数
    NewQueued   int  // 新加入下载队列的歌曲数
    Skipped     int  // 去重跳过的歌曲数
    Status      string // running/done/failed
    Error       string
}
```

### 2. 去重逻辑

去重键：`platform + song_id`

从 `DownloadTask` 表查询：如果该 (platform, song_id) 已有 status=done 的记录，跳过。

不使用文件名去重（不可靠），不使用哈希（过重）。

### 3. Chart Provider 接口扩展

在 `provider/interface.go` 新增：

```go
type Chart struct {
    ID   string
    Name string
}

// GetCharts 返回该平台支持的所有榜单
GetCharts() ([]Chart, error)

// GetChartSongs 返回指定榜单的 Top N 歌曲
GetChartSongs(chartID string, limit int) ([]Song, error)
```

每个平台实现自己的 charts 方法：
- `netease/charts.go`
- `qq/charts.go`
- `kugou/charts.go`

### 4. 调度器

使用 `github.com/robfig/cron/v3` 或直接用 `time.Ticker`（后者更简单）：

调度策略：
- 系统启动时加载所有 `enabled=true` 的 Monitor
- 按 NextRunAt 排序，用一个 goroutine 轮询
- 执行完成后更新 LastRunAt + 计算下一个 NextRunAt
- 新增/修改 Monitor 时动态注册到调度器

不引入 Cron 表达式（过度设计），interval 只支持 6/12/24 三档。

### 5. API

```
GET  /api/monitors              # 列出所有监控规则
POST /api/monitors              # 新建监控规则
PUT  /api/monitors/:id          # 修改（包括 enabled 状态）
DELETE /api/monitors/:id        # 删除
GET  /api/monitors/:id/runs     # 执行历史
POST /api/monitors/:id/trigger  # 手动立即触发一次

GET  /api/charts?platform=netease   # 获取平台支持的榜单列表（前端用）
```

### 6. 下载集成

Monitor 触发后，将新歌塞入现有下载队列（`download.Manager`），沿用所有现有逻辑（质量降级、NAS 路径、刮削）。Monitor 不绕过任何现有机制。

---

## 验收条件

以下全部满足才算完成：

1. 通过 API 创建一个网易热歌榜监控（TopN=5，Interval=6）
2. 手动触发 → 5 首歌进入下载队列
3. 再次手动触发 → 0 首新任务（已有记录，全部去重跳过）
4. 删除 2 首歌的下载记录 → 再触发 → 2 首重新入队
5. 系统重启 → 监控规则和历史记录保留

---

## 不在此 Milestone 范围内

- 自定义歌手/歌单追踪（P1，下轮再议）
- 下载结果的邮件/通知推送
- Bilibili/5sing/Jamendo 榜单（无公开 API，明确不做）
- Cron 表达式自定义调度（6/12/24h 三档已满足 90% 需求）

---

## 关键技术决策

| 决策点 | 选择 | 原因 |
|--------|------|------|
| 调度方式 | `time.Ticker` + goroutine | 避免引入 cron 依赖，3 档间隔不需要表达式 |
| 去重键 | `(platform, song_id)` | 唯一可靠标识，文件名不可靠 |
| 榜单范围 | 网易 + QQ + 酷狗 | 3 个已有 provider 且有已知 API，其余无公开榜单 |
| 执行历史保留期 | 最近 100 条/每个 Monitor | 避免数据库无限膨胀 |
