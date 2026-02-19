package ratelimit

import (
	"context"
	"time"
)

// Limiter is a token-bucket rate limiter backed by a channel.
// It is safe for concurrent use.
type Limiter struct {
	tokens chan struct{}
}

// NewLimiter returns a Limiter that allows rps requests per second.
// Returns nil when rps <= 0 (no rate limiting).
// A filler goroutine deposits tokens at 1/rps intervals (capped at burst = int(rps)).
// The filler exits when ctx is cancelled.
func NewLimiter(ctx context.Context, rps float64) *Limiter {
	if rps <= 0 {
		return nil
	}
	burst := int(rps)
	if burst < 1 {
		burst = 1
	}
	l := &Limiter{tokens: make(chan struct{}, burst)}
	interval := time.Duration(float64(time.Second) / rps)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				select {
				case l.tokens <- struct{}{}:
				default: // bucket full; drop token
				}
			}
		}
	}()
	return l
}

// Wait blocks until a token is available or ctx is cancelled.
// Returns false if ctx was cancelled before a token was obtained.
// Safe to call on a nil Limiter (returns true immediately).
func (l *Limiter) Wait(ctx context.Context) bool {
	if l == nil {
		return true
	}
	select {
	case <-l.tokens:
		return true
	case <-ctx.Done():
		return false
	}
}
