package identity

import (
	_ "embed"
	"strings"
	"sync"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
)

// RoleAuthenticated 是 Casbin 里的合成角色：任何已登录主体都带上它。
// 用来覆盖 /v1/me、/v1/partner 这类「只要登录就能进，再在 handler 里收窄」的接口。
const RoleAuthenticated = "authenticated"

//go:embed casbin_model.conf
var casbinModelText string

var (
	enforcerOnce   sync.Once
	enforcerErr    error
	sharedEnforcer *casbin.Enforcer
)

func loadEnforcer() (*casbin.Enforcer, error) {
	enforcerOnce.Do(func() {
		m, err := model.NewModelFromString(casbinModelText)
		if err != nil {
			enforcerErr = err
			return
		}
		e, err := casbin.NewEnforcer(m)
		if err != nil {
			enforcerErr = err
			return
		}
		e.EnableAutoSave(false)
		for _, rule := range casbinPolicy() {
			if _, err := e.AddPolicy(rule.role, rule.object, rule.action); err != nil {
				enforcerErr = err
				return
			}
		}
		sharedEnforcer = e
	})
	return sharedEnforcer, enforcerErr
}

func (s *Service) initEnforcer() error {
	e, err := loadEnforcer()
	if err != nil {
		return err
	}
	s.enforcer = e
	return nil
}

// Allow 用 Casbin 判断该身份能否调用 path + method。
// 默认拒绝：策略里没有的路径一律 false。
func (s *Service) Allow(principal *Principal, path, method string) bool {
	if s == nil || s.enforcer == nil || principal == nil {
		return false
	}
	path = strings.TrimSpace(path)
	method = strings.ToUpper(strings.TrimSpace(method))
	if path == "" || method == "" {
		return false
	}
	subjects := make([]string, 0, len(principal.Roles)+1)
	subjects = append(subjects, RoleAuthenticated)
	subjects = append(subjects, principal.Roles...)
	for _, sub := range subjects {
		ok, err := s.enforcer.Enforce(sub, path, method)
		if err != nil {
			return false
		}
		if ok {
			return true
		}
	}
	return false
}
