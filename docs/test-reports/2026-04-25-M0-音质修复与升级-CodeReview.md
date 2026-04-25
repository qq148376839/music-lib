# Code Review: M0 音质修复与升级

> 审查日期: 2026-04-25
> 设计文档: docs/2026-04-25-音质修复与升级.md
> 审查人: code-reviewer (独立审查)

---

## 总体结论: 建议修改

实现整体与设计文档一致性较高，核心流程（Provider 回写、Writer 安全替换、EnqueueUpgrade、前端音质显示）均已落地。发现 0 个 Critical 问题、4 个 Major 问题、5 个 Minor 问题、4 个 Info 级别备注。Major 问题集中在 Glob 路径注入风险、qualityScore 与设计文档的偏差、以及并发访问 candidates 的潜在竞态。建议修复 Major 问题后合并。

---

## 问题列表

### Major-1: filepath.Glob 模式中的特殊字符未转义 — 路径注入风险

**文件**: `/Users/rio/Documents/music-lib/download/writer.go` 第 111-112 行

**问题**: `baseName` 由 `SanitizeFilename(Artist + " - " + Name)` 生成后直接拼入 Glob 模式。`SanitizeFilename` 只过滤了 `\ / : * ? " < > |` 这些 Windows 非法字符，但 `filepath.Glob` 还把 `[` 和 `]` 当作字符类语法。

如果歌名或歌手名包含方括号（例如 `[Bonus Track]`、`J-Pop [Remix]`），`SanitizeFilename` 不会转义它们，Glob 会尝试解析为字符类，导致匹配失败甚至返回 error。此时代码走 `matches = nil` 分支——不会崩溃，但会导致已有文件检测失效，每次都当作新文件下载，绕过音质比较和安全替换逻辑。

**影响**: 包含方括号的歌曲将永远无法被识别为已存在，升级逻辑失效。中日韩歌曲名中方括号不少见。

**修复建议**: 在拼入 Glob 前对 baseName 中的 `[`、`]`、`\`、`*`、`?` 进行转义。Go 1.17+ 没有内置 `filepath.GlobEscape`，需手动替换：`[` -> `[[]`，`]` -> `[]]`，或者改用 `os.ReadDir` + `strings.HasPrefix` 手动匹配，彻底避免 Glob 语法问题。

---

### Major-2: qualityScore 与设计文档 5.4 节不一致 — m4a 高码率分支为设计外新增

**文件**: `/Users/rio/Documents/music-lib/download/writer.go` 第 59-60 行、第 81-85 行

**问题**: 设计文档 5.4 节的分数表:
- m4a: 90
- 其他: 50

实际代码:
```go
case "m4a", "aac":
    if bitrate >= 256 {
        return 250
    }
    return 90
```

1. **新增了 `aac` 格式**和 **m4a bitrate >= 256 返回 250** 的分支，设计文档未定义此逻辑。
2. m4a 256kbps 得分 250 高于 mp3 128kbps 的 128，但低于 mp3 320kbps 的 320——这个排序是否符合预期需要确认。当前三个 Provider 中，QQ 的 C400 回写 bitrate=96，网易不回写 m4a，酷狗可能返回任意值。目前没有 Provider 实际会产生 m4a 256kbps 的场景。

**影响**: 功能上目前不影响（没有 Provider 产出 m4a 256k），但代码与设计文档不一致，且未来可能导致意外的音质排序。

**修复建议**: 要么更新设计文档补充此分支，要么移除 `bitrate >= 256` 的分支使其与设计文档一致。

---

### Major-3: EnqueueUpgrade 在 RLock 下收集 candidates，后续无锁访问 candidate 字段

**文件**: `/Users/rio/Documents/music-lib/download/task.go` 第 576-597 行、第 599-647 行

**问题**: `EnqueueUpgrade` 方法先 `m.mu.RLock()` 收集 `candidates []*Task`，然后 `m.mu.RUnlock()`，之后遍历 candidates 读取 `t.Status`、`t.Source`、`t.Song` 等字段——此时没有持有任何锁。

```go
m.mu.RLock()
// ... collect candidates ...
m.mu.RUnlock()

for _, t := range candidates {
    if t.Status != StatusDone {  // 无锁读 t.Status
```

`candidates` 中的 `*Task` 指针指向的是 `m.tasks` map 中的同一个对象。如果此刻某个 task 正被 `runTask` goroutine 修改（例如状态从 done 变为其他值——虽然当前不存在此场景），会产生数据竞争。

当前代码中 done 状态的 task 不会再被修改（runTask 已完成），所以实际上不会触发 race。但这依赖一个隐含假设：`status=done` 的 task 永远不会再被写入。如果未来新增任何修改已完成 task 的逻辑，这里就会成为真实的竞态。

**影响**: 当前无实际竞态，但存在结构性风险。`go test -race` 不会报警（因为当前没有并发写），但架构上脆弱。

**修复建议**: 在 RLock 期间深拷贝 candidates 中需要的字段（至少 Song、Source、Status），而不是持有指针引用。或者将整个遍历逻辑放在 RLock 内部。

---

### Major-4: 升级 API 响应体 `errors` 字段序列化为 `null` 而非空数组

**文件**: `/Users/rio/Documents/music-lib/download/task.go` 第 59-64 行

**问题**: `UpgradeResult.Errors` 声明为 `[]UpgradeError`，初始值为 nil。当没有错误时，JSON 序列化输出 `"errors": null` 而非设计文档 4.2 节隐含的空数组 `[]`。

前端代码 `app.js` 中只读取 `data.queued`，不直接遍历 `errors`，所以不会 crash。但如果前端或其他调用方对 `errors` 做 `.length` 或 `.forEach`，`null` 会报错。

**影响**: 前端当前不受影响，但 API 响应格式与设计文档示例不一致。

**修复建议**: 初始化 `Errors` 为 `[]UpgradeError{}`（空 slice），或 JSON tag 加 `omitempty`。

---

### Minor-1: 网易云回写逻辑未处理 API 返回 m4a 的场景

**文件**: `/Users/rio/Documents/music-lib/netease/netease.go` 第 489-499 行

**问题**: 设计文档 6.1 节的网易映射表只有 flac/mp3 320/mp3 128 三种。代码中的 switch 逻辑也只处理这三种：

```go
switch {
case actualBr >= 900000:
    s.Ext = "flac"
case actualBr >= 320000:
    s.Ext = "mp3"
    s.Bitrate = 320
default:
    s.Ext = "mp3"
    s.Bitrate = 128
}
```

但网易 API 实际可能返回 m4a 格式（尤其是某些版权受限歌曲返回 192kbps AAC）。当 `actualBr` 为 192000 时，代码会将 Ext 回写为 "mp3" 且 Bitrate=128，这是错误的。

**影响**: 仅影响网易返回非常规码率的少数场景，概率低但信息不准确。

**修复建议**: 可以在 default 分支中使用 `actualBr / 1000` 作为 Bitrate 而不是硬编码 128，使回写更精确。

---

### Minor-2: qualityScoreToLabel 只用 ext 生成 previousQuality 标签，丢失码率信息

**文件**: `/Users/rio/Documents/music-lib/download/task.go` 第 546-562 行

**问题**: `qualityScoreToLabel("mp3")` 只返回 "MP3"，但被替换的文件可能是 320kbps MP3。`previous_quality` 字段显示 "MP3" 而不是 "320kbps MP3"，丢失了码率维度。前端显示 "已升级 MP3 -> FLAC" 比 "已升级 320kbps MP3 -> FLAC" 信息量少。

**影响**: 纯显示问题，不影响功能。

**修复建议**: 将 `WriteResult` 扩展为包含 `PreviousBitrate`，或者在 `qualityScoreToLabel` 中接受 bitrate 参数。需要权衡是否值得修改——已有文件的 bitrate 本身就是启发式的（`qualityScore` 传 bitrate=0），所以即使修改也不够精确。可推迟到 M1（SQLite 持久化后有精确值）。

---

### Minor-3: SanitizeFilename 将 `*` 和 `?` 替换为 `_`，但 Filename() 和 Glob 模式使用同一函数

**文件**: `/Users/rio/Documents/music-lib/utils/file.go` 第 6-19 行，`/Users/rio/Documents/music-lib/download/writer.go` 第 111 行

**问题**: `SanitizeFilename` 把 `*` 替换为 `_`。Filename() 用它生成实际文件名，Glob 模式也用它生成搜索 pattern。两者一致，所以功能上没问题。但如果歌名是 "Hello*World"，SanitizeFilename 产生 "Hello_World"，Glob 搜索 "Hello_World.*"，能匹配到实际文件 "Hello_World.mp3"。

不过有个边界情况：如果用户手动在 NAS 上重命名了文件（比如把下划线改回星号），Glob 就匹配不到了。这属于外部干预，不算 bug。

**影响**: 无实际影响，仅作记录。

---

### Minor-4: Upgrade 接口空 body 处理 — EOF 错误被静默忽略

**文件**: `/Users/rio/Documents/music-lib/internal/api/handler_nas.go` 第 259 行

**问题**: 
```go
if err := json.NewDecoder(c.Request.Body).Decode(&body); err != nil && err.Error() != "EOF" {
```

使用 `err.Error() != "EOF"` 字符串匹配来判断空 body，而不是 `err != io.EOF`。虽然标准库 `json.Decoder` 在空 body 时确实返回 `io.EOF`，但按 Go 惯例应使用 `errors.Is(err, io.EOF)` 进行比较。当前写法能工作但脆弱。

**影响**: 功能正常，但代码风格不佳。

**修复建议**: 改为 `err != io.EOF` 或 `!errors.Is(err, io.EOF)`。

---

### Minor-5: 前端降级检测逻辑对 "high" 模式的判断不够精确

**文件**: `/Users/rio/Documents/music-lib/web/js/app.js` 第 920-923 行

**问题**:
```javascript
const isDegraded =
  (req === 'lossless' && actual.toUpperCase().indexOf('FLAC') === -1) ||
  (req === 'high' && (actual.indexOf('128') !== -1 || actual.toUpperCase().indexOf('M4A') !== -1));
```

对 `high` 模式：如果 actual 是 "128kbps MP3" 则标记降级，但如果 actual 是 "96kbps M4A" 也标记降级（通过 M4A 检测）。这看起来合理。

但设计文档 6.4 节只定义了 lossless 降级的检测逻辑（`requested=lossless 且 actual 不含 FLAC`），没有定义 high 模式的降级检测。前端自行扩展了 high 模式的降级逻辑。这不是 bug，但与设计文档不一致。

另外，如果 `actual_quality` 恰好包含 "128" 子串但不代表 128kbps（理论上不会，但字符串匹配逻辑脆弱），会误判。

**影响**: 功能上合理，但设计文档未覆盖。

---

### Info-1: 酷狗回写逻辑与其他 Provider 风格不同

**文件**: `/Users/rio/Documents/music-lib/kugou/kugou.go` 第 393-400 行

酷狗的 `GetDownloadURL` 通过 `fetchSongInfo` 获取一个新的 `Song` 对象，然后从该对象复制 Ext 和 Bitrate 到传入的 `s`。这与 QQ/网易直接在 switch 中回写的风格不同。

```go
if info.Ext != "" {
    s.Ext = info.Ext
}
if info.Bitrate > 0 {
    s.Bitrate = info.Bitrate
}
```

注意：如果 `info.Bitrate == 0`（fetchSongInfo 中 `resp.BitRate / 1000` 可能为 0），则 `s.Bitrate` 不会被更新，保留搜索时的旧值。这与设计文档 6.1 节 "Bitrate 置 0" 的要求不同——设计文档说 "如果没有 bitrate，Bitrate 置 0"，但代码是 "如果没有 bitrate，不修改"。

不标记为 Major 是因为酷狗的 API 通常会返回 bitRate 字段，且搜索时已设置了 bitrate 估算值，保留旧值不会导致严重问题。

---

### Info-2: task map 无清理机制 — 长期运行存在内存增长风险

**文件**: `/Users/rio/Documents/music-lib/download/task.go`

`Manager.tasks` map 和 `Manager.order` slice 只增不删。每次 Enqueue（包括 EnqueueUpgrade）都会新增条目。如果 NAS 长期运行（数月/数年），任务量可能达到数万甚至更多。

当前设计文档 1.2 节明确说 "SQLite 持久化音质记录" 在 M1 范围。M1 完成后可以引入 task 清理机制。但当前 M0 阶段，如果用户频繁使用升级功能（每次升级都创建新 task），内存增长速度会加倍。

不标记为 Major 是因为 Task 结构体很小（< 1KB），10000 个 task 也只占几 MB，对 NAS 来说不算问题。但值得 M1 时处理。

---

### Info-3: 测试覆盖 — EnqueueUpgrade 无单元测试

**文件**: `/Users/rio/Documents/music-lib/download/task_test.go`

`task_test.go` 覆盖了 `newID`、`isRetryable`、`withRetry`、`Manager.LoadTasks`、`Manager.ListTasks`、`Manager.ListBatches`，但没有 `EnqueueUpgrade` 的单元测试。

设计文档 9.1 节明确要求: "EnqueueUpgrade() — 空 ID 列表 -> 升级所有 done 任务；无效 ID -> 记入 errors"。

建议补充以下测试:
1. taskIDs 为空，存在 done+!skipped 任务 -> queued > 0
2. taskIDs 包含无效 ID -> errors 列表有对应记录
3. taskIDs 包含 running 状态 task -> skipped + error
4. providers 中找不到对应 source -> skipped + error

---

### Info-4: favicon SVG 符合设计规格

**文件**: `/Users/rio/Documents/music-lib/web/favicon.svg`

viewBox 32x32，音符图案，深色背景 + `#58a6ff` 亮色，与页面主题一致。`index.html` 正确引用 `<link rel="icon" type="image/svg+xml" href="/favicon.svg">`。符合设计文档第 8 节要求。

---

## 设计文档一致性检查清单

| 检查项 | 设计文档章节 | 结论 | 备注 |
|--------|-------------|------|------|
| Song.QualityString() 方法 | 5.3 | 一致 | 实现与规格匹配 |
| QQ 音质回写映射表 | 6.1 | 一致 | F000/M800/M500/C400 四级映射正确 |
| 网易音质回写映射表 | 6.1 | 一致 | 优先使用 API 返回的 Br 值，符合设计建议 |
| 酷狗音质回写 | 6.1 | 基本一致 | 见 Info-1，bitrate=0 时行为偏差 |
| WriteResult 类型 | 5.2 | 一致 | 字段名、类型、Action 常量均匹配 |
| qualityScore 分数体系 | 5.4 | 偏差 | 见 Major-2，m4a 高码率分支为新增 |
| WriteSongToDisk 签名 | 6.2 | 一致 | 返回 (WriteResult, error) |
| Glob 搜索模式 | 6.2 | 一致 | `SanitizeFilename(Artist + " - " + Name) + ".*"` |
| 安全替换 .tmp + rename | 6.2 | 一致 | tmpPath = destPath + ".tmp"，失败时 Remove .tmp |
| Remove 旧文件失败只 warn | 7.1 | 一致 | `slog.Warn("download.remove_old_file_failed", ...)` |
| Task 4 个新字段 | 5.1 | 一致 | RequestedQuality/ActualQuality/Upgraded/PreviousQuality |
| UpgradeResult/UpgradeError 类型 | 6.3 | 一致 | 字段名和 JSON tag 匹配 |
| EnqueueUpgrade 空 taskIDs 逻辑 | 6.3 | 一致 | 遍历所有 done+!skipped |
| EnqueueUpgrade 深拷贝 Song | 6.3 | 一致 | Extra map 独立拷贝 |
| POST /api/nas/download/upgrade 路径 | 4.2 | 一致 | 路由注册在 router.go |
| 请求体 task_ids | 4.2 | 一致 | 空数组 = 全部升级 |
| 响应体 batch_id/queued/skipped/errors | 4.2 | 一致 | 字段名匹配（errors 可能为 null，见 Major-4） |
| 404 无可升级任务 | 4.2 | 一致 | queued==0 && skipped==0 -> 404 |
| 前端音质 tag 显示 | 6.4 | 一致 | actual_quality 存在时显示 |
| 前端降级标识 | 6.4 | 扩展 | 见 Minor-5，扩展了 high 模式检测 |
| 前端升级标识 | 6.4 | 一致 | upgraded=true 时显示 "已升级 prev -> actual" |
| 单首升级按钮 | 6.4 | 一致 | done+!skipped+!upgraded 时显示 |
| 全部升级按钮 | 6.4 | 一致 | upgrade-bar 中，统计可升级数，调用正确 API |
| Favicon | 8 | 一致 | SVG 32x32，音符图案 |
| 日志规范 slog | 7.2 | 一致 | download.quality / download.upgrade 日志格式匹配 |

---

## 安全和并发风险标注

| 风险项 | 级别 | 状态 |
|--------|------|------|
| Glob 路径注入（方括号） | Major | 需修复 |
| EnqueueUpgrade 无锁读 candidate 字段 | Major | 当前安全，结构性风险 |
| task map 无限增长 | Info | M1 处理 |
| downloadFile 未设置 User-Agent | Info | 依赖 longClient 默认值，部分 CDN 可能拒绝 |

---

## 总结

| 级别 | 数量 | 是否阻塞合并 |
|------|------|-------------|
| Critical | 0 | - |
| Major | 4 | 建议修复 Major-1（Glob 注入）和 Major-4（null vs []）后合并 |
| Minor | 5 | 非阻塞 |
| Info | 4 | 非阻塞 |

核心判断：M0 的功能实现完整度高，与设计文档的契合度在 90% 以上。Major-1（Glob 路径注入）是唯一可能在真实使用中导致功能异常的问题，建议优先修复。其余 Major 问题属于规范性和健壮性范畴，可以在合并后的 follow-up 中处理。
