package ratelimit

import (
	"context"
	"sync"
	"time"
)

type Limiter struct {
	mu      sync.Mutex
	max     int
	window  time.Duration
	now     func() time.Time
	entries map[string]*entry
}

type entry struct {
	count int
	start time.Time
}

func New(max int, window time.Duration) *Limiter {
	return &Limiter{
		max:     max,
		window:  window,
		now:     time.Now,
		entries: map[string]*entry{},
	}
}

func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	e, exists := l.entries[key]
	if !exists || now.Sub(e.start) >= l.window {
		e = &entry{start: now}
		l.entries[key] = e
	}

	e.count++
	return e.count <= l.max
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.entries, key)
}

func (l *Limiter) Sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()

	for key, e := range l.entries {
		if !e.start.Add(l.window).After(l.now()) {
			delete(l.entries, key)
		}
	}
}

func (l *Limiter) Run(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.Sweep()
		}
	}
}
