// Command lxgo-server 是 pkg/lx-go 库的独立可执行版本：加载 js/ 目录下的音源脚本，
// 起一个标准 HTTP 服务，兼容 pkg/music（LXClient）期望的 "LX Sync Server" 协议。
//
// 如果是从 pkg/music 里内嵌使用（不想手动起这个独立进程），直接 import
// "github.com/cxjava/open-xiaoai/pkg/lx-go" 这个库包即可，不需要跑这个命令行程序，
// 参见 pkg/music/lx_embedded.go。
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	lxgo "github.com/cxjava/open-xiaoai/pkg/lx-go"
)

func main() {
	jsDir := flag.String("js", "./js", "音源脚本目录")
	addr := flag.String("addr", ":8080", "监听地址")
	flag.Parse()

	registry, err := lxgo.LoadSources(*jsDir)
	if err != nil {
		panic(err)
	}
	defer registry.Close()

	srv := lxgo.NewServer(registry)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	log.Printf("已加载 %d 个音源，server started at %s", len(registry.Sources), *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		panic(fmt.Errorf("listen failed: %w", err))
	}
}
