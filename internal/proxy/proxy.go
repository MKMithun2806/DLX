package proxy

import (
	"math/rand"
	"sync"

	"videodl/internal/models"
)

// Mode identifies a rotation strategy.
const (
	ModeRandom      = "random"
	ModeRoundRobin  = "round_robin"
	ModeSticky      = "sticky"
)

// Pool manages a set of enabled proxies and hands out the next one to use
// according to the configured rotation mode.
type Pool struct {
	mu      sync.Mutex
	proxies []models.Proxy
	mode    string
	rrIdx   int
	sticky  string // currently sticky-selected proxy URL, empty if unset
}

func NewPool(mode string) *Pool {
	if mode == "" {
		mode = ModeRandom
	}
	return &Pool{mode: mode}
}

// SetProxies replaces the working set with only the enabled proxies.
func (p *Pool) SetProxies(all []models.Proxy) {
	p.mu.Lock()
	defer p.mu.Unlock()
	enabled := make([]models.Proxy, 0, len(all))
	for _, pr := range all {
		if pr.Enabled {
			enabled = append(enabled, pr)
		}
	}
	p.proxies = enabled
	if p.rrIdx >= len(enabled) {
		p.rrIdx = 0
	}
}

func (p *Pool) SetMode(mode string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mode = mode
	p.sticky = ""
}

// Next returns the proxy URL to use next, or "" if the pool is empty.
func (p *Pool) Next() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.proxies) == 0 {
		return ""
	}
	switch p.mode {
	case ModeRoundRobin:
		pr := p.proxies[p.rrIdx%len(p.proxies)]
		p.rrIdx++
		return pr.ProxyURL
	case ModeSticky:
		if p.sticky == "" {
			p.sticky = p.proxies[rand.Intn(len(p.proxies))].ProxyURL
		}
		return p.sticky
	default: // random
		return p.proxies[rand.Intn(len(p.proxies))].ProxyURL
	}
}

// Resolve determines the effective proxy URL to use for a single download
// given its proxy mode (global|direct|custom), the global default proxy
// (e.g. settings.ProxyHTTPS), and the pool's rotation policy. An empty
// global proxy combined with a populated pool will draw from the pool.
func (p *Pool) Resolve(downloadProxyMode, customProxy, globalProxy string) string {
	switch downloadProxyMode {
	case "direct":
		return ""
	case "custom":
		return customProxy
	default: // "global" or empty
		if globalProxy != "" {
			return globalProxy
		}
		return p.Next()
	}
}
