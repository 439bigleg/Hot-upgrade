// Hot upgrade demo: HTTP 业务按「二进制」区分旧/新行为；运行与热升级由 hotserver 组件负责。
//
// 旧二进制（随机 1–10s，返回 hello）:
//
//	go build -tags old -o server .
//
// 新二进制（随机 1–5s，返回 ok）:
//
//	go build -o server.new .
//
// 典型替换流程（路径保持不变，exec 才能加载到新文件）:
//
//	./server &                         # 当前跑的是旧二进制
//	mv server.new server               # 原子替换磁盘上的可执行文件（名称仍为 ./server）
//	kill -USR2 $(pgrep -n server)      # 触发热升级：子进程 exec 同一 argv[0]，读到的是新代码
//
// 运行方式建议 ./server & 或重定向日志，避免前台升级后 shell 提示符与日志交错。
//
// systemd socket activation 示例在并列目录 ../demo-systemd/（仅 LISTEN_FDS，无 SIGUSR2）。
package main

import (
	"fmt"
	"log"
	"os"

	"hotupgrade-demo/hotserver"
)

func main() {
	log.SetPrefix(fmt.Sprintf("[pid=%d] ", os.Getpid()))
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)

	addr := ":8085"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}

	log.Println(demoBuildLabel())

	srv := &hotserver.Server{
		Addr:    addr,
		Logger:  log.Default(),
		Handler: newDemoHandler(),
	}

	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}
