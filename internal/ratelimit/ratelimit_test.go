package ratelimit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// newTestLimiter возвращает лимитер с ручными часами: advance сдвигает время,
// и ждать окно по-настоящему не нужно.
func newTestLimiter(max int, window time.Duration) (l *Limiter, advance func(time.Duration)) {
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	l = New(max, window)
	l.now = func() time.Time { return now }
	return l, func(d time.Duration) { now = now.Add(d) }
}

// requireAllow делает n попыток для key и проверяет результат каждой.
func requireAllow(t *testing.T, l *Limiter, key string, want ...bool) {
	t.Helper()

	for i, w := range want {
		if got := l.Allow(key); got != w {
			t.Fatalf("attempt %d for %q: Allow = %v, want %v", i+1, key, got, w)
		}
	}
}

func TestAllow(t *testing.T) {
	t.Run("max attempts, then refuse", func(t *testing.T) {
		l, _ := newTestLimiter(3, time.Minute)

		requireAllow(t, l, "k", true, true, true, false, false)
	})

	t.Run("new window after the old one expires", func(t *testing.T) {
		l, advance := newTestLimiter(2, time.Minute)
		requireAllow(t, l, "k", true, true, false)

		advance(time.Minute - time.Second)
		requireAllow(t, l, "k", false)

		advance(time.Second)
		requireAllow(t, l, "k", true, true, false)
	})

	t.Run("keys are independent", func(t *testing.T) {
		l, _ := newTestLimiter(1, time.Minute)

		requireAllow(t, l, "a", true, false)
		requireAllow(t, l, "b", true, false)
	})
}

func TestReset(t *testing.T) {
	l, _ := newTestLimiter(2, time.Minute)
	requireAllow(t, l, "k", true, true, false)
	requireAllow(t, l, "other", true, true)

	l.Reset("k")

	requireAllow(t, l, "k", true, true, false)
	requireAllow(t, l, "other", false)

	// Сброс ключа, которого нет, ничего не ломает.
	l.Reset("missing")
}

func TestSweep(t *testing.T) {
	l, advance := newTestLimiter(1, time.Minute)
	l.Allow("old")
	advance(30 * time.Second)
	l.Allow("fresh")
	advance(30 * time.Second) // окно "old" истекло, у "fresh" осталось 30 секунд

	l.Sweep()

	if _, ok := l.entries["old"]; ok {
		t.Errorf("Sweep kept the expired entry %q", "old")
	}
	if _, ok := l.entries["fresh"]; !ok {
		t.Errorf("Sweep removed the entry %q, whose window has not expired yet", "fresh")
	}
	requireAllow(t, l, "fresh", false)
}

func TestAllowConcurrent(t *testing.T) {
	const max, goroutines = 5, 100
	l := New(max, time.Minute)

	var allowed atomic.Int64
	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			if l.Allow("k") {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()

	if got := allowed.Load(); got != max {
		t.Errorf("%d of %d concurrent attempts allowed, want exactly %d", got, goroutines, max)
	}
}

// synctest подменяет время внутри «пузыря»: Sleep и тикер срабатывают сразу,
// а time.Now сдвигается вместе с ними.
func TestRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := New(1, time.Minute)
		l.Allow("k")

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			l.Run(ctx, 10*time.Second)
			close(done)
		}()

		time.Sleep(time.Minute + 10*time.Second)
		synctest.Wait()

		l.mu.Lock()
		n := len(l.entries)
		l.mu.Unlock()
		if n != 0 {
			t.Errorf("%d entries left after the window expired, want Run to sweep them", n)
		}

		cancel()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Errorf("Run did not return after the context was canceled")
		}
	})
}
