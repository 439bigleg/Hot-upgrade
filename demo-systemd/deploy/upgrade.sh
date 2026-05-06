#!/usr/bin/env bash
# 在 Linux 上：安装新二进制后重启 service（socket 单元保持，systemd 再次注入 LISTEN_FDS）。
# 用法: sudo ./upgrade.sh /path/to/new-binary
set -euo pipefail
NEW="${1:?usage: $0 /path/to/hotupgrade-sd-demo}"
TARGET="${TARGET:-/usr/local/bin/hotupgrade-sd-demo}"
SERVICE="${SERVICE:-hotupgrade-sd-demo.service}"

install -m 755 "$NEW" "$TARGET"
systemctl restart "$SERVICE"
echo "installed -> $TARGET , restarted $SERVICE"
