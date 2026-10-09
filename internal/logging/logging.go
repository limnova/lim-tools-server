// Package logging 构造服务的结构化日志器。
//
// 用标准库 log/slog：Go 1.21 起它是官方日志方案，生态已收敛到它，
// 不需要为了日志再引 zap / logrus / zerolog。
package logging

import (
	"log/slog"
	"os"

	"github.com/limnova/lim-tools-server/internal/config"
)

// New 按运行环境返回日志器。
//
// 生产必须输出 JSON：纯文本的多行记录（比如 panic 栈）会被日志采集器
// 拆成多条，JSON 保证一条日志就是一行。
func New(cfg config.Config) *slog.Logger {
	if cfg.IsProduction() {
		return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	}

	// 开发环境用可读文本，本地排查时不用跟 JSON 较劲。
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

// Setup 构造日志器并设为进程级默认，返回同一个实例。
//
// 设为默认后，标准库和第三方库里直接调 slog 的地方会走同一条管道。
//
// 注意 gin 有自己的输出通道（gin.DefaultWriter / DefaultErrorWriter），不受 SetDefault 影响。
// 所以本项目用 gin.New() + 自建中间件，而不是 gin.Default() —— 后者会给每个请求打一行
// [GIN] 格式的访问日志，和结构化日志混在一起，采集器解析不了。
//
// 生产模式（ReleaseMode）下 gin 不再打印路由表，stdout 里只有 JSON；
// 开发模式的路由表是调试用的，留着无妨。
func Setup(cfg config.Config) *slog.Logger {
	logger := New(cfg)
	slog.SetDefault(logger)
	return logger
}
