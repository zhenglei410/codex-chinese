#!/usr/bin/env bash
# 一键发布到 GitHub（需要已安装并登录 gh）。
# 用法： bash scripts/publish.sh
# 可选： REPO=my-name VIS=public bash scripts/publish.sh
set -euo pipefail
cd "$(dirname "$0")/.."

REPO="${REPO:-codex-chinese}"
VIS="${VIS:-public}"
DESC="Force the Codex desktop app to use its built-in zh-CN interface, even when Statsig/VPN is unreachable."

if ! command -v gh >/dev/null 2>&1; then
  cat <<'EOF'
未检测到 gh CLI。二选一：

A. 安装并登录后重跑本脚本（推荐）
   https://cli.github.com/
   gh auth login

B. 手动推送
   git remote add origin git@github.com:<你的用户名>/codex-chinese.git
   git branch -M main
   git push -u origin main
EOF
  exit 1
fi

if ! gh auth status >/dev/null 2>&1; then
  echo "gh 未登录，请先执行： gh auth login"
  exit 1
fi

if git remote get-url origin >/dev/null 2>&1; then
  echo "已存在 origin，直接推送 ..."
  git push -u origin HEAD
else
  echo "创建远程仓库并推送 ..."
  gh repo create "$REPO" "--$VIS" --source=. --remote=origin --push --description "$DESC"
fi

echo "完成：$(gh repo view --json url -q .url 2>/dev/null || echo '已推送')"
