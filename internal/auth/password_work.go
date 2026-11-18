package auth

import (
	"context"
	"sync"
	"time"
)

const passwordConcurrency = 2
const passwordBurst = 12

// Bound both memory-hard work and its sustained rate, without a waiting queue.
// The zero value is usable, including for bootstrap and direct Store callers.
type passwordWorkBudget struct {
	mu      sync.Mutex
	active  int
	tokens  float64
	updated time.Time
}

func (b *passwordWorkBudget) acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if b.updated.IsZero() {
		b.tokens = passwordBurst
	} else {
		b.tokens = min(passwordBurst, b.tokens+now.Sub(b.updated).Seconds())
	}
	b.updated = now
	if b.active >= passwordConcurrency || b.tokens < 1 {
		return nil, ErrBusy
	}
	b.active++
	b.tokens--
	return func() { b.mu.Lock(); b.active--; b.mu.Unlock() }, nil
}
