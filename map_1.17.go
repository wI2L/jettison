// +build !go1.18

package jettison

import "unsafe"

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
