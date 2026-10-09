// Package config 负责从环境变量加载服务配置。
//
// 按 12-Factor App 的约定，配置只来自环境变量，不读配置文件、不带默认的敏感值。
// 变量名到字段的映射由 struct tag 声明，解析交给 caarlos0/env。
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// defaultShutdownTimeout 与 Config.ShutdownTimeout 上的 envDefault 保持一致：
// tag 负责"变量未设置"的情况，这个常量负责"设置了但值不合理"的兜底。
const defaultShutdownTimeout = 10 * time.Second

// Config 是服务运行所需的全部配置。
type Config struct {
	// Addr 是 HTTP 监听地址，例如 ":8080"。
	Addr string `env:"LIM_TOOLS_ADDR" envDefault:":8080"`
	// Env 是运行环境标识：development / staging / production。
	Env string `env:"LIM_TOOLS_ENV" envDefault:"development"`
	// ShutdownTimeout 是收到退出信号后，等待在途请求结束的最长时间。
	// 零值或负值会让关闭上下文立即过期并强制断开在途请求，因此由 Load 兜底成默认值。
	ShutdownTimeout time.Duration `env:"LIM_TOOLS_SHUTDOWN_TIMEOUT" envDefault:"10s"`
	// DB 是 PostgreSQL 连接配置。
	DB DB `envPrefix:"LIM_TOOLS_DB_"`
}

// DB 是 PostgreSQL 连接配置。
//
// Name / User / Password 暂时不是 Required：数据库连接层还没落地，
// 服务需要在完全没有数据库配置的情况下也能起来。等连接层接上后再补 required。
type DB struct {
	// Host 是数据库主机。
	Host string `env:"HOST" envDefault:"127.0.0.1"`
	// Port 是数据库端口。
	Port int `env:"PORT" envDefault:"5432"`
	// Name 是数据库名。
	Name string `env:"NAME"`
	// User 是连接使用的数据库账号。
	User string `env:"USER"`
	// Password 是连接账号的密码，没有默认值。
	Password string `env:"PASSWORD"`
	// SSLMode 是 libpq 风格的 SSL 模式：disable / require / verify-full 等。
	SSLMode string `env:"SSLMODE" envDefault:"disable"`
}

// IsProduction 用来决定日志级别、gin 运行模式这类随环境切换的行为。
func (c Config) IsProduction() bool {
	return c.Env == "production"
}

// Load 从环境变量读取配置。
//
// 变量缺失或格式非法都会返回错误，由调用方决定怎么处理 —— 配置错误在启动时暴露，
// 好过带着一个可能是笔误的值继续跑。
func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse config from environment: %w", err)
	}

	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = defaultShutdownTimeout
	}

	return cfg, nil
}
