// Command server 是 Lim Tools 后端的入口。
//
// main 只做三件事：读配置、装配依赖、启动服务。业务逻辑不写在这里。
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/limnova/lim-tools-server/internal/config"
	"github.com/limnova/lim-tools-server/internal/handler"
	"github.com/limnova/lim-tools-server/internal/logging"
	"github.com/limnova/lim-tools-server/internal/server"
	"github.com/limnova/lim-tools-server/internal/service"
)

// version 由构建时通过 -ldflags 注入，见 Makefile。
var version = "dev"

func main() {
	// 收到 SIGINT / SIGTERM 时取消 ctx，触发优雅关闭。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	log := logging.Setup(cfg)

	// 手动构造函数注入，依赖关系一眼可见：
	// config → logging → service → handler → server
	infoSvc := service.NewInfoService("lim-tools-server", cfg.Env, version)
	routes := handler.New(infoSvc)
	srv := server.New(cfg, log, routes)

	if err := srv.Run(ctx); err != nil {
		log.ErrorContext(ctx, "server exited with error", "error", err)
		os.Exit(1)
	}
	log.InfoContext(ctx, "server stopped")
}
