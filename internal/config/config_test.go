package config_test

import (
	"testing"
	"time"

	"github.com/limnova/lim-tools-server/internal/config"
)

// 所有会被读取的变量。每个用例开始前统一清空，避免用例之间互相污染。
var allKeys = []string{
	"LIM_TOOLS_ADDR",
	"LIM_TOOLS_ENV",
	"LIM_TOOLS_SHUTDOWN_TIMEOUT",
	"LIM_TOOLS_DB_HOST",
	"LIM_TOOLS_DB_PORT",
	"LIM_TOOLS_DB_NAME",
	"LIM_TOOLS_DB_USER",
	"LIM_TOOLS_DB_PASSWORD",
	"LIM_TOOLS_DB_SSLMODE",
}

// defaults 是变量全部为空时的期望配置。
func defaults() config.Config {
	return config.Config{
		Addr:            ":8080",
		Env:             "development",
		ShutdownTimeout: 10 * time.Second,
		DB: config.DB{
			Host:    "127.0.0.1",
			Port:    5432,
			SSLMode: "disable",
		},
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    config.Config
		wantErr bool
	}{
		{
			name: "未设置时全部回落默认值",
			want: defaults(),
		},
		{
			name: "空字符串等同未设置",
			env:  map[string]string{"LIM_TOOLS_ADDR": "", "LIM_TOOLS_DB_PORT": ""},
			want: defaults(),
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
				DB: config.DB{
					Host:    "127.0.0.1",
					Port:    5432,
					SSLMode: "disable",
				},
			},
		},
		{
			name: "读取数据库配置",
			env: map[string]string{
				"LIM_TOOLS_DB_HOST":     "db.internal",
				"LIM_TOOLS_DB_PORT":     "6543",
				"LIM_TOOLS_DB_NAME":     "limtools",
				"LIM_TOOLS_DB_USER":     "limadmin",
				"LIM_TOOLS_DB_PASSWORD": "s3cret",
				"LIM_TOOLS_DB_SSLMODE":  "require",
			},
			want: config.Config{
				Addr:            ":8080",
				Env:             "development",
				ShutdownTimeout: 10 * time.Second,
				DB: config.DB{
					Host:     "db.internal",
					Port:     6543,
					Name:     "limtools",
					User:     "limadmin",
					Password: "s3cret",
					SSLMode:  "require",
				},
			},
		},
		{
			name: "零时长回落默认值",
			env:  map[string]string{"LIM_TOOLS_SHUTDOWN_TIMEOUT": "0s"},
			want: defaults(),
		},
		{
			name: "负时长回落默认值",
			env:  map[string]string{"LIM_TOOLS_SHUTDOWN_TIMEOUT": "-1s"},
			want: defaults(),
		},
		{
			// 与旧实现的行为差异：非法值不再静默回落，而是启动即失败。
			name:    "时长非法时报错",
			env:     map[string]string{"LIM_TOOLS_SHUTDOWN_TIMEOUT": "not-a-duration"},
			wantErr: true,
		},
		{
			name:    "数据库端口非法时报错",
			env:     map[string]string{"LIM_TOOLS_DB_PORT": "not-a-port"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range allKeys {
				t.Setenv(key, "")
			}
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			got, err := config.Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() 期望返回错误，实际得到 %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() 返回意外错误: %v", err)
			}
			if got != tt.want {
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
