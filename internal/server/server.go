// Package server 组装 HTTP 服务并管理其生命周期。
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/limnova/lim-tools-server/internal/config"
	"github.com/limnova/lim-tools-server/internal/handler"
	"github.com/limnova/lim-tools-server/internal/middleware"
)

// Server 包装 http.Server，负责启动与优雅关闭。
type Server struct {
	http *http.Server
	cfg  config.Config
	log  *slog.Logger
}

// New 组装 gin 引擎并返回 Server，依赖全部由调用方注入。
func New(cfg config.Config, log *slog.Logger, routes *handler.Handler) *Server {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	// 顺序有讲究：Recovery 在最外层才能兜住后面所有中间件和 handler 的 panic；
	// RequestID 必须早于日志，否则日志里没有 ID 可带。
	engine.Use(
		middleware.Recovery(),
		middleware.RequestID(),
		middleware.WithRequestLogger(log),
		middleware.AccessLog(),
	)
	routes.Register(engine)

	return &Server{
		cfg: cfg,
		log: log,
		http: &http.Server{
			Addr:              cfg.Addr,
			Handler:           engine,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
	}
}

// Run 启动 HTTP 服务并阻塞，直到 ctx 被取消（进程收到退出信号），
// 然后停止接收新连接并等待在途请求结束。
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		s.log.InfoContext(ctx, "http server listening", "addr", s.http.Addr, "env", s.cfg.Env)
		err := s.http.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	s.log.InfoContext(ctx, "shutdown signal received", "timeout", s.cfg.ShutdownTimeout)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
	defer cancel()

	if err := s.http.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	return nil
}
