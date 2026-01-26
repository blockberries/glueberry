package pool

import (
	"sync"
	"testing"
)

func TestNewBufferPool(t *testing.T) {
	p := NewBufferPool()
	if p == nil {
		t.Fatal("NewBufferPool() returned nil")
	}
}

func TestBufferPool_Get_Small(t *testing.T) {
	p := NewBufferPool()

	buf := p.Get(100)
	if buf == nil {
		t.Fatal("Get() returned nil")
	}
	if len(*buf) != 0 {
		t.Errorf("buffer length = %d, want 0", len(*buf))
	}
	if cap(*buf) < 100 {
		t.Errorf("buffer capacity = %d, want >= 100", cap(*buf))
	}
}

func TestBufferPool_Get_Medium(t *testing.T) {
	p := NewBufferPool()

	buf := p.Get(2000)
	if buf == nil {
		t.Fatal("Get() returned nil")
	}
	if cap(*buf) < 2000 {
		t.Errorf("buffer capacity = %d, want >= 2000", cap(*buf))
	}
}

func TestBufferPool_Get_Large(t *testing.T) {
	p := NewBufferPool()

	buf := p.Get(50000)
	if buf == nil {
		t.Fatal("Get() returned nil")
	}
	if cap(*buf) < 50000 {
		t.Errorf("buffer capacity = %d, want >= 50000", cap(*buf))
	}
}

func TestBufferPool_Get_VeryLarge(t *testing.T) {
	p := NewBufferPool()

	// Very large buffers are allocated directly
	buf := p.Get(100000)
	if buf == nil {
		t.Fatal("Get() returned nil")
	}
	if cap(*buf) < 100000 {
		t.Errorf("buffer capacity = %d, want >= 100000", cap(*buf))
	}
}

func TestBufferPool_Put_ReturnsToPool(t *testing.T) {
	p := NewBufferPool()

	// Get and modify a buffer
	buf1 := p.Get(100)
	*buf1 = append(*buf1, []byte("hello")...)
	originalPtr := buf1

	// Return to pool
	p.Put(buf1)

	// Get another buffer - should get the same one back
	buf2 := p.Get(100)

	// Check that we got the same buffer back (pointer comparison)
	if buf2 != originalPtr {
		t.Log("Note: Got different buffer, which is OK if pool decided to give us a new one")
	}

	// Length should be reset
	if len(*buf2) != 0 {
		t.Errorf("reused buffer length = %d, want 0", len(*buf2))
	}
}

func TestBufferPool_Put_Nil(t *testing.T) {
	p := NewBufferPool()

	// Should not panic
	p.Put(nil)
}

func TestBufferPool_GetExact(t *testing.T) {
	p := NewBufferPool()

	buf := p.GetExact(256)
	if buf == nil {
		t.Fatal("GetExact() returned nil")
	}
	if len(*buf) != 256 {
		t.Errorf("buffer length = %d, want 256", len(*buf))
	}

	// Buffer should be zeroed
	for i, b := range *buf {
		if b != 0 {
			t.Errorf("buffer[%d] = %d, want 0", i, b)
			break
		}
	}
}

func TestGlobalPool(t *testing.T) {
	buf := GetBuffer(100)
	if buf == nil {
		t.Fatal("GetBuffer() returned nil")
	}

	*buf = append(*buf, []byte("test")...)
	PutBuffer(buf)

	buf2 := GetExactBuffer(50)
	if buf2 == nil {
		t.Fatal("GetExactBuffer() returned nil")
	}
	if len(*buf2) != 50 {
		t.Errorf("buffer length = %d, want 50", len(*buf2))
	}
	PutBuffer(buf2)
}

func TestBufferPool_Concurrent(t *testing.T) {
	p := NewBufferPool()
	var wg sync.WaitGroup
	numGoroutines := 100
	numOps := 1000

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				// Vary the size to hit different pools
				sizes := []int{100, 2000, 50000, 100000}
				size := sizes[j%len(sizes)]

				buf := p.Get(size)
				*buf = append(*buf, byte(j))
				p.Put(buf)
			}
		}()
	}

	wg.Wait()
}

func TestBufferPool_SizeClasses(t *testing.T) {
	p := NewBufferPool()

	tests := []struct {
		size         int
		expectedPool string
	}{
		{100, "small"},
		{SmallBufferSize, "small"},
		{SmallBufferSize + 1, "medium"},
		{DefaultBufferSize, "medium"},
		{DefaultBufferSize + 1, "large"},
		{LargeBufferSize, "large"},
		{LargeBufferSize + 1, "direct"},
	}

	for _, tt := range tests {
		buf := p.Get(tt.size)
		if buf == nil {
			t.Errorf("Get(%d) returned nil", tt.size)
			continue
		}
		if cap(*buf) < tt.size {
			t.Errorf("Get(%d) returned buffer with cap %d < %d", tt.size, cap(*buf), tt.size)
		}
		p.Put(buf)
	}
}

func BenchmarkBufferPool_Get_Small(b *testing.B) {
	p := NewBufferPool()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := p.Get(100)
		p.Put(buf)
	}
}

func BenchmarkBufferPool_Get_Medium(b *testing.B) {
	p := NewBufferPool()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := p.Get(2000)
		p.Put(buf)
	}
}

func BenchmarkBufferPool_Get_Large(b *testing.B) {
	p := NewBufferPool()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := p.Get(50000)
		p.Put(buf)
	}
}

func BenchmarkAlloc_Small(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := make([]byte, 0, 100)
		_ = buf
	}
}

func BenchmarkAlloc_Medium(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := make([]byte, 0, 2000)
		_ = buf
	}
}

func BenchmarkAlloc_Large(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := make([]byte, 0, 50000)
		_ = buf
	}
}
