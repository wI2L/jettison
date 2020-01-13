package jettison

import (
	"reflect"
	"unsafe"
)

// eface is the runtime representation of
// the empty interface.
type eface struct {
	rtype unsafe.Pointer
	word  unsafe.Pointer
}

// sliceHeader is the runtime representation
// of a slice.
type sliceHeader struct {
	Data unsafe.Pointer
	Len  int
	Cap  int
}

// stringHeader is the runtime representation
// of a string.
type stringHeader struct {
	Data unsafe.Pointer
	Len  int
}

//nolint:staticcheck
//go:nosplit
func noescape(p unsafe.Pointer) unsafe.Pointer {
	x := uintptr(p)
	return unsafe.Pointer(x ^ 0)
}

func unpackEface(i interface{}) *eface {
	return (*eface)(unsafe.Pointer(&i))
}

func packEface(p unsafe.Pointer, t reflect.Type, ptr bool) interface{} {
	var i interface{}
	e := (*eface)(unsafe.Pointer(&i))
	e.rtype = unpackEface(t).word

	if ptr {
		// Value is indirect, but interface is
		// direct. We need to load the data at
		// p into the interface data word.
		e.word = *(*unsafe.Pointer)(p)
	} else {
		// Value is direct, and so is the interface.
		e.word = p
	}
	return i
}

// sp2b converts a string pointer to a byte slice.
//go:nosplit
func sp2b(p unsafe.Pointer) []byte {
	shdr := (*stringHeader)(p)
	return *(*[]byte)(unsafe.Pointer(&sliceHeader{
		Data: shdr.Data,
		Len:  shdr.Len,
		Cap:  shdr.Len,
	}))
}
