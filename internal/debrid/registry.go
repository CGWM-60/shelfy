package debrid

import (
	"fmt"
	"sort"
	"sync"
)

type Registry struct {
	mu        sync.RWMutex
	providers map[string]DebridProvider
}

func NewRegistry() *Registry {
	return &Registry{providers: map[string]DebridProvider{}}
}

func (r *Registry) Register(provider DebridProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[provider.Name()] = provider
}

func (r *Registry) Get(name string) (DebridProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %s not registered", name)
	}
	return provider, nil
}

func (r *Registry) List() []DebridProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(r.providers))
	for k := range r.providers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]DebridProvider, 0, len(keys))
	for _, k := range keys {
		out = append(out, r.providers[k])
	}
	return out
}
