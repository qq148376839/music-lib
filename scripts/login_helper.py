#!/usr/bin/env python3
"""
Playwright-based QR login helper for Netease and QQ Music.

Communication protocol (stdout JSON lines):
  {"type":"qr_ready","image":"<base64 PNG>"}
  {"type":"status","code":802}
  {"type":"login_success","cookies":"...","nickname":"..."}
  {"type":"expired"}
  {"type":"error","message":"..."}

Usage:
  python3 login_helper.py netease
  python3 login_helper.py qq
"""

import base64
import json
import sys
import time
import logging

logging.basicConfig(level=logging.INFO, stream=sys.stderr,
                    format="%(asctime)s [%(levelname)s] %(message)s")
log = logging.getLogger("login_helper")


def emit(obj: dict):
    """Write a JSON line to stdout and flush."""
    print(json.dumps(obj, ensure_ascii=False), flush=True)


def emit_error(msg: str):
    emit({"type": "error", "message": msg})


def screenshot_element(el) -> str:
    """Screenshot an element and return base64 PNG."""
    data = el.screenshot()
    return base64.b64encode(data).decode("ascii")


# ---------------------------------------------------------------------------
# Netease  (music.163.com)
#
# Page structure (from live capture):
#   - Login trigger: a[data-action="login"]  (header "登录" link)
#   - QR container:  #login-qrcode  (.lg.n-login-scan)
#   - QR canvas:     #login-qrcode .canvas.j-flag
#   - Scanned state: #login-qrcode .confirm.j-flag  becomes visible
#   - Expired:       page shows "二维码已过期" text; [data-action="refresh"] appears
#   - Success:       MUSIC_U cookie is set
# ---------------------------------------------------------------------------

def run_netease():
    from playwright.sync_api import sync_playwright

    with sync_playwright() as pw:
        browser = pw.chromium.launch(
            headless=True,
            args=[
                "--disable-blink-features=AutomationControlled",
                "--no-sandbox",
                "--disable-dev-shm-usage",
            ],
        )
        context = browser.new_context(
            user_agent=(
                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                "AppleWebKit/537.36 (KHTML, like Gecko) "
                "Chrome/120.0.0.0 Safari/537.36"
            ),
            viewport={"width": 1280, "height": 800},
            locale="zh-CN",
        )
        page = context.new_page()

        try:
            _netease_login_loop(page, context)
        except Exception as e:
            log.exception("netease login error")
            emit_error(str(e))
        finally:
            browser.close()


def _netease_login_loop(page, context):
    """Navigate to Netease, open the QR login dialog, capture QR, poll."""
    while True:
        page.goto("https://music.163.com/", wait_until="domcontentloaded")
        page.wait_for_timeout(3000)

        # --- Click the "登录" link in the header ---
        login_clicked = False
        for sel in [
            'a[data-action="login"]',       # official data-action attribute
            'a.link.s-fc3:has-text("登录")', # class + text combo
            'text=登录',                      # fallback: any "登录" text
        ]:
            try:
                loc = page.locator(sel).first
                if loc.is_visible(timeout=2000):
                    loc.click()
                    login_clicked = True
                    log.info("clicked login trigger: %s", sel)
                    break
            except Exception:
                continue

        if not login_clicked:
            log.warning("no login trigger found, trying direct hash navigation")
            page.goto("https://music.163.com/#/login", wait_until="domcontentloaded")

        page.wait_for_timeout(3000)

        # --- Capture the QR code ---
        qr_image = _netease_capture_qr(page)
        if not qr_image:
            emit_error("无法获取网易云二维码")
            return

        emit({"type": "qr_ready", "image": qr_image})

        # --- Poll for MUSIC_U cookie ---
        result = _netease_poll(page, context)
        if result == "success":
            return
        if result == "expired":
            emit({"type": "expired"})
            log.info("QR expired, will reload page and retry")
            page.wait_for_timeout(2000)
            continue
        # error — already emitted inside _netease_poll
        return


def _netease_capture_qr(page) -> str:
    """Screenshot the QR code element on the Netease login page."""

    # Priority 1: the QR canvas inside #login-qrcode
    for sel in [
        "#login-qrcode canvas",              # canvas rendered QR
        "#login-qrcode .canvas.j-flag",       # specific class
        "#login-qrcode img",                   # might be an <img> instead
    ]:
        try:
            loc = page.locator(sel).first
            if loc.is_visible(timeout=2000):
                return screenshot_element(loc)
        except Exception:
            continue

    # Priority 2: the whole QR container
    for sel in [
        "#login-qrcode",
        ".n-login-scan",
    ]:
        try:
            loc = page.locator(sel).first
            if loc.is_visible(timeout=2000):
                return screenshot_element(loc)
        except Exception:
            continue

    # Priority 3: any visible canvas on the page (QR is often drawn on canvas)
    try:
        canvases = page.locator("canvas")
        for i in range(canvases.count()):
            c = canvases.nth(i)
            if c.is_visible():
                return screenshot_element(c)
    except Exception:
        pass

    # Priority 4: broader login modal area
    for sel in [
        ".n-log2",
        ".m-layer",
        ".u-layer",
    ]:
        try:
            loc = page.locator(sel).first
            if loc.is_visible(timeout=1000):
                return screenshot_element(loc)
        except Exception:
            continue

    log.warning("no QR element found on Netease page")
    return ""


def _netease_poll(page, context) -> str:
    """Poll for MUSIC_U cookie. Returns 'success', 'expired', or 'error'."""
    max_wait = 300  # seconds
    start = time.time()

    while time.time() - start < max_wait:
        page.wait_for_timeout(2000)

        # Check cookies
        cookies = context.cookies()
        cookie_dict = {c["name"]: c["value"] for c in cookies}

        if "MUSIC_U" in cookie_dict:
            # Collect all .163.com / .music.163.com cookies
            parts = []
            for c in cookies:
                d = c.get("domain", "")
                if "163.com" in d:
                    parts.append(f"{c['name']}={c['value']}")
            cookie_str = "; ".join(parts)

            nickname = _netease_get_nickname(page, cookie_str)

            emit({
                "type": "login_success",
                "cookies": cookie_str,
                "nickname": nickname,
            })
            return "success"

        # Check "已扫码" (scanned) state
        try:
            confirm = page.locator("#login-qrcode .confirm.j-flag").first
            if confirm.is_visible(timeout=200):
                emit({"type": "status", "code": 802})
        except Exception:
            pass

        # Check if QR expired
        try:
            refresh = page.locator('[data-action="refresh"]').first
            if refresh.is_visible(timeout=200):
                return "expired"
        except Exception:
            pass

        # Also check for expiry text
        try:
            for text in ["二维码已过期", "二维码已失效"]:
                if page.locator(f"text={text}").count() > 0:
                    return "expired"
        except Exception:
            pass

    return "expired"


def _netease_get_nickname(page, cookie_str: str) -> str:
    """Fetch nickname via the account API using the obtained cookie."""
    try:
        resp = page.request.fetch(
            "https://music.163.com/api/nuser/account/get",
            headers={
                "Referer": "https://music.163.com/",
                "Cookie": cookie_str,
            },
        )
        data = resp.json()
        profile = data.get("profile") or {}
        return profile.get("nickname", "")
    except Exception as e:
        log.warning("failed to get netease nickname: %s", e)
    return ""


# ---------------------------------------------------------------------------
# QQ Music  (y.qq.com)
#
# Login flow (from live network capture):
#   1. y.qq.com has a "登录" button that opens a popup
#   2. The popup loads graph.qq.com/oauth2.0/authorize
#   3. Which loads xui.ptlogin2.qq.com/cgi-bin/xlogin in an iframe
#   4. The QR code image is fetched from xui.ptlogin2.qq.com/ssl/ptqrshow
#   5. After scan, cookies are set on .qq.com domain
#
# Key cookie: qqmusic_key / qm_keyst
# ---------------------------------------------------------------------------

def run_qq():
    from playwright.sync_api import sync_playwright

    with sync_playwright() as pw:
        browser = pw.chromium.launch(
            headless=True,
            args=[
                "--disable-blink-features=AutomationControlled",
                "--no-sandbox",
                "--disable-dev-shm-usage",
            ],
        )
        context = browser.new_context(
            user_agent=(
                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                "AppleWebKit/537.36 (KHTML, like Gecko) "
                "Chrome/120.0.0.0 Safari/537.36"
            ),
            viewport={"width": 1280, "height": 800},
            locale="zh-CN",
        )
        page = context.new_page()

        try:
            _qq_login_loop(page, context)
        except Exception as e:
            log.exception("qq login error")
            emit_error(str(e))
        finally:
            browser.close()


def _qq_login_loop(page, context):
    """Navigate to QQ Music, trigger login, capture QR from ptlogin iframe."""
    while True:
        page.goto("https://y.qq.com/", wait_until="domcontentloaded")
        page.wait_for_timeout(3000)

        # --- Click the login button ---
        login_clicked = False
        for sel in [
            'a.top_login__link',
            '.top_login__link',
            'text=登录',
            'a:has-text("登录")',
        ]:
            try:
                loc = page.locator(sel).first
                if loc.is_visible(timeout=2000):
                    loc.click()
                    login_clicked = True
                    log.info("clicked QQ login trigger: %s", sel)
                    break
            except Exception:
                continue

        if not login_clicked:
            log.warning("no QQ login trigger found on page")

        page.wait_for_timeout(4000)

        # --- Find the ptlogin2 iframe and capture QR ---
        qr_image = _qq_capture_qr(page)
        if not qr_image:
            emit_error("无法获取QQ音乐二维码")
            return

        emit({"type": "qr_ready", "image": qr_image})

        # --- Poll for qqmusic_key cookie ---
        result = _qq_poll(page, context)
        if result == "success":
            return
        if result == "expired":
            emit({"type": "expired"})
            log.info("QQ QR expired, will reload and retry")
            page.wait_for_timeout(2000)
            continue
        return


def _qq_find_ptlogin_frame(page):
    """Find the ptlogin2.qq.com iframe."""
    for f in page.frames:
        if "ptlogin2" in f.url or "xui.ptlogin2" in f.url:
            return f
    return None


def _qq_capture_qr(page) -> str:
    """Screenshot the QR code from the ptlogin iframe."""

    frame = _qq_find_ptlogin_frame(page)

    if frame:
        log.info("found ptlogin frame: %s", frame.url[:80])

        # The QR image has src containing "ptqrshow"
        for sel in [
            "img#qrlogin_img",                # known id in ptlogin
            "img[src*='ptqrshow']",            # URL-based selector
            "#qr",                              # possible QR container
            "#qrlogin_img",
        ]:
            try:
                loc = frame.locator(sel).first
                if loc.is_visible(timeout=3000):
                    return screenshot_element(loc)
            except Exception:
                continue

        # Screenshot the whole QR login area in the iframe
        for sel in [
            "#qrlogin",
            ".qrlogin",
            ".web_qr_login",
        ]:
            try:
                loc = frame.locator(sel).first
                if loc.is_visible(timeout=2000):
                    return screenshot_element(loc)
            except Exception:
                continue

    # No ptlogin frame found — maybe QQ uses a different flow now.
    # Try to find any QR-like image on the page or in any frame.
    for f in page.frames:
        try:
            imgs = f.locator("img[src*='qr']")
            for i in range(imgs.count()):
                img = imgs.nth(i)
                if img.is_visible():
                    return screenshot_element(img)
        except Exception:
            continue

    # Fallback: look for WeChat QR (open.weixin.qq.com)
    for f in page.frames:
        if "open.weixin.qq.com" in f.url:
            try:
                img = f.locator("img.web_qr_img, img[src*='qrcode']").first
                if img.is_visible(timeout=2000):
                    return screenshot_element(img)
            except Exception:
                pass

    log.warning("no QR element found for QQ login")
    return ""


def _qq_poll(page, context) -> str:
    """Poll for qqmusic_key cookie. Returns 'success', 'expired', or 'error'."""
    max_wait = 300
    start = time.time()

    while time.time() - start < max_wait:
        page.wait_for_timeout(2000)

        cookies = context.cookies()
        cookie_dict = {c["name"]: c["value"] for c in cookies}

        has_key = ("qqmusic_key" in cookie_dict or "qm_keyst" in cookie_dict)

        if has_key:
            # Collect all .qq.com domain cookies
            parts = []
            for c in cookies:
                d = c.get("domain", "")
                if "qq.com" in d:
                    parts.append(f"{c['name']}={c['value']}")
            cookie_str = "; ".join(parts)

            # Nickname from uin
            nickname = cookie_dict.get("uin", cookie_dict.get("login_type", "QQ用户"))
            if isinstance(nickname, str) and nickname.startswith("o"):
                nickname = nickname[1:]

            emit({
                "type": "login_success",
                "cookies": cookie_str,
                "nickname": nickname,
            })
            return "success"

        # Check for QR expiry inside ptlogin iframe
        frame = _qq_find_ptlogin_frame(page)
        if frame:
            try:
                # ptlogin shows "二维码已失效" or a refresh link when expired
                for text in ["二维码已失效", "二维码已过期"]:
                    if frame.locator(f"text={text}").count() > 0:
                        return "expired"
                # Also check for the refresh link that appears when expired
                refresh = frame.locator("#qrlogin_img_out, .ptlogin_qrcode_expired")
                if refresh.count() > 0 and refresh.first.is_visible():
                    return "expired"
            except Exception:
                pass

    return "expired"


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    if len(sys.argv) < 2:
        emit_error("usage: login_helper.py <netease|qq>")
        sys.exit(1)

    platform = sys.argv[1].lower()

    if platform == "netease":
        run_netease()
    elif platform == "qq":
        run_qq()
    else:
        emit_error(f"unknown platform: {platform}")
        sys.exit(1)


if __name__ == "__main__":
    main()
