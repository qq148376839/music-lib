# Milestone 5 — Go 原生 QR 登录（去 Python 化）

> 状态：待开始
> 前置条件：M4 完成（前端已有登录 UI，只需对接新后端）
> 触发原因：Docker 部署后登录报错 `exec: "python3": executable file not found in $PATH`

---

## 这个 Milestone 解决什么问题

当前 QR 登录依赖 Python3 + Playwright（浏览器自动化），Docker 最小容器里没有这些东西。
这不是"漏装了依赖"的问题——是架构方向错了。

**审计当前方案：**

| 维度 | Python + Playwright | Go 原生 HTTP |
|------|---------------------|-------------|
| 依赖体积 | ~500MB（Python + Chromium） | 0（纯标准库 + 1 个 QR 库） |
| 稳定性 | 脆弱（DOM 选择器随平台改版而失效） | 稳定（HTTP API 变更频率远低于前端） |
| 启动速度 | 慢（浏览器启动 3-5 秒） | 即时（HTTP 请求 <100ms） |
| Docker 兼容 | 需要 Chromium 运行时 | 零外部依赖 |

**结论：** Playwright 是逆向工程阶段的开发工具，不是生产架构。网易云和 QQ 音乐都有原生 HTTP API 支持 QR 登录，所有参考项目（go-music-dl 等）都用这种方式。

**完成后：** Docker 容器内 QR 登录正常工作，镜像体积不变（~20MB），前端无需任何修改。

---

## 交付范围

### P0 功能

**1. 网易云 QR 登录（Go 原生）**

协议流程（WeAPI）：
```
1. POST /weapi/login/qrcode/unikey
   → 返回 unikey

2. 拼接 QR URL: https://music.163.com/login?codekey={unikey}
   → Go 生成 QR 码 PNG（skip2/go-qrcode）
   → base64 编码发给前端

3. POST /weapi/login/qrcode/client/login  (轮询，2s 间隔)
   key={unikey}
   → code 800: 等待扫码
   → code 801: 已扫码等待确认
   → code 802: 二维码过期
   → code 803: 登录成功，响应包含 Set-Cookie
```

实现位置：`netease/qr_login.go`（新文件）

**2. QQ 音乐 QR 登录（Go 原生）**

协议流程（ptlogin2）：
```
1. GET https://ssl.ptlogin2.qq.com/ptqrshow?appid=716027609&...
   → 直接返回 QR 码 PNG 图片
   → 同时返回 qrsig cookie

2. 计算 ptqrtoken = hash(qrsig)

3. GET https://ssl.ptlogin2.qq.com/ptqrlogin?ptqrtoken={token}&...  (轮询)
   → 解析 ptuiCB 回调：
     '66' = 未失效
     '67' = 验证中
     '0'  = 成功，返回重定向 URL

4. 跟随重定向获取 QQ 认证 cookie

5. POST https://u.y.qq.com/cgi-bin/musicu.fcg
   → 交换 QQ auth cookie 为 QQ 音乐 cookie（qqmusic_key / qm_keyst）
```

实现位置：`qq/qr_login.go`（新文件）

**3. 重写 login/manager.go**

- 删除 `exec.Command` 子进程逻辑
- 改为直接调用 `netease.StartQRLogin()` / `qq.StartQRLogin()` 的 Go 方法
- 保持相同的状态机（Starting → WaitingScan → Scanned → Success/Expired/Error）
- 保持相同的 `GetStatus()` 接口签名（前端零改动）
- 保持相同的 cookie 持久化回调

### 删除项

| 文件 | 处置 | 原因 |
|------|------|------|
| `scripts/login_helper.py` | 删除 | 被 Go 原生实现替代 |
| `scripts/capture_login.py` | 保留 | 独立的逆向分析工具，不影响生产 |
| `login/manager.go` 中 `pythonPath`、`scriptPath` 字段 | 删除 | 不再需要 |
| `cmd/server/main.go` 中 `PYTHON_PATH`、`LOGIN_SCRIPT` 环境变量 | 删除 | 不再需要 |

---

## API 接口（无变更）

前端已有的 4 个登录接口保持不变：

| 端点 | 方法 | 行为变更 |
|------|------|---------|
| `/api/login/qr/start?platform=` | POST | 内部从 spawn Python → 调用 Go 函数 |
| `/api/login/qr/poll?platform=` | GET | 无变更 |
| `/api/login/status?platform=` | GET | 无变更 |
| `/api/login/logout?platform=` | POST | 无变更 |

**关键：前端零改动。** 状态机 JSON 响应格式完全一致。

---

## 新增依赖

| 库 | 用途 | 体积影响 |
|----|------|---------|
| `github.com/skip2/go-qrcode` | 网易云：从 URL 生成 QR 码 PNG | ~50KB 编译后 |

QQ 音乐不需要 QR 库——`ptqrshow` API 直接返回 QR 码图片。

---

## 数据模型

### QRSession（内部状态，不入库）

```go
type QRSession struct {
    Platform  string       // "netease" | "qq"
    State     SessionState // 复用现有枚举
    QRImage   string       // base64 PNG
    UniKey    string       // 网易云: unikey; QQ: qrsig
    Token     string       // QQ: ptqrtoken
    Nickname  string
    Cookies   string
    Error     string
    Cancel    context.CancelFunc
}
```

---

## 文件变更清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `netease/qr_login.go` | 新建 | QR 生成 + 轮询 + cookie 提取 |
| `qq/qr_login.go` | 新建 | QR 获取 + 轮询 + cookie 交换 |
| `login/manager.go` | 重写 | 删除子进程，改用 Go 调用 |
| `cmd/server/main.go` | 修改 | 删除 PYTHON_PATH / LOGIN_SCRIPT |
| `scripts/login_helper.py` | 删除 | 被替代 |
| `go.mod` | 修改 | 添加 go-qrcode 依赖 |

---

## 验收标准

1. **Docker 容器内 QR 登录可用**：`docker compose up` → 浏览器访问设置页 → 点击网易云登录 → 看到 QR 码 → 手机扫码 → 登录成功 → 刷新页面仍显示已登录
2. **QQ 音乐同上**：扫码 → 登录成功 → cookie 持久化
3. **无 Python 依赖**：Dockerfile 和容器内无 Python 相关内容
4. **前端零改动**：`frontend/` 目录无任何文件变更
5. **Cookie 持久化不变**：容器重启后仍保持登录状态（cookie 文件在 /data/config/）

---

## 测试策略

| 场景 | 验证方式 |
|------|---------|
| 网易云 QR 生成 | 单测：mock HTTP → 验证返回 base64 PNG |
| 网易云轮询状态机 | 单测：mock 800/801/802/803 响应 → 验证状态流转 |
| QQ ptqrtoken 计算 | 单测：已知 qrsig → 验证 hash 结果 |
| QQ cookie 交换 | 单测：mock musicu.fcg 响应 → 验证提取 qqmusic_key |
| Manager 生命周期 | 单测：StartLogin → GetStatus → 超时清理 |
| 集成测试 | 手动：Docker 容器内完整扫码流程 |

---

## 假设审计

| 假设 | 验证方式 | 风险 |
|------|---------|------|
| 网易云 WeAPI QR 接口仍可用 | WebSearch 确认 + 实测 | 低：稳定多年 |
| QQ ptlogin2 接口仍可用 | WebSearch 确认 + 实测 | 低：QQ 核心登录体系 |
| go-qrcode 库兼容 Go 1.22 | go get 验证 | 极低：纯 Go 实现 |
| musicu.fcg cookie 交换流程未变 | 参考 go-music-dl 实现 + 实测 | 中：腾讯偶尔调整参数 |

---

## 风险与降级

**如果网易云 WeAPI 加密参数变化：**
- 现有 `netease/crypto.go` 已实现 WeAPI 加密（`EncryptWeApi`），QR 登录接口使用相同加密

**如果 QQ 登录 cookie 交换流程变更：**
- 降级方案：前端增加"手动粘贴 Cookie"输入框（5 行代码 + 1 个 API）
- 这个降级方案可以作为 P1 在本 milestone 内一并实现
