package hotserver

import (
	"fmt"
	"net"
	"os"
	"strconv"
)

// systemdListeners 从 systemd 注入的 LISTEN_FDS / LISTEN_PID 构造监听器。
// 非 systemd 启动或变量不匹配时返回 (nil, nil)。
func systemdListeners() ([]net.Listener, error) {
	pidStr := os.Getenv("LISTEN_PID")
	if pidStr == "" {
		return nil, nil
	}
	listenPID, err := strconv.Atoi(pidStr)
	if err != nil {
		return nil, nil
	}
	if listenPID != 0 && listenPID != os.Getpid() {
		return nil, nil
	}

	nStr := os.Getenv("LISTEN_FDS")
	if nStr == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(nStr)
	if err != nil || n <= 0 {
		return nil, fmt.Errorf("invalid LISTEN_FDS: %q", nStr)
	}

	out := make([]net.Listener, 0, n)
	for i := 0; i < n; i++ {
		fd := uintptr(3 + i)
		f := os.NewFile(fd, fmt.Sprintf("systemd-listen-%d", fd))
		ln, err := net.FileListener(f)
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("FileListener fd %d: %w", fd, err)
		}
		out = append(out, ln)
	}
	return out, nil
}
