package api

import (
	"context"
	"sync"
	"time"
)

// Background is the set of loops NewRouterContext starts (the backup
// scheduler, activity-history pruning, the login rate-limit sweep). They
// stop when its context ends; Wait returns once every loop has returned, so
// a backup in progress finishes before the process exits (audit F181).
type Background struct{ wg sync.WaitGroup }

// Wait blocks until every background loop has returned.
func (b *Background) Wait() { b.wg.Wait() }

func (b *Background) start(ctx context.Context, loop func(context.Context)) {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		loop(ctx)
	}()
}

// sleepCtx waits d, or less if ctx ends first; it reports whether the full
// wait elapsed (false means stop).
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
