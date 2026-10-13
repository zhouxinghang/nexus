package checker

import (
	"context"
	"sync"
	"time"
)

// FakeProber is an injectable Prober for tests. It returns canned results per
// URL (or a default) and records how many probes it has served, so tests can
// drive health checks deterministically without any real network access.
type FakeProber struct {
	mu       sync.Mutex
	results  map[string]ProbeResult
	fallback ProbeResult
	Calls    int
}

// NewFakeProber builds a FakeProber whose default result is "up" (status 200).
func NewFakeProber() *FakeProber {
	return &FakeProber{
		results:  map[string]ProbeResult{},
		fallback: ProbeResult{Up: true, Status: 200, Latency: time.Millisecond},
	}
}

// SetResult pins the result returned for a specific URL.
func (f *FakeProber) SetResult(url string, r ProbeResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results[url] = r
}

// SetDefault changes the result returned for URLs without a pinned result.
func (f *FakeProber) SetDefault(r ProbeResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fallback = r
}

func (f *FakeProber) Probe(_ context.Context, url string) ProbeResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls++
	if r, ok := f.results[url]; ok {
		return r
	}
	return f.fallback
}

var _ Prober = (*FakeProber)(nil)
