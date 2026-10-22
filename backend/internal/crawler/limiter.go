package crawler

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"
)

// DomainLimiter is a per-domain token bucket. Every host gets its own bucket
// that refills at a fixed rate up to a burst ceiling, and the limiter is shared
// by all crawl jobs so two jobs against the same site can't double the load.
type DomainLimiter struct {
	rate  float64 // tokens added per second
	burst float64 // bucket capacity

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	mu     sync.Mutex
	tokens float64 // may go negative: that is reserved "debt" owed by queued waiters
	last   time.Time
}

// NewDomainLimiter returns a limiter allowing ratePerSecond requests per
// second per host, with bursts of up to burst requests.
func NewDomainLimiter(ratePerSecond float64, burst int) *DomainLimiter {
	if ratePerSecond <= 0 {
		ratePerSecond = 1
	}
	if burst < 1 {
		burst = 1
	}
	return &DomainLimiter{
		rate:    ratePerSecond,
		burst:   float64(burst),
		buckets: make(map[string]*bucket),
	}
}

func (d *DomainLimiter) bucketFor(host string) *bucket {
	host = strings.ToLower(host)
	d.mu.Lock()
	defer d.mu.Unlock()
	b, ok := d.buckets[host]
	if !ok {
		b = &bucket{tokens: d.burst, last: time.Now()}
		d.buckets[host] = b
	}
	return b
}

// Wait blocks until a request to host is allowed, or ctx is cancelled, in which
// case it returns ctx.Err() and gives the reserved token back.
func (d *DomainLimiter) Wait(ctx context.Context, host string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b := d.bucketFor(host)
	delay := b.reserve(time.Now(), d.rate, d.burst)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		b.refund(d.burst)
		return ctx.Err()
	}
}

// reserve takes one token and returns how long the caller must wait before
// using it. Concurrent callers therefore queue up fairly instead of racing.
func (b *bucket) reserve(now time.Time, rate, burst float64) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = math.Min(burst, b.tokens+elapsed*rate)
		b.last = now
	}
	b.tokens--
	if b.tokens >= 0 {
		return 0
	}
	return time.Duration(-b.tokens / rate * float64(time.Second))
}

func (b *bucket) refund(burst float64) {
	b.mu.Lock()
	b.tokens = math.Min(burst, b.tokens+1)
	b.mu.Unlock()
}
