#!/usr/bin/env python3
"""
QQ音乐 / 网易云 登录抓包分析脚本

用法:
    python3 scripts/capture_login.py qq       # 打开 QQ 音乐登录页
    python3 scripts/capture_login.py netease  # 打开网易云登录页

脚本会:
1. 启动一个真实的 Chromium 浏览器 (非无头)
2. 拦截并记录所有 network 请求/响应, 重点关注:
   - Set-Cookie 响应头
   - 包含 login/qr/auth/token 等关键词的 URL
   - POST 请求的 body
3. 你在浏览器中手动完成登录操作
4. 登录完成后回到终端按 Enter, 脚本会:
   - 导出浏览器中所有 Cookie
   - 保存完整的请求日志到 scripts/capture_{target}_log.json
   - 保存最终 Cookie 到 scripts/capture_{target}_cookies.json
"""

import json
import sys
import time
import re
from datetime import datetime
from pathlib import Path
from playwright.sync_api import sync_playwright

# ── 配置 ──────────────────────────────────────────────────

TARGETS = {
    "qq": {
        "name": "QQ音乐",
        "url": "https://y.qq.com/",
        "login_url": "https://y.qq.com/",
        "cookie_domains": [".qq.com", ".y.qq.com", "y.qq.com", ".music.qq.com"],
    },
    "netease": {
        "name": "网易云音乐",
        "url": "https://music.163.com/",
        "login_url": "https://music.163.com/",
        "cookie_domains": [".163.com", ".music.163.com", "music.163.com"],
    },
}

# 需要重点关注的 URL 关键词
INTERESTING_KEYWORDS = [
    "login", "qr", "auth", "token", "cookie", "session",
    "passport", "ssl", "check", "poll", "scan", "verify",
    "unikey", "codekey", "ptqrshow", "ptqrlogin", "xlogin",
    "account", "user", "nuser", "oauth", "authorize",
]

# ── 请求记录 ─────────────────────────────────────────────

captured_requests = []
set_cookie_log = []


def is_interesting_url(url: str) -> bool:
    url_lower = url.lower()
    return any(kw in url_lower for kw in INTERESTING_KEYWORDS)


def on_request(request):
    entry = {
        "timestamp": datetime.now().isoformat(),
        "method": request.method,
        "url": request.url,
        "headers": dict(request.headers),
        "post_data": None,
        "interesting": is_interesting_url(request.url),
    }

    if request.method == "POST" and request.post_data:
        entry["post_data"] = request.post_data[:2000]

    captured_requests.append(entry)

    # 终端实时输出关键请求
    if entry["interesting"]:
        print(f"\n{'='*80}")
        print(f"[{entry['method']}] {entry['url']}")
        if entry["post_data"]:
            print(f"  Body: {entry['post_data'][:200]}")


def on_response(response):
    url = response.url
    status = response.status
    headers = response.headers

    # 捕获 Set-Cookie
    # Playwright 的 response.headers 是合并后的, 用 headers_array 获取完整列表
    try:
        all_headers = response.all_headers()
    except Exception:
        all_headers = headers

    set_cookies = []
    try:
        header_list = response.headers_array()
        for h in header_list:
            if h["name"].lower() == "set-cookie":
                set_cookies.append(h["value"])
    except Exception:
        # fallback
        sc = all_headers.get("set-cookie", "")
        if sc:
            set_cookies = [sc]

    if set_cookies:
        entry = {
            "timestamp": datetime.now().isoformat(),
            "url": url,
            "status": status,
            "set_cookies": set_cookies,
        }
        set_cookie_log.append(entry)

        print(f"\n{'*'*80}")
        print(f"[Set-Cookie] {url}")
        for sc in set_cookies:
            # 高亮关键 cookie
            name = sc.split("=")[0].strip()
            is_key = any(k in name.lower() for k in [
                "music_u", "uin", "skey", "p_skey", "pt4_token",
                "login_type", "qm_keyst", "wxunionid", "qqmusic_key",
                "psrf_qqaccess_token", "psrf_qqopenid",
            ])
            marker = " ★★★" if is_key else ""
            print(f"  {sc[:150]}{marker}")

    # 记录关键 URL 的响应
    if is_interesting_url(url):
        body_text = ""
        try:
            ct = all_headers.get("content-type", "")
            if "json" in ct or "text" in ct or "javascript" in ct:
                body_text = response.text()[:3000]
        except Exception:
            pass

        for req_entry in reversed(captured_requests):
            if req_entry["url"] == url:
                req_entry["response_status"] = status
                req_entry["response_body"] = body_text
                if set_cookies:
                    req_entry["set_cookies"] = set_cookies
                break

        if body_text:
            print(f"  Status: {status}")
            print(f"  Body: {body_text[:300]}")


# ── 主流程 ───────────────────────────────────────────────

def main():
    if len(sys.argv) < 2 or sys.argv[1] not in TARGETS:
        print(f"用法: python3 {sys.argv[0]} [{'|'.join(TARGETS.keys())}]")
        sys.exit(1)

    target_key = sys.argv[1]
    target = TARGETS[target_key]
    out_dir = Path(__file__).parent

    print(f"{'='*80}")
    print(f"  登录抓包分析: {target['name']}")
    print(f"  目标 URL: {target['url']}")
    print(f"{'='*80}")
    print()
    print("即将打开浏览器，请在浏览器中手动完成登录操作。")
    print("脚本会实时监控所有 network 请求。")
    print("登录完成后，回到终端按 Enter 键保存结果。")
    print()

    with sync_playwright() as p:
        browser = p.chromium.launch(
            headless=False,
            args=[
                "--disable-blink-features=AutomationControlled",
                "--window-size=1280,900",
            ],
        )

        context = browser.new_context(
            viewport={"width": 1280, "height": 900},
            user_agent=(
                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                "AppleWebKit/537.36 (KHTML, like Gecko) "
                "Chrome/120.0.0.0 Safari/537.36"
            ),
            locale="zh-CN",
        )

        page = context.new_page()

        # 绑定事件
        page.on("request", on_request)
        page.on("response", on_response)

        # 打开目标页面
        print(f"正在打开 {target['url']} ...")
        page.goto(target["url"], wait_until="domcontentloaded")

        print()
        print(f"{'='*80}")
        print("  浏览器已打开，请在浏览器中完成登录操作。")
        print("  脚本正在实时监控 network 请求...")
        print(f"  完成后按 Enter 保存结果 (或输入 q 退出不保存)")
        print(f"{'='*80}")
        print()

        try:
            user_input = input(">>> 按 Enter 保存结果...")
        except (EOFError, KeyboardInterrupt):
            user_input = "q"

        if user_input.strip().lower() == "q":
            print("\n放弃保存。")
            browser.close()
            return

        # ── 导出 Cookie ──────────────────────────────────
        print("\n正在导出 Cookie...")
        cookies = context.cookies()

        # 按域名分组
        cookie_by_domain = {}
        for c in cookies:
            domain = c.get("domain", "unknown")
            if domain not in cookie_by_domain:
                cookie_by_domain[domain] = []
            cookie_by_domain[domain].append({
                "name": c["name"],
                "value": c["value"],
                "domain": domain,
                "path": c.get("path", "/"),
                "httpOnly": c.get("httpOnly", False),
                "secure": c.get("secure", False),
                "expires": c.get("expires", -1),
            })

        # 筛选目标域名的 cookie 并拼成 cookie string
        relevant_cookies = []
        for c in cookies:
            domain = c.get("domain", "")
            if any(domain.endswith(d.lstrip(".")) for d in target["cookie_domains"]):
                relevant_cookies.append(c)

        cookie_string = "; ".join(
            f"{c['name']}={c['value']}" for c in relevant_cookies
        )

        # ── 保存结果 ─────────────────────────────────────
        # 1. 完整请求日志
        log_file = out_dir / f"capture_{target_key}_log.json"
        log_data = {
            "target": target_key,
            "timestamp": datetime.now().isoformat(),
            "total_requests": len(captured_requests),
            "interesting_requests": [
                r for r in captured_requests if r.get("interesting")
            ],
            "set_cookie_events": set_cookie_log,
            "all_requests": captured_requests,
        }
        log_file.write_text(json.dumps(log_data, ensure_ascii=False, indent=2))
        print(f"请求日志已保存: {log_file}")

        # 2. Cookie
        cookie_file = out_dir / f"capture_{target_key}_cookies.json"
        cookie_data = {
            "target": target_key,
            "timestamp": datetime.now().isoformat(),
            "cookie_string": cookie_string,
            "cookies_by_domain": cookie_by_domain,
            "all_cookies": cookies,
        }
        cookie_file.write_text(json.dumps(cookie_data, ensure_ascii=False, indent=2))
        print(f"Cookie 已保存: {cookie_file}")

        # ── 终端摘要 ─────────────────────────────────────
        print(f"\n{'='*80}")
        print(f"  分析摘要")
        print(f"{'='*80}")
        print(f"  总请求数: {len(captured_requests)}")
        print(f"  关键请求数: {len(log_data['interesting_requests'])}")
        print(f"  Set-Cookie 事件: {len(set_cookie_log)}")
        print(f"  导出 Cookie 数: {len(cookies)} (目标域名: {len(relevant_cookies)})")
        print()

        if relevant_cookies:
            print("  目标域名关键 Cookie:")
            for c in relevant_cookies:
                val_preview = c["value"][:40] + "..." if len(c["value"]) > 40 else c["value"]
                print(f"    {c['domain']:30s} {c['name']:30s} = {val_preview}")

        print()
        print(f"  Cookie String (可直接用于 Go 代码):")
        print(f"  {cookie_string[:200]}{'...' if len(cookie_string) > 200 else ''}")
        print()

        # 列出关键的登录相关请求流程
        interesting = log_data["interesting_requests"]
        if interesting:
            print(f"  登录相关请求流程 ({len(interesting)} 个):")
            for i, r in enumerate(interesting, 1):
                status = r.get("response_status", "?")
                sc_count = len(r.get("set_cookies", []))
                sc_marker = f" [+{sc_count} cookies]" if sc_count else ""
                print(f"    {i:3d}. [{r['method']:4s}] {status} {r['url'][:100]}{sc_marker}")
            print()

        browser.close()

    print("完成。请查看日志文件进行详细分析。")


if __name__ == "__main__":
    main()
