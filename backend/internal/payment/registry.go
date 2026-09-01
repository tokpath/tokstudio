package payment

import (
	"sort"
	"strings"
	"sync"
)

// Registry 是支付插件目录。渠道台通道卡、用户收银台按钮、下单校验都从这里读。
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

func NewRegistry() *Registry {
	return &Registry{adapters: map[string]Adapter{}}
}

func (r *Registry) Register(adapter Adapter) error {
	if adapter == nil {
		return ErrInvalidAdapter
	}
	id := strings.ToLower(strings.TrimSpace(adapter.Spec().ID))
	if id == "" {
		return ErrInvalidAdapter
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[id] = adapter
	return nil
}

func (r *Registry) MustRegister(adapter Adapter) {
	if err := r.Register(adapter); err != nil {
		panic(err)
	}
}

func (r *Registry) Get(id string) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[strings.ToLower(strings.TrimSpace(id))]
	return a, ok
}

func (r *Registry) Has(id string) bool {
	_, ok := r.Get(id)
	return ok
}

func (r *Registry) List() []Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Adapter, 0, len(r.adapters))
	for _, a := range r.adapters {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		return adapterRank(out[i]) < adapterRank(out[j])
	})
	return out
}

// UserFacing 是会出现在渠道通道卡和用户收银台的插件（不含人工入账）。
func (r *Registry) UserFacing() []Adapter {
	var out []Adapter
	for _, a := range r.List() {
		if a.Spec().UserFacing {
			out = append(out, a)
		}
	}
	return out
}

func (r *Registry) Specs() []AdapterSpec {
	list := r.List()
	out := make([]AdapterSpec, 0, len(list))
	for _, a := range list {
		out = append(out, a.Spec())
	}
	return out
}

func adapterRank(a Adapter) int {
	switch a.Spec().ID {
	case AdapterAlipay:
		return 1
	case AdapterWechat:
		return 2
	case AdapterStripe:
		return 3
	case AdapterManual:
		return 90
	default:
		return 50
	}
}

var (
	builtinOnce sync.Once
	builtinReg  *Registry
)

func newBuiltinRegistry() *Registry {
	r := NewRegistry()
	r.MustRegister(alipayDriver{})
	r.MustRegister(wechatDriver{})
	r.MustRegister(stripeDriver{})
	r.MustRegister(manualDriver{})
	return r
}

func DefaultRegistry() *Registry {
	builtinOnce.Do(func() {
		builtinReg = newBuiltinRegistry()
	})
	return builtinReg
}

func ValidAdapter(name string) bool {
	return DefaultRegistry().Has(name)
}

func SupportsAutoRenew(adapter string) bool {
	a, ok := DefaultRegistry().Get(adapter)
	return ok && a.Spec().AutoRenew
}

func CredentialSchema(adapter string) []CredentialFieldView {
	a, ok := DefaultRegistry().Get(adapter)
	if !ok {
		return nil
	}
	return a.Spec().Credentials
}
