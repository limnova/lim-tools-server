package config_test

import (
	"testing"
	"time"

	"github.com/limnova/lim-tools-server/internal/config"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want config.Config
	}{
		{
			name: "未设置时全部回落默认值",
			want: config.Config{
				Addr:            ":8080",
				Env:             "development",
				ShutdownTimeout: 10 * time.Second,
			},
		},
		{
			name: "从环境变量读取",
			env: map[string]string{
				"LIM_TOOLS_ADDR":             ":9000",
				"LIM_TOOLS_ENV":              "production",
				"LIM_TOOLS_SHUTDOWN_TIMEOUT": "30s",
			},
			want: config.Config{
				Addr:            ":9000",
				Env:             "production",
				ShutdownTimeout: 30 * time.Second,
			},
		},
		{
			name: "时长解析失败时回落默认值",
			env:  map[string]string{"LIM_TOOLS_SHUTDOWN_TIMEOUT": "not-a-duration"},
			want: config.Config{
				Addr:            ":8080",
				Env:             "development",
				ShutdownTimeout: 10 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			if got := config.Load(); got != tt.want {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestIsProduction(t *testing.T) {
	if (config.Config{Env: "production"}).IsProduction() != true {
		t.Error("production 应判定为生产环境")
	}
	if (config.Config{Env: "development"}).IsProduction() != false {
		t.Error("development 不应判定为生产环境")
	}
}
