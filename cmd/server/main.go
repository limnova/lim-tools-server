// Command server 是 Lim Tools 后端的入口。
//
// main 只做三件事：读配置、装配依赖、启动服务。业务逻辑不写在这里。
package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

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

	// 日志尚未装配，装配前的失败只能用 slog 默认 handler 报出去。
	if err := loadDotEnv(); err != nil {
		slog.Warn("could not load .env", "error", err)
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
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

// loadDotEnv 把 .env 灌进进程环境，省去本地开发手动 export。
//
// 只在非生产环境生效：生产的配置应当来自真实部署环境，留一个 .env 会把部署问题掩盖成
// 本地文件问题。已存在的环境变量优先，godotenv 不覆盖它们。.env 不存在是正常情况
// （CI、裸机部署），不算错误。
func loadDotEnv() error {
	if os.Getenv("LIM_TOOLS_ENV") == "production" {
		return nil
	}
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
