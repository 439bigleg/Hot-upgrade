#!/usr/bin/env bash
# 固定节拍：每隔 INTERVAL_SEC 秒发起一次请求（不等待上次响应结束），总时长 DURATION_SEC。
# 若单次响应超过间隔，会出现并发中的多个 curl。
# 用法: ./poll-hour.sh
#      ./poll-hour.sh 'http://127.0.0.1:8085/custom'
#      ./poll-hour.sh 'http://127.0.0.1:8085/' 3600 5

set -uo pipefail

URL="${1:-http://127.0.0.1:8085/}"
DURATION_SEC="${2:-3600}" # 默认 3600 秒 = 1 小时
INTERVAL_SEC="${3:-5}"

START_TS=$(date +%s)
END_TS=$((START_TS + DURATION_SEC))
n=0

echo "URL=$URL  duration=${DURATION_SEC}s  fixed interval=${INTERVAL_SEC}s between starts (no wait for response)"
echo "until $(date -r "$END_TS" '+%Y-%m-%d %H:%M:%S' 2>/dev/null || date -j -f '%s' "$END_TS" '+%Y-%m-%d %H:%M:%S')"
echo "---"

while (( $(date +%s) < END_TS )); do
	n=$((n + 1))
	now=$(date '+%Y-%m-%d %H:%M:%S')
	echo "$now #$n fired"

	(
		id=$n
		if out=$(curl -sS --max-time 120 "$URL"); then
			printf '%s #%d done OK len=%s body=%q\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$id" "${#out}" "$out"
		else
			ec=$?
			printf '%s #%d done FAIL curl_exit=%s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$id" "$ec"
		fi
	) &

	now_ts=$(date +%s)
	remaining=$((END_TS - now_ts))
	if (( remaining <= 0 )); then
		break
	fi
	if (( remaining < INTERVAL_SEC )); then
		sleep "$remaining"
		break
	fi
	sleep "$INTERVAL_SEC"
done

echo "--- waiting for in-flight requests..."
wait
echo "--- done: $n requests fired"
