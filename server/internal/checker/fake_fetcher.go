package checker

import (
	"context"
	"sync"
)

// FakeFetcher is an injectable Fetcher for tests. It returns canned metadata
// (or an error) per URL, or a default, and counts calls — so tests drive
// title/favicon enrichment without any real network access.
type FakeFetcher struct {
	mu          sync.Mutex
	results     map[string]Meta
	errs        map[string]error
	fallback    Meta
	fallbackErr error
	Calls       int
}

// NewFakeFetcher builds a FakeFetcher whose default result is empty metadata.
func NewFakeFetcher() *FakeFetcher {
	return &FakeFetcher{results: map[string]Meta{}, errs: map[string]error{}}
}

// SetResult pins the metadata returned for a specific URL.
func (f *FakeFetcher) SetResult(url string, m Meta) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.errs, url)
	f.results[url] = m
}

// SetError makes fetches of a specific URL fail.
func (f *FakeFetcher) SetError(url string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.results, url)
	f.errs[url] = err
}

// SetDefault changes the result for URLs without a pinned result or error.
func (f *FakeFetcher) SetDefault(m Meta, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fallback, f.fallbackErr = m, err
}

func (f *FakeFetcher) FetchMeta(_ context.Context, url string) (Meta, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls++
	if err, ok := f.errs[url]; ok {
		return Meta{}, err
	}
	if m, ok := f.results[url]; ok {
		return m, nil
	}
	return f.fallback, f.fallbackErr
}

var _ Fetcher = (*FakeFetcher)(nil)
