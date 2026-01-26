// Package pool provides memory pooling utilities for reducing GC pressure.
package pool

import (
	"sync"
)

const (
	// DefaultBufferSize is the default initial capacity for pooled buffers.
	DefaultBufferSize = 4096

	// SmallBufferSize is used for small messages (< 1KB).
	SmallBufferSize = 1024

	// LargeBufferSize is used for large messages.
	LargeBufferSize = 65536
)

// BufferPool provides pooled byte slices to reduce allocation overhead.
// It maintains separate pools for different size classes to avoid wasting memory.
type BufferPool struct {
	smallPool  sync.Pool // For buffers up to SmallBufferSize
	mediumPool sync.Pool // For buffers up to DefaultBufferSize
	largePool  sync.Pool // For buffers up to LargeBufferSize
}

// NewBufferPool creates a new buffer pool.
func NewBufferPool() *BufferPool {
	return &BufferPool{
		smallPool: sync.Pool{
			New: func() any {
				buf := make([]byte, 0, SmallBufferSize)
				return &buf
			},
		},
		mediumPool: sync.Pool{
			New: func() any {
				buf := make([]byte, 0, DefaultBufferSize)
				return &buf
			},
		},
		largePool: sync.Pool{
			New: func() any {
				buf := make([]byte, 0, LargeBufferSize)
				return &buf
			},
		},
	}
}

// Get returns a buffer with at least the specified capacity.
// The returned buffer has length 0 but sufficient capacity.
// Call Put when done to return the buffer to the pool.
func (p *BufferPool) Get(size int) *[]byte {
	if size <= SmallBufferSize {
		buf := p.smallPool.Get().(*[]byte)
		*buf = (*buf)[:0]
		return buf
	}
	if size <= DefaultBufferSize {
		buf := p.mediumPool.Get().(*[]byte)
		*buf = (*buf)[:0]
		return buf
	}
	if size <= LargeBufferSize {
		buf := p.largePool.Get().(*[]byte)
		*buf = (*buf)[:0]
		return buf
	}
	// For very large buffers, allocate directly (don't pool)
	buf := make([]byte, 0, size)
	return &buf
}

// Put returns a buffer to the pool.
// The buffer should not be used after calling Put.
func (p *BufferPool) Put(buf *[]byte) {
	if buf == nil {
		return
	}
	cap := cap(*buf)
	*buf = (*buf)[:0] // Reset length

	// Return to appropriate pool based on capacity
	if cap <= SmallBufferSize {
		p.smallPool.Put(buf)
	} else if cap <= DefaultBufferSize {
		p.mediumPool.Put(buf)
	} else if cap <= LargeBufferSize {
		p.largePool.Put(buf)
	}
	// Very large buffers are not pooled, let GC handle them
}

// GetExact returns a buffer with exactly the specified length.
// The buffer is zeroed if it was reused from the pool.
func (p *BufferPool) GetExact(size int) *[]byte {
	buf := p.Get(size)
	if cap(*buf) < size {
		// Shouldn't happen for sizes <= LargeBufferSize
		*buf = make([]byte, size)
		return buf
	}
	*buf = (*buf)[:size]
	// Zero the buffer for safety
	clear(*buf)
	return buf
}

// global is the default global buffer pool.
var global = NewBufferPool()

// GetBuffer returns a buffer from the global pool with at least the specified capacity.
func GetBuffer(size int) *[]byte {
	return global.Get(size)
}

// PutBuffer returns a buffer to the global pool.
func PutBuffer(buf *[]byte) {
	global.Put(buf)
}

// GetExactBuffer returns a buffer from the global pool with exactly the specified length.
func GetExactBuffer(size int) *[]byte {
	return global.GetExact(size)
}
