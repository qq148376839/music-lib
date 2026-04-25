#!/usr/bin/env python3
"""
环境体检脚本（SessionStart hook）
NAS 音乐管理系统 — Go 项目
"""

import os
import subprocess
from pathlib import Path


def check_env_file():
    """检查 .env 文件是否配置"""
    env_example = Path(".env.example")
    env_file = Path(".env")
    env_local = Path(".env.local")

    if env_example.exists() and not env_file.exists() and not env_local.exists():
        print("⚠️  发现 .env.example 但未找到 .env 或 .env.local，请配置环境变量")
        return False
    return True


def check_git_status():
    """检查 git 状态"""
    try:
        result = subprocess.run(
            ["git", "rev-parse", "--is-inside-work-tree"],
            capture_output=True, text=True, timeout=5
        )
        if result.returncode != 0:
            return True

        result = subprocess.run(
            ["git", "stash", "list"],
            capture_output=True, text=True, timeout=5
        )
        if result.stdout.strip():
            stash_count = len(result.stdout.strip().split("\n"))
            print(f"📌 有 {stash_count} 个 stash 未处理")

        result = subprocess.run(
            ["git", "status", "--porcelain"],
            capture_output=True, text=True, timeout=5
        )
        if result.stdout.strip():
            changed = len(result.stdout.strip().split("\n"))
            print(f"📝 有 {changed} 个文件有未提交的变更")

    except (subprocess.TimeoutExpired, FileNotFoundError):
        pass
    return True


def check_go_env():
    """检查 Go 开发环境"""
    try:
        result = subprocess.run(
            ["go", "version"],
            capture_output=True, text=True, timeout=5
        )
        if result.returncode != 0:
            print("⚠️  Go 未安装或不在 PATH 中")
            return False
    except FileNotFoundError:
        print("⚠️  Go 未安装")
        return False

    if Path("go.mod").exists():
        result = subprocess.run(
            ["go", "mod", "verify"],
            capture_output=True, text=True, timeout=15
        )
        if result.returncode != 0:
            print("⚠️  Go 依赖校验失败，建议运行 go mod tidy")
            return False
    return True


def check_docker():
    """检查 Docker 是否可用（NAS 部署需要）"""
    try:
        result = subprocess.run(
            ["docker", "version", "--format", "{{.Server.Version}}"],
            capture_output=True, text=True, timeout=5
        )
        if result.returncode != 0:
            print("📌 Docker 未运行（部署时需要）")
    except FileNotFoundError:
        print("📌 Docker 未安装（部署时需要）")
    return True


def main():
    print("🔍 环境体检开始...\n")

    checks = [
        ("环境变量", check_env_file),
        ("Git 状态", check_git_status),
        ("Go 环境", check_go_env),
        ("Docker", check_docker),
    ]

    warnings = []
    for name, check_fn in checks:
        if not check_fn():
            warnings.append(name)

    print()
    if warnings:
        print(f"⚠️  体检完成，{len(warnings)} 项需要注意：{', '.join(warnings)}")
    else:
        print("✅ 环境体检通过，一切就绪！")

    dev_log_dir = Path("dev-log")
    if dev_log_dir.exists():
        logs = sorted(dev_log_dir.glob("*.md"), reverse=True)
        if logs:
            latest = logs[0]
            print(f"\n📋 上次 Session 日志：{latest.name}")


if __name__ == "__main__":
    main()
