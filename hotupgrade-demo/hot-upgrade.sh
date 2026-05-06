#!/usr/bin/env bash
# 热升级一键脚本：把新二进制部署到当前运行的可执行路径上，并向监听端口的进程发 SIGUSR2。
# 依赖：与本 demo 一致（hotserver 默认 SIGUSR2 + 同一 argv[0] 路径被替换）。
#
# 用法示例：
#   ./hot-upgrade.sh -a ./server_new -t ./server
#   PORT=8085 ./hot-upgrade.sh --artifact ./server_new --target ./server
#   ./hot-upgrade.sh --dry-run -a ./server_new -t ./server
#
# 查找 PID：按监听端口（默认 8085），与 main 默认地址一致。

set -euo pipefail

PORT="${PORT:-8085}"
TARGET="./server"
ARTIFACT=""
DRY_RUN=0

usage() {
	echo "用法: $0 -a <新二进制路径> [-t <部署路径>] [-p <端口>] [--dry-run]" >&2
	echo "  -a, --artifact   必填，构建好的新可执行文件" >&2
	echo "  -t, --target     覆盖路径，默认 ./server（须与当前进程的 argv[0] 实际路径一致）" >&2
	echo "  -p, --port       查找监听进程用的端口，默认 8085" >&2
	echo "      --dry-run    只打印将执行的步骤，不写盘、不发信号" >&2
	exit 1
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	-p | --port)
		PORT="$2"
		shift 2
		;;
	-t | --target)
		TARGET="$2"
		shift 2
		;;
	-a | --artifact)
		ARTIFACT="$2"
		shift 2
		;;
	--dry-run)
		DRY_RUN=1
		shift
		;;
	-h | --help)
		usage
		;;
	*)
		if [[ -z "$ARTIFACT" && -f "$1" ]]; then
			ARTIFACT="$1"
			shift
		else
			usage
		fi
		;;
	esac
done

[[ -n "$ARTIFACT" ]] || usage
[[ -f "$ARTIFACT" ]] || {
	echo "找不到新二进制: $ARTIFACT" >&2
	exit 1
}

PIDS=$(lsof -nP -iTCP:"$PORT" -sTCP:LISTEN -t 2>/dev/null || true)
if [[ -z "$PIDS" ]]; then
	echo "端口 ${PORT} 上没有 LISTEN 进程，确认服务已启动。" >&2
	exit 1
fi

PID=$(echo "$PIDS" | head -1)
N=$(echo "$PIDS" | wc -l | tr -d ' ')
if [[ "$N" -gt 1 ]]; then
	echo "警告: 端口 ${PORT} 上有多个 LISTEN PID，使用第一个: ${PID}" >&2
fi

if [[ "$DRY_RUN" -eq 1 ]]; then
	echo "[dry-run] 将复制: $ARTIFACT -> $TARGET (chmod +x)"
	echo "[dry-run] 将发送: kill -USR2 $PID"
	exit 0
fi

TMP="${TARGET}.tmp.$$"
cp "$ARTIFACT" "$TMP"
chmod +x "$TMP"
mv "$TMP" "$TARGET"

kill -USR2 "$PID"
echo "已部署 -> $TARGET ，已对 pid=$PID 发送 SIGUSR2（端口 ${PORT}）"
