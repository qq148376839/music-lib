# Milestone 1 — 基建重构

> 状态：待开始
> 前置条件：无
> 预计工作量：中（纯重构，无新功能）

---

## 这个 Milestone 解决什么问题

现有系统有两个生产级致命伤：

1. **任务历史在内存里**：重启 = 所有下载记录消失，用户不知道哪些歌下了哪些没下
2. **net/http 平铺路由**：加一个 Monitor CRUD 就要新增 10+ handler，没有 group/middleware，CORS、日志中间件要手写

这个 Milestone 完成后，后续所有功能（刮削、监控）都有稳固基础可以构建。

---

## 交付范围

### 1. Gin 框架迁移

将 `cmd/server/main.go` 从 `net/http` + `mux` 迁移到 Gin：

- 所有现有 API 路由保持 URL 和行为不变（不破坏前端）
- 提取公共中间件：CORS、请求日志、错误恢复
- 路由分组：`/api/search`、`/api/playlist`、`/api/login`、`/api/nas`

文件结构：
```
cmd/server/
  main.go          # 仅启动，不含路由定义
api/
  router.go        # Gin engine + 路由注册
  handler_search.go
  handler_playlist.go
  handler_login.go
  handler_nas.go
```

### 2. SQLite + GORM 持久化

引入 `gorm.io/gorm` + `gorm.io/driver/sqlite`：

**需要持久化的数据：**

```go
// 下载任务
type DownloadTask struct {
    ID         uint   `gorm:"primaryKey"`
    Source     string // 来源平台 netease/qq/...
    SongID     string
    Title      string
    Artist     string
    Album      string
    Quality    string // lossless/high/standard
    FilePath   string // 实际存储路径
    Status     string // pending/downloading/done/failed
    Error      string
    BatchID    string // 关联批量任务
    CreatedAt  time.Time
    UpdatedAt  time.Time
}

// 批量任务（歌单下载）
type DownloadBatch struct {
    ID        string `gorm:"primaryKey"` // UUID
    Name      string // 歌单名或用户备注
    Total     int
    Done      int
    Failed    int
    Status    string
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

迁移策略：
- `download/task.go` 中的内存 map 替换为 GORM 操作
- 已有的 API（`/api/nas/tasks`、`/api/nas/batches`）行为不变

数据库文件位置：`$DATA_DIR/music-lib.db`（Docker volume 挂载路径）

### 3. Docker 多阶段构建

```
Dockerfile          # 多阶段：go build → alpine
docker-compose.yml  # 含 volumes 定义
```

`docker-compose.yml` 最终形态：
```yaml
services:
  music-lib:
    build: .
    ports:
      - "35280:35280"
    environment:
      - MUSIC_DIR=/music
      - DATA_DIR=/data
      - PORT=35280
    volumes:
      - /nas/music:/music      # 音乐文件存储
      - /nas/config:/data      # 配置 + 数据库
    restart: unless-stopped
```

`Dockerfile` 使用 CGO_ENABLED=0，最终镜像基于 `alpine:3.20`，引入 ca-certificates。

注意：SQLite with GORM 的 `gorm.io/driver/sqlite` 需要 CGO。使用 `modernc.org/sqlite`（纯 Go SQLite，无需 CGO）替代。

### 4. 结构化日志

将 `log.Printf` 全部替换为 `slog`（Go 1.21 stdlib）：

```go
// 格式：JSON，level 通过 LOG_LEVEL 环境变量控制
slog.Info("download started", "task_id", task.ID, "song", task.Title, "source", task.Source)
slog.Error("download failed", "task_id", task.ID, "error", err)
```

---

## 验收条件

以下全部满足才算完成：

1. `docker-compose up --build` 在全新目录下成功启动，访问 `localhost:35280` 看到 UI
2. 发起一个下载任务，然后 `docker-compose restart`，重启后 `/api/nas/tasks` 仍能查到该任务记录
3. 所有现有 API（search/playlist/login/download）行为与重构前一致
4. Docker 镜像大小 < 50MB

---

## 不在此 Milestone 范围内

- 任何新功能（刮削、监控）
- 数据库查询优化
- API 版本控制

---

## 关键技术决策

| 决策点 | 选择 | 原因 |
|--------|------|------|
| SQLite driver | `modernc.org/sqlite` | 纯 Go，CGO_ENABLED=0，Docker 构建简单 |
| Gin 版本 | 最新 stable (1.9.x) | 执行前 WebSearch 确认 |
| 日志级别控制 | `LOG_LEVEL` 环境变量 | NAS 部署友好，不需要重编译 |
