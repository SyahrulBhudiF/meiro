package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStreamCacheResolvesOnceAndSharesInFlight(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	c := newStreamCache(func(_ context.Context, id, _ string) (string, time.Duration, error) {
		calls.Add(1)
		<-release
		return "url-" + id, time.Minute, nil
	})
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if url, total, err := c.get(context.Background(), "a", "cookie"); err != nil || url != "url-a" || total != time.Minute {
				t.Errorf("get = %q, %v, %v", url, total, err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	_, _, _ = c.get(context.Background(), "a", "cookie")
	if got := calls.Load(); got != 1 {
		t.Errorf("resolved %d times, want 1", got)
	}
}

func TestStreamCacheCallerCancellationDoesNotCancelSharedResolve(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	c := newStreamCache(func(ctx context.Context, _, _ string) (string, time.Duration, error) {
		calls.Add(1)
		close(started)
		select {
		case <-release:
			return "shared-url", time.Minute, nil
		case <-ctx.Done():
			return "", 0, ctx.Err()
		}
	})

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, _, err := c.get(firstCtx, "track", "")
		firstDone <- err
	}()
	<-started

	secondStarted := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		close(secondStarted)
		url, total, err := c.get(context.Background(), "track", "")
		if err == nil && (url != "shared-url" || total != time.Minute) {
			err = errors.New("shared result was incorrect")
		}
		secondDone <- err
	}()
	<-secondStarted

	cancelFirst()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled caller error = %v, want context.Canceled", err)
	}
	close(release)
	if err := <-secondDone; err != nil {
		t.Fatalf("remaining caller error = %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("resolver called %d times, want once", got)
	}
}

func TestStreamCacheHonorsAlreadyCancelledContext(t *testing.T) {
	var calls atomic.Int32
	c := newStreamCache(func(context.Context, string, string) (string, time.Duration, error) {
		calls.Add(1)
		return "url", 0, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := c.get(ctx, "track", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("get error = %v, want context.Canceled", err)
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("resolver called %d times for cancelled request", got)
	}
}

func TestStreamCacheDoesNotKeepFailuresOrOtherSessions(t *testing.T) {
	calls := 0
	fail := true
	c := newStreamCache(func(context.Context, string, string) (string, time.Duration, error) {
		calls++
		if fail {
			return "", 0, errors.New("boom")
		}
		return "url", 0, nil
	})
	if _, _, err := c.get(context.Background(), "a", ""); err == nil {
		t.Fatal("want the resolve error")
	}
	fail = false
	if _, _, err := c.get(context.Background(), "a", ""); err != nil || calls != 2 {
		t.Fatalf("a failure was kept: err %v, calls %d", err, calls)
	}
	_, _, _ = c.get(context.Background(), "a", "signed-in")
	if calls != 3 {
		t.Errorf("a different cookie reused the URL: calls %d", calls)
	}
}

func TestStreamCacheExpiresAndBounds(t *testing.T) {
	now := time.Now()
	calls := 0
	c := newStreamCache(func(context.Context, string, string) (string, time.Duration, error) {
		calls++
		return "url", 0, nil
	})
	c.now = func() time.Time { return now }
	_, _, _ = c.get(context.Background(), "a", "")
	now = now.Add(streamURLTTL + time.Second)
	_, _, _ = c.get(context.Background(), "a", "")
	if calls != 2 {
		t.Errorf("an expired URL was reused: calls %d", calls)
	}
	for i := range streamURLLimit + 5 {
		now = now.Add(time.Second)
		_, _, _ = c.get(context.Background(), string(rune('a'+i)), "")
	}
	if len(c.entries) > streamURLLimit {
		t.Errorf("%d entries kept, limit %d", len(c.entries), streamURLLimit)
	}
}
