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
	// AccessLog 包住 Recovery，恢复后才能记录最终的 500 状态。
	// Recovery 保护请求 ID、日志注入与 handler；RequestID 早于日志注入。
	engine.Use(
		middleware.AccessLog(),
		middleware.Recovery(),
		middleware.RequestID(),
		middleware.WithRequestLogger(log),
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
		errCh <- s.http.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("listen and serve: %w", err)
	case <-ctx.Done():
	}

	s.log.InfoContext(ctx, "shutdown signal received", "timeout", s.cfg.ShutdownTimeout)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
	defer cancel()

	var shutdownErr error
	if err := s.http.Shutdown(shutdownCtx); err != nil {
		shutdownErr = fmt.Errorf("graceful shutdown: %w", err)
		// Shutdown 超时不会关闭活跃连接；强制关闭以取消请求 context。
		if closeErr := s.http.Close(); closeErr != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("close server: %w", closeErr))
		}
	}
	// 等待监听 goroutine 退出，同时保留与取消信号并发发生的启动错误。
	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return errors.Join(shutdownErr, fmt.Errorf("listen and serve: %w", err))
	}
	return shutdownErr
}
