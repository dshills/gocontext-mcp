package indexer

import "sync/atomic"

// IndexLock provides non-blocking lock semantics using atomic operations.
// This replaces sync.Mutex.TryLock() which doesn't exist in Go 1.25.
type IndexLock struct {
	state atomic.Int32 // 0 = unlocked, 1 = locked
}

// TryAcquire attempts to acquire the lock without blocking.
// Returns true if the lock was successfully acquired, false otherwise.
func (l *IndexLock) TryAcquire() bool {
	return l.state.CompareAndSwap(0, 1)
}

// Release releases the lock.
// Must only be called by the goroutine that successfully acquired the lock.
func (l *IndexLock) Release() {
	l.state.Store(0)
}
