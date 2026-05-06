// systemd socket activation 版 demo：监听由 .socket 单元持有，服务进程通过 LISTEN_FDS 接管。
// 热更新二进制：替换可执行文件后执行 systemctl restart（见 deploy/ 与 NOTES.txt）。
// 须在 systemd 下由 .socket 拉起；直接 go run 会因缺少 LISTEN_FDS 而失败。
package main

import (
	"fmt"
	"log"
	"os"

	"hotupgrade-systemd-demo/internal/hotserver"
)

func main() {
	log.SetPrefix(fmt.Sprintf("[pid=%d] ", os.Getpid()))
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)

	log.Println(demoBuildLabel())

	srv := &hotserver.Server{
		Logger:  log.Default(),
		Handler: newDemoHandler(),
	}

	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}
