// Package service 存放业务逻辑。它不依赖 HTTP 层，便于单独测试和复用。
package service

import "time"

// Info 是服务的自述信息。
type Info struct {
	Name      string    `json:"name"`
	Env       string    `json:"env"`
	Version   string    `json:"version"`
	StartedAt time.Time `json:"startedAt"`
}

// InfoService 提供服务的元信息。目前只做一件事，是给后续业务服务留的样板。
type InfoService struct {
	name      string
	env       string
	version   string
	startedAt time.Time
}

// NewInfoService 构造 InfoService。version 由构建时注入，见 Makefile。
func NewInfoService(name, env, version string) *InfoService {
	return &InfoService{
		name:      name,
		env:       env,
		version:   version,
		startedAt: time.Now(),
	}
}

// Info 返回当前的服务信息快照。
func (s *InfoService) Info() Info {
	return Info{
		Name:      s.name,
		Env:       s.env,
		Version:   s.version,
		StartedAt: s.startedAt,
	}
}
