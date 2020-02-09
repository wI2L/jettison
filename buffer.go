package jettison

import "sync"

const defaultBufCap = 4096

var bufferPool sync.Pool // *buffer

type buffer struct{ B []byte }

// Reset resets the buffer to be empty.
func (b *buffer) Reset() { b.B = b.B[:0] }

// cachedBuffer returns an empty buffer
// from a pool, or initialize a new one
// with a default capacity.
func cachedBuffer() *buffer {
	v := bufferPool.Get()
	if v != nil {
		buf := v.(*buffer)
		buf.Reset()
		return buf
	}
	return &buffer{
		B: make([]byte, 0, defaultBufCap),
	}
}
