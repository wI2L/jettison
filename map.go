package jettison

import (
	"bytes"
	"sync"
	"unsafe"
)

var (
	hiterPool    sync.Pool // *hiter
	mapElemsPool sync.Pool // *mapElems
)

// kv represents a map key/value pair.
type kv struct {
	key    []byte
	keyval []byte
}

type mapElems struct{ s []kv }

// releaseMapElems zeroes the content of the
// map elements slice and resets the length to
// zero before putting it back to the pool.
func releaseMapElems(me *mapElems) {
	for i := range me.s {
		me.s[i] = kv{}
	}
	me.s = me.s[:0]
	mapElemsPool.Put(me)
}

func (m mapElems) Len() int           { return len(m.s) }
func (m mapElems) Swap(i, j int)      { m.s[i], m.s[j] = m.s[j], m.s[i] }
func (m mapElems) Less(i, j int) bool { return bytes.Compare(m.s[i].key, m.s[j].key) < 0 }

// hiter is the runtime representation
// of a hashmap iteration structure.
type hiter struct {
	key unsafe.Pointer
	val unsafe.Pointer

	// remaining fields are ignored but
	// present in the struct so that it
	// can be zeroed for reuse.
	// see hiter in src/runtime/map.go
	_ [6]unsafe.Pointer
	_ uintptr
	_ uint8
	_ bool
	_ [2]uint8
	_ [2]uintptr
}

var zeroHiter = &hiter{}

func newHiter(t, m unsafe.Pointer) *hiter {
	v := hiterPool.Get()
	if v == nil {
		return newmapiter(t, m)
	}
	it := v.(*hiter)
	*it = *zeroHiter
	mapiterinit(t, m, unsafe.Pointer(it))
	return it
}

//go:noescape
//go:linkname newmapiter reflect.mapiterinit
func newmapiter(unsafe.Pointer, unsafe.Pointer) *hiter

//go:noescape
//go:linkname mapiterinit runtime.mapiterinit
func mapiterinit(unsafe.Pointer, unsafe.Pointer, unsafe.Pointer)

//go:noescape
//go:linkname mapiternext reflect.mapiternext
func mapiternext(*hiter)

//go:noescape
//go:linkname maplen reflect.maplen
func maplen(unsafe.Pointer) int
