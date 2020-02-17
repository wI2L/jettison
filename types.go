package jettison

import (
	"encoding"
	"encoding/json"
	"reflect"
	"sync"
	"time"
	"unsafe"
)

var (
	timeTimeType           = reflect.TypeOf(time.Time{})
	timeDurationType       = reflect.TypeOf(time.Duration(0))
	syncMapType            = reflect.TypeOf((*sync.Map)(nil)).Elem()
	jsonNumberType         = reflect.TypeOf(json.Number(""))
	jsonRawMessageType     = reflect.TypeOf(json.RawMessage(nil))
	jsonMarshalerType      = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	textMarshalerType      = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	appendMarshalerType    = reflect.TypeOf((*AppendMarshaler)(nil)).Elem()
	appendMarshalerCtxType = reflect.TypeOf((*AppendMarshalerCtx)(nil)).Elem()
)

var emptyFnCache sync.Map // map[reflect.Type]emptyFunc

// emptyFunc is a function that returns whether a
// value pointed by an unsafe.Pointer represents the
// zero value of its type.
type emptyFunc func(unsafe.Pointer) bool

// marshalerEncodeFunc is a function that appends
// the result of a marshaler method call to dst.
type marshalerEncodeFunc func(interface{}, []byte, encOpts, reflect.Type) ([]byte, error)

func isBasicType(t reflect.Type) bool {
	return isBoolean(t) || isString(t) || isFloatingPoint(t) || isInteger(t)
}

func isBoolean(t reflect.Type) bool { return t.Kind() == reflect.Bool }
func isString(t reflect.Type) bool  { return t.Kind() == reflect.String }

func isFloatingPoint(t reflect.Type) bool {
	kind := t.Kind()
	if kind == reflect.Float32 || kind == reflect.Float64 {
		return true
	}
	return false
}

func isInteger(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr:
		return true
	default:
		return false
	}
}

func isInlined(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Ptr, reflect.Map:
		return true
	case reflect.Struct:
		return t.NumField() == 1 && isInlined(t.Field(0).Type)
	default:
		return false
	}
}

func isNilable(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map:
		return true
	}
	return false
}

// cachedEmptyFuncOf is similar to emptyFuncOf, but
// returns a cached function, to avoid duplicates.
func cachedEmptyFuncOf(t reflect.Type) emptyFunc {
	if fn, ok := emptyFnCache.Load(t); ok {
		return fn.(emptyFunc)
	}
	fn, _ := emptyFnCache.LoadOrStore(t, emptyFuncOf(t))
	return fn.(emptyFunc)
}

// emptyFuncOf returns a function that can be used to
// determine if a value pointed by an unsafe,Pointer
// represents the zero-value of type t.
func emptyFuncOf(t reflect.Type) emptyFunc {
	switch t.Kind() {
	case reflect.Bool:
		return func(p unsafe.Pointer) bool {
			return !*(*bool)(p)
		}
	case reflect.String:
		return func(p unsafe.Pointer) bool {
			return (*stringHeader)(p).Len == 0
		}
	case reflect.Int:
		return func(p unsafe.Pointer) bool {
			return *(*int)(p) == 0
		}
	case reflect.Int8:
		return func(p unsafe.Pointer) bool {
			return *(*int8)(p) == 0
		}
	case reflect.Int16:
		return func(p unsafe.Pointer) bool {
			return *(*int16)(p) == 0
		}
	case reflect.Int32:
		return func(p unsafe.Pointer) bool {
			return *(*int32)(p) == 0
		}
	case reflect.Int64:
		return func(p unsafe.Pointer) bool {
			return *(*int64)(p) == 0
		}
	case reflect.Uint:
		return func(p unsafe.Pointer) bool {
			return *(*uint)(p) == 0
		}
	case reflect.Uint8:
		return func(p unsafe.Pointer) bool {
			return *(*uint8)(p) == 0
		}
	case reflect.Uint16:
		return func(p unsafe.Pointer) bool {
			return *(*uint16)(p) == 0
		}
	case reflect.Uint32:
		return func(p unsafe.Pointer) bool {
			return *(*uint32)(p) == 0
		}
	case reflect.Uint64:
		return func(p unsafe.Pointer) bool {
			return *(*uint64)(p) == 0
		}
	case reflect.Uintptr:
		return func(p unsafe.Pointer) bool {
			return *(*uintptr)(p) == 0
		}
	case reflect.Float32:
		return func(p unsafe.Pointer) bool {
			return *(*float32)(p) == 0
		}
	case reflect.Float64:
		return func(p unsafe.Pointer) bool {
			return *(*float64)(p) == 0
		}
	case reflect.Map:
		return func(p unsafe.Pointer) bool {
			return maplen(*(*unsafe.Pointer)(p)) == 0
		}
	case reflect.Ptr:
		return func(p unsafe.Pointer) bool {
			return *(*unsafe.Pointer)(p) == nil
		}
	case reflect.Interface:
		return func(p unsafe.Pointer) bool {
			return *(*unsafe.Pointer)(p) == nil
		}
	case reflect.Slice:
		return func(p unsafe.Pointer) bool {
			return (*sliceHeader)(p).Len == 0
		}
	case reflect.Array:
		if t.Len() == 0 {
			return func(unsafe.Pointer) bool { return true }
		}
	}
	return func(unsafe.Pointer) bool { return false }
}
