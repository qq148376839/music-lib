# M6: 稳定性修复

> 日期：2026-04-26
> 性质：Bugfix 轮次，修复 M1-M5 交付后暴露的三个问题

---

## 核心问题

M1-M5 的功能链路能跑，但在"推荐歌单"和"数据持久化"两个点上体验碎裂：推荐歌单点了没反应、重启后一切归零、不支持的功能入口没屏蔽。

---

## Bug 清单

### Bug 1（P0）：QQ 推荐歌单点击后 "playlist not found (empty cdlist)"

**根因**：`GetRecommendedPlaylists()` 从 `musicu.fcg` API 拿到 `content_id`，但 `fetchPlaylistDetail()` 用 `fcg_ucc_getcdinfo_byids_cp.fcg` 查歌曲，要求 `disstid`。两套 ID 体系不通用。

**涉及文件**：
- `qq/qq.go:250-340`（GetRecommendedPlaylists — 用 content_id）
- `qq/qq.go:343-473`（fetchPlaylistDetail — 用 disstid）
- `internal/api/handler_playlist.go:85-102`（handlePlaylistRecommended）

**修复方案**：
推荐歌单详情需要用不同的 API 接口获取歌曲列表。两个方向：
1. 在 `GetRecommendedPlaylists` 返回时标记 ID 类型，`GetPlaylistSongs` 根据类型选择不同的查询接口
2. 找到 QQ 音乐推荐歌单的专用详情接口（可能是 `musicu.fcg` 的另一个 module）

优先尝试方案 2（同一 API 体系内解决），方案 1 作为 fallback。

**验收条件**：在 Web UI 上点击 QQ 推荐歌单 → 正常显示歌曲列表 → 可以下载。

---

### Bug 2（P0）：重启/重新部署后所有配置丢失

**根因分析**：
代码层面持久化逻辑正确（SQLite + cookie 文件 + 启动恢复），问题在部署层：

1. **`.gitignore` 未忽略 `data/` 整个目录** — 只忽略了 `data/downloads/` 和 `data/video_output/`，`music-lib.db` 和 cookie 文件有被 git 跟踪的风险。部署脚本的 `git reset --hard FETCH_HEAD` 会覆盖所有被跟踪文件。
2. **`docker-compose.yml` 的 `build: .` 被注释** — 部署脚本跑 `docker compose build --no-cache` 时无效，实际可能拉取旧 image。
3. **缺少启动验证日志** — 恢复了多少条监控规则、多少条任务记录、cookie 是否加载成功，当前只有部分日志。

**修复方案**：
1. `.gitignore` 加入 `data/`（整个目录），防止数据文件被 git 跟踪
2. `docker-compose.yml` 默认启用 `build: .`（NAS 场景本地构建）
3. 启动日志增加数据恢复摘要：monitor 数量、task 数量、cookie 状态
4. 在 NAS 上实际验证：部署 → 配置监控+下载 → 重新部署 → 检查数据是否保留

**验收条件**：在 NAS 上执行标准部署流程（git fetch + reset + build + up），重启前后监控规则、下载历史、登录状态全部保留。

---

### Bug 3（P1）：不支持推荐歌单的平台前端显示报错

**根因**：Soda/Migu/Fivesing 等平台没有推荐歌单 API，后端返回 501，但前端仍然显示推荐入口。

**涉及文件**：
- `cmd/server/providers.go:83-91`（Soda 无 GetRecommended）
- `internal/api/handler_playlist.go:85-102`（501 响应）
- 前端推荐歌单组件

**修复方案**：
1. 后端新增 `/api/capabilities` 接口，返回每个平台支持的功能列表（推荐歌单、排行榜等）
2. 前端根据 capabilities 动态隐藏不支持的入口

**验收条件**：切换到 Soda 平台时，推荐歌单入口不显示。切换到 QQ 时正常显示。

---

## 执行顺序

```
Bug 2（持久化）→ Bug 1（推荐歌单）→ Bug 3（前端能力适配）
```

理由：Bug 2 不解决，修好 Bug 1 也会在下次部署后丢失测试数据。Bug 3 最低优先级，不阻塞核心功能。

---

## 反向审计

- **只做前 20% 能不能解决问题？** Bug 2 的 `.gitignore` 修复和 NAS 验证就能覆盖 80% 的痛苦。可以。
- **三个 bug 能合并吗？** 不能，根因完全不同：API 兼容性、部署配置、前端 UX。
- **验收条件够具体吗？** 每条都是在 NAS 上可当场验证的操作，不是描述性文字。
