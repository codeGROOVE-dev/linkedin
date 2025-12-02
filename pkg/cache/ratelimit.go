package cache

import (
	"net/url"
	"sync"
	"time"
)

// DomainRateLimiter enforces a minimum delay between requests to the same domain.
// It is safe for concurrent use from multiple goroutines.
type DomainRateLimiter struct {
	lastRequest sync.Map // map[string]time.Time
	mu          sync.Map // map[string]*sync.Mutex - per-domain locks
	minDelay    time.Duration
}

// NewDomainRateLimiter creates a rate limiter that enforces minDelay between
// requests to the same domain.
func NewDomainRateLimiter(minDelay time.Duration) *DomainRateLimiter {
	return &DomainRateLimiter{
		minDelay: minDelay,
	}
}

// Wait blocks until it's safe to make a request to the given URL's domain.
// It ensures at least minDelay has passed since the last request to that domain.
func (r *DomainRateLimiter) Wait(rawURL string) {
	domain := extractDomain(rawURL)
	if domain == "" {
		return
	}

	// Get or create per-domain mutex
	muI, _ := r.mu.LoadOrStore(domain, &sync.Mutex{})
	mu, ok := muI.(*sync.Mutex)
	if !ok {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	// Check last request time
	if lastI, ok := r.lastRequest.Load(domain); ok {
		if last, ok := lastI.(time.Time); ok {
			elapsed := time.Since(last)
			if elapsed < r.minDelay {
				time.Sleep(r.minDelay - elapsed)
			}
		}
	}

	// Record this request
	r.lastRequest.Store(domain, time.Now())
}

// extractDomain returns the host portion of a URL, or empty string on error.
func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}
