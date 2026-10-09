// Package config 负责从环境变量加载服务配置。
//
// 按 12-Factor App 的约定，配置只来自环境变量，不读配置文件、不带默认的敏感值。
package config

import (
	"os"
	"time"
)

const (
	defaultAddr            = ":8080"
	defaultEnv             = "development"
	defaultShutdownTimeout = 10 * time.Second
)

// Config 是服务运行所需的全部配置。
type Config struct {
	// Addr 是 HTTP 监听地址，例如 ":8080"。
	Addr string
	// Env 是运行环境标识：development / staging / production。
	Env string
	// ShutdownTimeout 是收到退出信号后，等待在途请求结束的最长时间。
	ShutdownTimeout time.Duration
}

// IsProduction 用来决定日志级别、gin 运行模式这类随环境切换的行为。
func (c Config) IsProduction() bool {
	return c.Env == "production"
}

// Load 从环境变量读取配置，未设置或无法解析时回落到默认值。
func Load() Config {
	return Config{
		Addr:            envOr("LIM_TOOLS_ADDR", defaultAddr),
		Env:             envOr("LIM_TOOLS_ENV", defaultEnv),
		ShutdownTimeout: durationOr("LIM_TOOLS_SHUTDOWN_TIMEOUT", defaultShutdownTimeout),
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationOr(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
