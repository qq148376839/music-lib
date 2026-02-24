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


# ---------------------------------------------------------------------------
# Netease
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
                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
                "AppleWebKit/537.36 (KHTML, like Gecko) "
                "Chrome/120.0.0.0 Safari/537.36"
            ),
            viewport={"width": 1280, "height": 800},
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
    """Run the Netease QR login flow. Handles expiry by looping."""
    while True:
        page.goto("https://music.163.com/", wait_until="domcontentloaded")
        time.sleep(2)

        # Click the login trigger — try multiple selectors
        login_selectors = [
            "a.link.s-fc3",           # "登录" link in header
            'a[href="javascript:void(0)"]',
            "text=登录",
        ]
        clicked = False
        for sel in login_selectors:
            try:
                el = page.query_selector(sel)
                if el and el.is_visible():
                    el.click()
                    clicked = True
                    break
            except Exception:
                continue

        if not clicked:
            # Try navigating directly to login page
            page.goto("https://music.163.com/#/login", wait_until="domcontentloaded")
            time.sleep(1)

        # Wait for the QR code area to appear
        time.sleep(3)

        # Try to find and screenshot the QR code
        qr_image = _netease_capture_qr(page)
        if not qr_image:
            emit_error("无法获取网易云二维码")
            return

        emit({"type": "qr_ready", "image": qr_image})

        # Poll for login success
        result = _netease_poll(page, context)
        if result == "success":
            return
        elif result == "expired":
            emit({"type": "expired"})
            log.info("QR expired, refreshing...")
            time.sleep(2)
            continue
        else:
            return


def _netease_capture_qr(page) -> str:
    """Try to capture the QR code image from the Netease login page."""
    # Look for QR code image or canvas in various possible locations
    qr_selectors = [
        ".qr-code-area img",
        ".qr-code img",
        "img[src*='qrcode']",
        "img[src*='login']",
        "canvas",
        ".j-img",
        "#qrImg",
        "img.qr",
    ]

    for sel in qr_selectors:
        try:
            el = page.query_selector(sel)
            if el and el.is_visible():
                screenshot = el.screenshot()
                return base64.b64encode(screenshot).decode("ascii")
        except Exception:
            continue

    # Fallback: screenshot a centered region of the page
    try:
        # Try taking a screenshot of the login modal area
        modal_selectors = [
            ".m-layer",
            ".u-layer",
            ".login-box",
            ".qr-login",
            "[class*='login']",
        ]
        for sel in modal_selectors:
            try:
                el = page.query_selector(sel)
                if el and el.is_visible():
                    screenshot = el.screenshot()
                    return base64.b64encode(screenshot).decode("ascii")
            except Exception:
                continue
    except Exception:
        pass

    return ""


def _netease_poll(page, context) -> str:
    """Poll cookies for MUSIC_U. Returns 'success', 'expired', or 'error'."""
    max_wait = 300  # 5 minutes
    start = time.time()

    while time.time() - start < max_wait:
        time.sleep(2)

        cookies = context.cookies()
        cookie_dict = {c["name"]: c["value"] for c in cookies}

        if "MUSIC_U" in cookie_dict:
            # Build cookie string from relevant cookies
            relevant = ["MUSIC_U", "__csrf", "NMTID", "JSESSIONID-WYYY", "WNMCID"]
            parts = []
            for c in cookies:
                if c["name"] in relevant or c["name"].startswith("_"):
                    parts.append(f"{c['name']}={c['value']}")
            cookie_str = "; ".join(parts)

            # Try to get nickname
            nickname = _netease_get_nickname(page)

            emit({
                "type": "login_success",
                "cookies": cookie_str,
                "nickname": nickname,
            })
            return "success"

        # Check if QR has expired by looking for expiry indicators
        try:
            expired_els = page.query_selector_all(
                "text=二维码已过期, text=已失效, text=expired"
            )
            if expired_els:
                return "expired"
        except Exception:
            pass

    return "expired"


def _netease_get_nickname(page) -> str:
    """Try to extract the logged-in username."""
    try:
        # After login, the page might show the nickname
        page.goto("https://music.163.com/", wait_until="domcontentloaded")
        time.sleep(2)
        # Look for nickname element
        nick_selectors = [
            ".head_name .f-thide",
            ".nickname",
            "span.name",
        ]
        for sel in nick_selectors:
            try:
                el = page.query_selector(sel)
                if el:
                    text = el.inner_text().strip()
                    if text:
                        return text
            except Exception:
                continue
    except Exception:
        pass
    return ""


# ---------------------------------------------------------------------------
# QQ Music
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
                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
                "AppleWebKit/537.36 (KHTML, like Gecko) "
                "Chrome/120.0.0.0 Safari/537.36"
            ),
            viewport={"width": 1280, "height": 800},
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
    """Run the QQ Music QR login flow."""
    while True:
        page.goto("https://y.qq.com/", wait_until="domcontentloaded")
        time.sleep(2)

        # Click login button
        login_selectors = [
            "a.top_login__link",
            "text=登录",
            ".login_link",
            "a[href*='login']",
        ]
        for sel in login_selectors:
            try:
                el = page.query_selector(sel)
                if el and el.is_visible():
                    el.click()
                    break
            except Exception:
                continue

        time.sleep(3)

        # The QQ login page uses an iframe from ptlogin2.qq.com
        qr_image = _qq_capture_qr(page)
        if not qr_image:
            emit_error("无法获取QQ音乐二维码")
            return

        emit({"type": "qr_ready", "image": qr_image})

        result = _qq_poll(page, context)
        if result == "success":
            return
        elif result == "expired":
            emit({"type": "expired"})
            log.info("QR expired, refreshing...")
            time.sleep(2)
            continue
        else:
            return


def _qq_capture_qr(page) -> str:
    """Try to capture QR code from QQ login iframe."""
    # Try to find the ptlogin iframe
    frame = None
    for f in page.frames:
        if "ptlogin" in f.url or "xui.ptlogin2" in f.url:
            frame = f
            break

    if frame:
        # Look for QR image inside the iframe
        qr_selectors = ["#qrlogin_img", "img[src*='ptqrshow']", "img.qr"]
        for sel in qr_selectors:
            try:
                el = frame.query_selector(sel)
                if el and el.is_visible():
                    screenshot = el.screenshot()
                    return base64.b64encode(screenshot).decode("ascii")
            except Exception:
                continue

    # Fallback: try on the main page
    qr_selectors = [
        "img[src*='qr']",
        "img[src*='ptqrshow']",
        "#login_qr_img",
        "canvas",
    ]
    for sel in qr_selectors:
        try:
            el = page.query_selector(sel)
            if el and el.is_visible():
                screenshot = el.screenshot()
                return base64.b64encode(screenshot).decode("ascii")
        except Exception:
            continue

    # Last resort: screenshot the login modal
    modal_selectors = [
        ".login_dialog",
        ".mod_login",
        "[class*='login']",
    ]
    for sel in modal_selectors:
        try:
            el = page.query_selector(sel)
            if el and el.is_visible():
                screenshot = el.screenshot()
                return base64.b64encode(screenshot).decode("ascii")
        except Exception:
            continue

    return ""


def _qq_poll(page, context) -> str:
    """Poll cookies for QQ music keys. Returns 'success', 'expired', or 'error'."""
    max_wait = 300  # 5 minutes
    start = time.time()

    while time.time() - start < max_wait:
        time.sleep(2)

        cookies = context.cookies()
        cookie_dict = {c["name"]: c["value"] for c in cookies}

        has_key = ("qqmusic_key" in cookie_dict or "qm_keyst" in cookie_dict)

        if has_key:
            # Filter relevant QQ cookies
            relevant_domains = [".qq.com", ".y.qq.com", "y.qq.com", "qq.com"]
            parts = []
            for c in cookies:
                domain = c.get("domain", "")
                if any(domain.endswith(d) or domain == d for d in relevant_domains):
                    parts.append(f"{c['name']}={c['value']}")
            cookie_str = "; ".join(parts)

            # Try to get nickname from uin cookie
            nickname = cookie_dict.get("uin", "QQ用户")
            if nickname.startswith("o"):
                nickname = nickname[1:]  # uin often has "o" prefix

            emit({
                "type": "login_success",
                "cookies": cookie_str,
                "nickname": nickname,
            })
            return "success"

        # Check for QR expiry
        try:
            for f in page.frames:
                if "ptlogin" in f.url:
                    expired = f.query_selector_all("text=二维码已失效, text=已过期")
                    if expired:
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
