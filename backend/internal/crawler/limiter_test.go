package crawler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestDomainLimiterBurstThenThrottle(t *testing.T) {
	l := NewDomainLimiter(20, 2) // one token every 50ms, burst of 2
	ctx := context.Background()

	start := time.Now()
	for i := 0; i < 2; i++ {
		if err := l.Wait(ctx, "a.test"); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d > 30*time.Millisecond {
		t.Fatalf("burst should be immediate, took %v", d)
	}

	for i := 0; i < 4; i++ {
		if err := l.Wait(ctx, "a.test"); err != nil {
			t.Fatal(err)
		}
	}
	// Four tokens beyond the burst at 50ms each is ~200ms.
	if d := time.Since(start); d < 180*time.Millisecond {
		t.Fatalf("limiter did not throttle: 6 requests finished in %v", d)
	}
}

func TestDomainLimiterIsolatesDomains(t *testing.T) {
	l := NewDomainLimiter(1, 1)
	ctx := context.Background()
	if err := l.Wait(ctx, "slow.test"); err != nil { // spends slow.test's only token
		t.Fatal(err)
	}

	start := time.Now()
	if err := l.Wait(ctx, "other.test"); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("a different domain should not be throttled, waited %v", d)
	}
}

func TestDomainLimiterHostIsCaseInsensitive(t *testing.T) {
	l := NewDomainLimiter(1, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := l.Wait(ctx, "Example.COM"); err != nil {
		t.Fatal(err)
	}
	if err := l.Wait(ctx, "example.com"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("same host in another case should share a bucket, got err=%v", err)
	}
}

func TestDomainLimiterCancel(t *testing.T) {
	l := NewDomainLimiter(0.5, 1) // 2s per token
	if err := l.Wait(context.Background(), "a.test"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := l.Wait(ctx, "a.test")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("cancelled wait should return promptly, took %v", d)
	}
}

func TestDomainLimiterConcurrent(t *testing.T) {
	l := NewDomainLimiter(1000, 5)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = l.Wait(context.Background(), "a.test")
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("concurrent waiters did not all finish")
	}
}
