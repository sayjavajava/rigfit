// SPDX-License-Identifier: GPL-3.0-only

package reqs

import (
	"context"
	"math"
	"sync"
	"time"
)

// tokenBucket is a minimal hand-rolled rate limiter (§6.1: "token-bucket
// limiter"). golang.org/x/time/rate is not in the approved dependency
// list (§2), and the algorithm is small enough that adding a dependency
// for it isn't justified.
type tokenBucket struct {
	mu           sync.Mutex
	tokens       float64
	capacity     float64
	refillPerSec float64
	last         time.Time
}

func newTokenBucket(capacity float64, refillPerSec float64) *tokenBucket {
	return &tokenBucket{tokens: capacity, capacity: capacity, refillPerSec: refillPerSec, last: time.Now()}
}

// wait blocks until a token is available (or ctx is done), consuming one.
func (b *tokenBucket) wait(ctx context.Context) error {
	for {
		b.mu.Lock()
		now := time.Now()
		b.tokens = math.Min(b.capacity, b.tokens+now.Sub(b.last).Seconds()*b.refillPerSec)
		b.last = now
		if b.tokens >= 1 {
			b.tokens--
			b.mu.Unlock()
			return nil
		}
		waitDur := time.Duration((1 - b.tokens) / b.refillPerSec * float64(time.Second))
		b.mu.Unlock()

		timer := time.NewTimer(waitDur)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
}
