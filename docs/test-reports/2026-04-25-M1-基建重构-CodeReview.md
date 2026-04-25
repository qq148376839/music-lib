# M1 基建重构 -- Code Review 报告

> 文档编号：docs/test-reports/2026-04-25-M1-基建重构-CodeReview.md
> 作者：质检官 (code-reviewer)
> 类型：Code Review
> 结论：**不通过**

---

## 1. 概述

### 1.1 检查范围

M1 基建重构全部新增/修改文件，共 19 个文件：

| 模块 | 文件 | 类型 |
|------|------|------|
| internal/store | db.go, task.go, batch.go | 新增 |
| internal/api | router.go, middleware.go, handler_search.go, handler_playlist.go, handler_login.go, handler_nas.go, embed.go, provider.go | 新增 |
| download | task.go (重写), provider.go (新增) | 修改/新增 |
| cmd/server | main.go (重写), providers.go (新增) | 修改/新增 |
| web | embed.go | 新增 |
| 根目录 | Dockerfile, docker-compose.yml, go.mod | 修改 |
| 未动但被调用 | download/fallback.go, download/writer.go, login/manager.go | 接口兼容性检查 |

### 1.2 对照文档

`docs/2026-04-24-基建重构.md` -- 唯一真相源

### 1.3 结论摘要

- 总问题数：14（Critical: 3, Major: 5, Minor: 4, Info: 2）
- 通过条件：Critical = 0 且 Major <= 2
- 结论：**不通过**（Critical = 3, Major = 5）

---

## 2. 问题清单

### Critical（必须修复，阻塞发布）

| # | 问题 | 位置 | 描述 | 修复建议 |
|---|------|------|------|----------|
| C1 | notifyUpdate 在持锁状态下启动 goroutine 导致并发竞态 | `download/task.go:132-138` | `notifyUpdate` 方法注释写"Must be called while holding m.mu (write lock)"，方法内部做了 `t := *task` 浅拷贝后 `go m.onTaskUpdate(&t)`。问题在于 `Task.Song` 包含 `Extra map[string]string`，浅拷贝不会深拷贝这个 map。如果回调 `onTaskUpdate`（即 `store.SaveTask`）在读 `t.Song.Extra["quality"]` 的同时，另一个 goroutine 正在修改原 task 的 `Song.Extra`（例如 handler 里 `song.Extra["quality"] = quality` 后传入 `Enqueue`），就是并发 map 读写。虽然当前业务路径下 Enqueue 传入的 song 不会被并发修改，但这是一个隐含的、未文档化的线程安全假设。更关键的是：`m.mu` 锁保护了 task 字段的修改，但 `go m.onTaskUpdate(&t)` 让回调在锁外执行，如果 `store.SaveTask` 耗时较长（如 SQLite WAL 刷盘），多个 notifyUpdate goroutine 可能并发执行，导致旧状态覆盖新状态的 DB 记录（后写入的 goroutine 携带的是更早的快照）。 | 方案 A：移除 `go`，让 `onTaskUpdate` 在锁内同步执行（代价：锁持有时间增加）。方案 B：引入一个单 goroutine 的写入队列（channel），保证 DB 写入顺序与状态变更顺序一致。推荐 B，因为既解耦锁又保序。 |
| C2 | handleListBatches 重启后丢失 batch 名称和数据 | `download/task.go:245-298` + `internal/api/handler_nas.go:218-224` | `ListBatches()` 完全基于内存 `m.tasks` 聚合。`EnqueueBatch` 时创建的 synthetic batch entry（`m.tasks[batchID]`，第 186-191 行）不会被 `onTaskUpdate` 回调持久化（它不是真正的 task），也不会被 `LoadTasks` 恢复。重启后 synthetic entry 丢失，`ListBatches` 查不到任何 batch。设计文档 S5.1 明确说"done/failed/running/pending 的实时计数从 download_tasks 按 batch_id 聚合"，当前实现违反了这一设计。 | `handleListBatches` 改为从 DB 聚合：`SELECT batch_id, COUNT(*) as total, SUM(CASE WHEN status='done' THEN 1 ELSE 0 END) as done, ...` JOIN `download_batches` 获取 name。内存中的 synthetic batch entry 可保留用于不查 DB 的快速路径，但 handler 必须有 DB 回退。 |
| C3 | EnqueueBatch 中 synthetic batch entry 不在 `order` 中但在 `tasks` 中，导致 GetTask 能查到"假任务" | `download/task.go:184-192` | synthetic batch entry 被写入 `m.tasks[batchID]` 但未加入 `m.order`。`ListTasks` 按 `m.order` 遍历所以不会返回它（正确），但 `GetTask(batchID)` 会返回这个 status=done、Song.Name=batchName 的假任务（错误）。前端调用 `GET /api/nas/task?id=b-xxx` 会得到一个不是真实下载任务的对象，字段混乱（没有 Artist/Album/FilePath，Source 有值但没有实际下载行为）。这违反了 API 语义一致性。 | 方案 A：将 batch 信息存储在单独的 `batches map[string]BatchMeta` 中，不污染 `tasks` map。方案 B：`GetTask` 过滤掉 `b-` 前缀的 ID。推荐 A，干净隔离。 |

### Major（应该修复，影响质量）

| # | 问题 | 位置 | 描述 | 修复建议 |
|---|------|------|------|----------|
| M1 | `isRetryable` 基于 error 字符串匹配，脆弱且不完整 | `download/task.go:457-500` | HTTP 状态码通过 `strings.Contains(msg, "status 50")` 等模式匹配。`download/writer.go:80` 生成的错误格式是 `"http status %d"`，而 `isRetryable` 同时匹配 `"http status 5"` 和 `"status 50"/"status 51"`。这导致：(1) `"status 50"` 匹配了 500 但也匹配 501-509；`"status 51"` 匹配 510-519；但漏掉了 540-599（虽然少见）。(2) 如果第三方 provider 包返回不同格式的错误（如 `"HTTP/1.1 503 Service Unavailable"`），完全不触发重试。(3) 更严重的是，`isRetryable` 先检查了非重试条件（404/403），但代码结构导致一个微妙 bug：如果 error message 同时包含 "status 404" 和 "status 503"（如重试日志拼接），5xx 检查在前会匹配成功返回 true，绕过了 404 检查。 | 引入结构化错误类型 `type HTTPError struct { StatusCode int }`，让 `downloadFile`（writer.go）和各 provider 返回此类型。`isRetryable` 用 `errors.As` 提取状态码做数值判断。 |
| M2 | `download/fallback.go` 和 `download/writer.go` 仍用 `log.Printf` / `fmt.Printf` | `download/fallback.go:86,98` + `download/writer.go:52` | 设计文档 S8.2 步骤 14 要求"全局替换 log.Printf -> slog"。fallback.go 用 `log.Printf`，writer.go 用 `fmt.Printf`（第 52 行）。这导致日志输出混合：slog JSON 格式 + log text 格式 + fmt 裸输出。Docker 环境下日志采集工具无法统一解析。 | `log.Printf("[download] fallback ...")` 替换为 `slog.Warn("download.fallback.search_error", "provider", name, "error", searchErr)`。`fmt.Printf("[download] lyrics ...")` 替换为 `slog.Warn("download.lyrics_save", "error", lrcErr)`。 |
| M3 | `handleProxyDownload` 的 `io.Copy` 错误被静默吞掉 | `internal/api/handler_nas.go:87` | `io.Copy(c.Writer, resp.Body)` 的返回值被完全忽略。如果流式传输中断（网络断开、客户端取消），错误不会被记录。考虑到代理下载可能传输数百 MB 音频文件，这个静默失败会使排障困难。 | 捕获 `io.Copy` 返回的 error，用 `slog.Warn` 记录。注意此时 HTTP header 已发送，无法再改 status code，但日志是必要的。 |
| M4 | CORS 中间件不支持多 origin | `internal/api/middleware.go:51-66` | 设计文档 S2.3 说"CORS_ORIGINS 环境变量"，当前实现直接把环境变量值原样设为 `Access-Control-Allow-Origin` header。如果用户设置 `CORS_ORIGINS=http://192.168.1.1,http://192.168.1.2`，浏览器会拒绝——`Access-Control-Allow-Origin` 不支持逗号分隔的多值（只接受单个 origin 或 `*`）。NAS 用户可能从多个设备访问。 | 按逗号分隔 `CORS_ORIGINS`，在请求时检查 `Origin` header 是否在允许列表中，动态返回匹配的单个 origin。如果列表为空或包含 `*`，返回 `*`。 |
| M5 | `SaveTask` 的 upsert 可能在首次创建时丢失 `CreatedAt` | `internal/store/task.go:39-68` | `SaveTask` 每次都设置 `UpdatedAt: time.Now()`，但 `CreatedAt` 来自 `t.CreatedAt`。GORM 的 `Save()` 在 SQLite 上执行 INSERT OR REPLACE。当 INSERT 首次执行时，如果 `t.CreatedAt` 是零值（比如 `LoadTasks` 恢复后再次 `SaveTask`），记录的 `CreatedAt` 会被覆盖为零值。虽然当前 `Enqueue` 中 `CreatedAt: time.Now()` 保证了首次非零，但 `LoadTasks` 恢复的 task 通过回调再次 `SaveTask` 时，`t.CreatedAt` 来自 DB 查询结果（非零），所以目前不会触发。然而这依赖于一个链条假设：DB 中的 CreatedAt 必须非零，而代码中没有任何校验。 | 在 `SaveTask` 中增加防御：`if r.CreatedAt.IsZero() { r.CreatedAt = now }`。 |

### Minor（建议修复，提升质量）

| # | 问题 | 位置 | 描述 |
|---|------|------|------|
| m1 | Dockerfile 使用 `golang:1.21-alpine` 但 go.mod 依赖可能需要更高版本 | `Dockerfile:2` | go.mod 声明 `go 1.21`，但 `gin v1.10.0` 和 `glebarez/sqlite v1.11.0` 的依赖树中可能有要求 Go 1.22+ 的包。本地开发用 Go 1.24.3。需要实际 Docker 构建验证（`/deploy` 阶段）。 |
| m2 | `ListAllTasks` 无排序保证 | `internal/store/task.go:74-75` | `db.Find(&records)` 不指定 ORDER BY。SQLite 默认返回 rowid 顺序，但不是合约。`LoadTasks` 恢复后 `m.order` 的顺序取决于 DB 返回顺序，前端任务列表顺序可能与重启前不同。 | 
| m3 | `withRetry` 的退避计算对大 `maxRetries` 值会产生过大等待 | `download/task.go:520` | `math.Pow(float64(backoffBase), float64(attempt-1))`：如果有人设 `DOWNLOAD_MAX_RETRIES=10, DOWNLOAD_RETRY_BACKOFF=3`，第 9 次退避 = 3^8 = 6561 秒 = 109 分钟。没有退避上限（cap）。设计文档虽然默认值合理（3 次重试 backoff=2 -> 1s, 2s），但环境变量可被任意设置。 |
| m4 | `login/manager.go` 在 `StopLogin` 中存在潜在死锁模式 | `login/manager.go:202-216` | `StopLogin` 先锁 `m.mu`（第 203 行），取出 session 后解锁，再锁 `session.mu`（第 213 行）。与 `handleMessage` 的锁序（`m.mu.RLock` -> `session.mu.Lock`）一致。但 `StartLogin` 中先锁 `m.mu`（第 66 行），然后锁 `existing.mu`（第 71 行）——这是嵌套锁。虽然 `StopLogin` 在解 `m.mu` 后再锁 `session.mu`，没有形成死锁环，但这种锁序不一致是脆弱的。这不是 M1 引入的问题（login/manager.go 未修改），但 M1 的 handler 层直接调用了它，应记录风险。 |

### Info（信息记录，无需修复）

| # | 内容 | 说明 |
|---|------|------|
| i1 | `store.Init` 设置 GORM logger 为 Silent | `internal/store/db.go:22-24` | 合理的生产配置。开发调试时可能需要通过环境变量控制 GORM 日志级别，但不影响 M1 功能。 |
| i2 | docker-compose.yml 缺少 `CONFIG_DIR` 和 `LOGIN_SCRIPT` 环境变量 | `docker-compose.yml` | main.go 中 `CONFIG_DIR` 默认 "config"、`LOGIN_SCRIPT` 默认 "scripts/login_helper.py"。Docker 容器内没有这些路径和 Python 环境，登录功能在 Docker 内不可用。但设计文档 S1.2 明确 M1 不覆盖新功能，登录是已有功能且需 Playwright，Docker 内不支持是预期行为。建议在文档或 docker-compose 注释中说明。 |

---

## 3. 设计一致性检查

### 3.1 API 接口

| 接口 | 设计 | 实现 | 一致 |
|------|------|------|------|
| GET /health | handler_search.go | handler_search.go:13 | OK |
| GET /providers | handler_search.go | handler_search.go:18 | OK |
| GET /api/search | handler_search.go | handler_search.go:48 | OK |
| POST /api/lyrics | handler_search.go | handler_search.go:72 | OK |
| GET /api/parse | handler_search.go | handler_search.go:96 | OK |
| GET /api/playlist/search | handler_playlist.go | handler_playlist.go:11 | OK |
| GET /api/playlist/songs | handler_playlist.go | handler_playlist.go:35 | OK |
| GET /api/playlist/parse | handler_playlist.go | handler_playlist.go:59 | OK |
| GET /api/playlist/recommended | handler_playlist.go | handler_playlist.go:86 | OK |
| POST /api/login/qr/start | handler_login.go | handler_login.go:10 | OK |
| GET /api/login/qr/poll | handler_login.go | handler_login.go:24 | OK |
| GET /api/login/status | handler_login.go | handler_login.go:40 | OK |
| POST /api/login/logout | handler_login.go | handler_login.go:61 | OK |
| POST /api/download/file | handler_nas.go | handler_nas.go:35 | OK |
| GET /api/nas/status | handler_nas.go | handler_nas.go:21 | OK |
| POST /api/nas/download | handler_nas.go | handler_nas.go:91 | OK |
| POST /api/nas/download/batch | handler_nas.go | handler_nas.go:127 | OK |
| GET /api/nas/tasks | handler_nas.go | handler_nas.go:188 | OK |
| GET /api/nas/task | handler_nas.go | handler_nas.go:197 | OK |
| GET /api/nas/batches | handler_nas.go | handler_nas.go:218 | **偏离** (见 C2) |
| GET / (static) | embed.go | embed.go:12 via NoRoute | OK |

21 条路由全部注册正确。`/api/nas/batches` 的实现逻辑偏离设计文档（从内存聚合而非 DB 聚合）。

### 3.2 数据模型

| 表/字段 | 设计 | 实现 | 一致 |
|---------|------|------|------|
| download_batches 表 | 6 字段 (id, source, name, total, created_at, updated_at) | store/batch.go BatchRecord 6 字段 | OK |
| download_tasks 表 | 19 字段 | store/task.go TaskRecord 19 字段 | OK |
| TaskRecord.Skipped 类型 | 设计 S5.1: `INTEGER DEFAULT 0` | 实现: `bool` (GORM 自动转换) | OK (GORM 处理 bool<->int) |
| idx_source_song 复合索引 | (source, song_id) | GORM tag `index:idx_source_song,composite:source/song` | OK |
| idx_status | status | GORM tag `index` on Status | OK |
| idx_batch_id | batch_id | GORM tag `index` on BatchID | OK |
| Task.RetryCount | `int json:"retry_count"` | download/task.go:40 | OK |
| BatchInfo 字段 | Name/Total/Done/Failed/Running/Pending | download/task.go:48-57 | OK |
| Task ID 格式 | `t-{16hex}` / `b-{16hex}` | newID() crypto/rand 8 bytes -> 16 hex | OK |
| store.Init 签名 | `func Init(dataDir string) (*gorm.DB, error)` | store/db.go:15 | OK |
| store.SaveTask 签名 | `func SaveTask(db *gorm.DB, t *download.Task) error` | store/task.go:39 | OK |
| store.ListAllTasks 签名 | `func ListAllTasks(db *gorm.DB) ([]*download.Task, error)` | store/task.go:73 | OK |
| store.MarkRunningAsFailed 签名 | `func MarkRunningAsFailed(db *gorm.DB) error` | store/task.go:116 | OK |
| store.CreateBatch 签名 | `func CreateBatch(db *gorm.DB, id, source, name string, total int) error` | store/batch.go:23 | OK |
| store.GetBatch 签名 | `func GetBatch(db *gorm.DB, id string) (*BatchRecord, error)` | store/batch.go:36 | OK |

### 3.3 错误码

| 场景 | 设计 HTTP Status | 实现 | 一致 |
|------|-----------------|------|------|
| source 缺失/无效 | 400 | writeError(c, 400, ...) 各 handler | OK |
| keyword/id/link 缺失 | 400 | writeError(c, 400, ...) | OK |
| JSON 解析失败 | 400 | writeError(c, 400, "invalid request body: ...") | OK |
| provider 不支持功能 | 501 | writeError(c, 501, ...) | OK |
| NAS 未配置 | 503 | writeError(c, 503, "NAS download not configured ...") | OK |
| 平台 API 失败 | 500 | writeError(c, 500, err.Error()) | OK |
| task_id 不存在 | 404 | writeError(c, 404, "task not found") | OK |
| Panic Recovery | 500 + slog.Error | middleware.go Recovery() | OK |
| 统一响应格式 | `{code:0,data}` / `{code:-1,message}` | writeOK / writeError | OK |

### 3.4 中间件配置

| 要求 | 设计 | 实现 | 一致 |
|------|------|------|------|
| gin.New() | 不用 gin.Default() | router.go:21 | OK |
| Recovery() | slog.Error + stack | middleware.go:15-33 | OK |
| SlogLogger() | method/path/status/latency_ms | middleware.go:36-47 | OK |
| CORS() | CORS_ORIGINS env | middleware.go:51-66 | OK (功能受限, 见 M4) |

### 3.5 wiring 顺序

| 设计步骤 | 实现 | 一致 |
|----------|------|------|
| 1. 读环境变量 | main.go:18-27 | OK |
| 2. store.Init(dataDir) | main.go:32-36 | OK |
| 3. MarkRunningAsFailed | main.go:39-41 | OK |
| 4. ListAllTasks | main.go:44-47 | OK |
| 5-6. Config + NewManager | main.go:93-105 | OK |
| 7. SetOnTaskUpdate | main.go:113-117 | OK |
| 8. LoadTasks | main.go:119 | OK |
| 9-10. NewRouter + Run | main.go:123-138 | OK |

### 3.6 日志规范

| 要求 | 设计 | 实现 | 一致 |
|------|------|------|------|
| 服务启动日志 | slog.Info("server starting", ...) | main.go:132 | OK |
| 请求日志 (middleware) | slog.Info("request", ...) | middleware.go:40-46 | OK |
| download.enqueue | slog.Info + task_id/title/artist/source | task.go:162-167 | OK |
| download.done | slog.Info + task_id/file/skipped | task.go:417-421 | OK |
| download.failed | slog.Error + task_id/song/error | task.go:432-436 | OK |
| download.retry | slog.Warn + task_id/attempt/max/error/wait_ms | task.go:338-344 | OK |
| download.fallback | slog.Warn + task_id/from/to/retry_exhausted | task.go:366-370 | OK |
| login 事件 | slog.Info/Warn | main.go:69,75 (callback) | OK (callback 中) |
| LOG_LEVEL 环境变量 | debug/info/warn/error | main.go:159-172 | OK |
| slog JSON handler | 是 | main.go:171 NewJSONHandler | OK |
| **全局 log.Printf 替换** | 步骤 14：全量替换 | fallback.go 仍用 log.Printf, writer.go 用 fmt.Printf | **偏离** (见 M2) |

---

## 4. 假设审计检查

### 4.1 发现的未审计假设

| # | 假设内容 | 位置 | 风险 | 建议 |
|---|----------|------|------|------|
| A1 | Provider 包的错误消息格式包含 "http status NNN" 或 "status NNN" | download/task.go:472-483 | 高 -- provider 格式不统一导致该重试的不重试 | 引入 HTTPError 结构化类型（见 M1） |
| A2 | `Task` 浅拷贝足以隔离并发访问 | download/task.go:135 `t := *task` | 中 -- Song.Extra 是 map，浅拷贝共享底层数据 | 深拷贝 Extra map（见 C1） |
| A3 | GORM Save() 在 SQLite 上等效 INSERT OR REPLACE 且是原子的 | store/task.go:67 | 低 -- SQLite 单写 + GORM WAL 模式 | 可接受，但应有注释说明 |
| A4 | crypto/rand.Read 永远成功 | download/task.go:447 | 低 -- 已有 fallback 到 time.UnixNano() | 可接受 |
| A5 | `handleNASBatchDownload` 中 `store.CreateBatch` 失败只 warn 不阻塞 | handler_nas.go:176-178 | 中 -- batch 入 DB 失败但 task 已开始下载，重启后 ListBatches 查不到这个 batch | 应返回错误或至少确保 retry |
| A6 | `proxyClient` 全局 10 分钟超时足以覆盖所有音频文件 | handler_nas.go:18 | 低 -- 对超大文件(>500MB FLAC) 可能不够 | 可接受但应在文档中注明上限 |
| A7 | Docker 容器内无 Python 环境不影响服务启动 | cmd/server/main.go:62 | 低 -- loginMgr 创建不依赖 Python 存在；StartLogin 才会 exec.Command | 可接受，但 Docker 内登录功能实际不可用 (见 i2) |

---

## 5. 与测试报告交叉验证

测试报告 `docs/test-reports/2026-04-25-M1-基建重构.md` 发现了以下问题，Code Review 独立验证结果：

| 测试报告编号 | 测试报告描述 | Code Review 独立验证 | 备注 |
|-------------|-------------|---------------------|------|
| C1 (测试) | M1 新增模块零测试覆盖 | **确认** -- Code Review 范围不包含测试覆盖率评审，但注意到无任何测试文件 | 测试报告已充分覆盖 |
| C2 (测试) | 重启后批量下载名称丢失 | **确认并升级** -- Code Review 发现该问题更严重：不仅是名称丢失，重启后 ListBatches 完全为空（本报告 C2）；且 synthetic entry 污染 GetTask（本报告 C3） | Code Review 发现了测试报告未指出的 C3 |
| M1 (测试) | isRetryable 字符串匹配脆弱 | **确认** -- Code Review 进一步发现匹配逻辑有覆盖范围漏洞和优先级 bug（本报告 M1） | Code Review 补充了具体 bug 场景 |
| M2 (测试) | log.Printf 未迁移 | **确认** -- Code Review 补充发现 writer.go 中还有 fmt.Printf 未迁移（本报告 M2） | Code Review 发现了额外遗漏 |

### Code Review 独立发现（测试报告未覆盖）

| 编号 | 问题 | 风险 |
|------|------|------|
| C1 | notifyUpdate 并发写入 DB 乱序 | 高 -- 生产环境下可能导致 task 状态回退 |
| C3 | GetTask 返回 synthetic batch entry | 中 -- 前端展示异常 |
| M3 | handleProxyDownload io.Copy 错误被吞 | 中 -- 排障困难 |
| M4 | CORS 不支持多 origin | 中 -- 多设备 NAS 场景受限 |
| M5 | SaveTask CreatedAt 零值风险 | 低 -- 当前路径不触发，但缺乏防御 |

---

## 6. 后续行动

| 优先级 | 行动 | 负责角色 | 状态 |
|--------|------|----------|------|
| P0 | 修复 C1: notifyUpdate 并发 DB 写入乱序 -- 引入写入队列保序 | dev-engineer | 待处理 |
| P0 | 修复 C2: handleListBatches 改为 DB 聚合查询 | dev-engineer | 待处理 |
| P0 | 修复 C3: 将 synthetic batch entry 从 tasks map 分离到独立存储 | dev-engineer | 待处理 |
| P1 | 修复 M1: isRetryable 引入 HTTPError 结构化类型 | dev-engineer | 待处理 |
| P1 | 修复 M2: fallback.go + writer.go 全量迁移到 slog | dev-engineer | 待处理 |
| P1 | 修复 M3: handleProxyDownload 记录 io.Copy 错误 | dev-engineer | 待处理 |
| P1 | 修复 M4: CORS 中间件支持多 origin | dev-engineer | 待处理 |
| P1 | 修复 M5: SaveTask 增加 CreatedAt 零值防御 | dev-engineer | 待处理 |
| P2 | 修复 m2: ListAllTasks 增加 ORDER BY created_at | dev-engineer | 待处理 |
| P2 | 修复 m3: withRetry 增加退避上限 (如 60s) | dev-engineer | 待处理 |
| P2 | 验证 m1: Docker 构建 (需 /deploy 阶段) | deploy | 待处理 |
