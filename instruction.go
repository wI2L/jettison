package jettison

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"unsafe"
)

var (
	instrCachePtr    unsafe.Pointer // *instrCache
	structInstrCache sync.Map       // map[string]instruction
)

// An instruction appends the JSON representation
// of a value pointed by the unsafe.Pointer p to
// dst and returns the extended buffer.
type instruction func(unsafe.Pointer, []byte, encOpts) ([]byte, error)

// instrCache is an eventually consistent cache that
// maps Go type definitions to dynamically generated
// instructions. The key is unsafe.Pointer instead of
// reflect.Type to improve lookup performance.
type instrCache map[unsafe.Pointer]instruction

func typeID(t reflect.Type) unsafe.Pointer {
	return unpackEface(t).word
}

// cachedInstr returns an instruction to encode the
// given type from a cache, or create one on the fly.
func cachedInstr(t reflect.Type) instruction {
	id := typeID(t)

	if instr, ok := loadInstr(id); ok {
		return instr
	}
	canAddr := t.Kind() == reflect.Ptr

	// canAddr indicates if the input value is addressable.
	// At this point, we only need to know if the value is
	// a pointer, the others instructions will handle that
	// themselves for their type, or pass-by the value.
	instr := newInstruction(t, canAddr, false)
	if isInlined(t) {
		instr = wrapInlineInstr(instr)
	}
	storeInstr(id, instr, loadCache())

	return instr
}

func loadCache() instrCache {
	p := atomic.LoadPointer(&instrCachePtr)
	return *(*instrCache)(unsafe.Pointer(&p))
}

func loadInstr(id unsafe.Pointer) (instruction, bool) {
	cache := loadCache()
	instr, ok := cache[id]
	return instr, ok
}

func storeInstr(key unsafe.Pointer, instr instruction, cache instrCache) {
	newCache := make(instrCache, len(cache)+1)

	// Clone the current cache and add the
	// new instruction.
	for k, v := range cache {
		newCache[k] = v
	}
	newCache[key] = instr

	atomic.StorePointer(
		&instrCachePtr,
		*(*unsafe.Pointer)(unsafe.Pointer(&newCache)),
	)
}

// newInstruction returns an instruction to encode t.
// canAddr and quoted respectively indicates if the
// value to encode is addressable and must enclosed
// with double-quote character in the output.
func newInstruction(t reflect.Type, canAddr, quoted bool) instruction {
	// Go types must be checked first, because a Duration
	// is an int64, json.Number is a string, and both would
	// be interpreted as a basic type. Also, the time.Time
	// type implements the TextMarshaler interface, but we
	// want to use a special instruction instead.
	if ins := newGoTypeInstr(t); ins != nil {
		return ins
	}
	if ins := newMarshalerTypeInstr(t, canAddr); ins != nil {
		return ins
	}
	if ins := newBasicTypeInstr(t, quoted); ins != nil {
		return ins
	}
	switch t.Kind() {
	case reflect.Interface:
		return encodeInterface
	case reflect.Struct:
		return newStructInstr(t, canAddr)
	case reflect.Map:
		return newMapInstr(t)
	case reflect.Slice:
		return newSliceInstr(t)
	case reflect.Array:
		return newArrayInstr(t, canAddr)
	case reflect.Ptr:
		return newPtrInstr(t, quoted)
	}
	return newUnsupportedTypeInstr(t)
}

func newGoTypeInstr(t reflect.Type) instruction {
	switch t {
	case syncMapType:
		return encodeSyncMap
	case timeTimeType:
		return encodeTime
	case timeDurationType:
		return encodeDuration
	case jsonNumberType:
		return encodeNumber
	case jsonRawMessageType:
		return encodeRawMessage
	default:
		return nil
	}
}

// newMarshalerTypeInstr returns an instruction to handle
// a type that implement one of the Marshaler, MarshalerCtx,
// json.Marshal, encoding.TextMarshaler interfaces.
func newMarshalerTypeInstr(t reflect.Type, canAddr bool) instruction {
	isPtr := t.Kind() == reflect.Ptr
	ptrTo := reflect.PtrTo(t)

	switch {
	case t.Implements(appendMarshalerCtxType):
		return newAppendMarshalerCtxInstr(t, false)
	case !isPtr && canAddr && ptrTo.Implements(appendMarshalerCtxType):
		return newAppendMarshalerCtxInstr(t, true)
	case t.Implements(appendMarshalerType):
		return newAppendMarshalerInstr(t, false)
	case !isPtr && canAddr && ptrTo.Implements(appendMarshalerType):
		return newAppendMarshalerInstr(t, true)
	case t.Implements(jsonMarshalerType):
		return newJSONMarshalerInstr(t, false)
	case !isPtr && canAddr && ptrTo.Implements(jsonMarshalerType):
		return newJSONMarshalerInstr(t, true)
	case t.Implements(textMarshalerType):
		return newTextMarshalerInstr(t, false)
	case !isPtr && canAddr && ptrTo.Implements(textMarshalerType):
		return newTextMarshalerInstr(t, true)
	default:
		return nil
	}
}

func newBasicTypeInstr(t reflect.Type, quoted bool) instruction {
	var ins instruction

	switch t.Kind() {
	case reflect.Bool:
		ins = encodeBool
	case reflect.String:
		return newStringInstr(quoted)
	case reflect.Int:
		ins = encodeInt
	case reflect.Int8:
		ins = encodeInt8
	case reflect.Int16:
		ins = encodeInt16
	case reflect.Int32:
		ins = encodeInt32
	case reflect.Int64:
		ins = encodeInt64
	case reflect.Uint:
		ins = encodeUint
	case reflect.Uint8:
		ins = encodeUint8
	case reflect.Uint16:
		ins = encodeUint16
	case reflect.Uint32:
		ins = encodeUint32
	case reflect.Uint64:
		ins = encodeUint64
	case reflect.Uintptr:
		ins = encodeUintptr
	case reflect.Float32:
		ins = encodeFloat32
	case reflect.Float64:
		ins = encodeFloat64
	default:
		return nil
	}
	if quoted {
		return wrapQuotedInstr(ins)
	}
	return ins
}

func newStringInstr(quoted bool) instruction {
	if quoted {
		return encodeQuotedString
	}
	return encodeString
}

func newUnsupportedTypeInstr(t reflect.Type) instruction {
	return func(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
		return dst, &UnsupportedTypeError{t}
	}
}

func newPtrInstr(t reflect.Type, quoted bool) instruction {
	e := t.Elem()
	i := newInstruction(e, true, quoted)
	return func(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
		return encodePointer(p, dst, opts, i)
	}
}

func newAppendMarshalerCtxInstr(t reflect.Type, hasPtr bool) instruction {
	return func(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
		return encodeMarshaler(p, dst, opts, t, hasPtr, encodeAppendMarshalerCtx)
	}
}

func newAppendMarshalerInstr(t reflect.Type, hasPtr bool) instruction {
	return func(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
		return encodeMarshaler(p, dst, opts, t, hasPtr, encodeAppendMarshaler)
	}
}

func newJSONMarshalerInstr(t reflect.Type, hasPtr bool) instruction {
	return func(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
		return encodeMarshaler(p, dst, opts, t, hasPtr, encodeJSONMarshaler)
	}
}

func newTextMarshalerInstr(t reflect.Type, hasPtr bool) instruction {
	return func(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
		return encodeMarshaler(p, dst, opts, t, hasPtr, encodeTextMarshaler)
	}
}

func newStructInstr(t reflect.Type, canAddr bool) instruction {
	id := fmt.Sprintf("%p-%t", typeID(t), canAddr)

	if instr, ok := structInstrCache.Load(id); ok {
		return instr.(instruction)
	}
	// To deal with recursive types, populate the
	// instructions cache with an indirect func
	// before we build it. This type waits on the
	// real instruction (ins) to be ready and then
	// calls it. This indirect function is only
	// used for recursive types.
	var (
		wg  sync.WaitGroup
		ins instruction
	)
	wg.Add(1)
	i, loaded := structInstrCache.LoadOrStore(id,
		instruction(func(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
			wg.Wait() // few ns/op overhead
			return ins(p, dst, opts)
		}),
	)
	if loaded {
		return i.(instruction)
	}
	// Generate the real instruction and replace
	// the indirect func with it.
	ins = newStructFieldsInstr(t, canAddr)
	wg.Done()
	structInstrCache.Store(id, ins)

	return ins
}

func newStructFieldsInstr(t reflect.Type, canAddr bool) instruction {
	if t.NumField() == 0 {
		// Fast path for empty struct.
		return func(_ unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
			return append(dst, "{}"...), nil
		}
	}
	var (
		flds = cachedFields(t)
		dupl = append(flds[:0:0], flds...) // clone
	)
	for i := range dupl {
		f := &dupl[i]
		ftyp := typeByIndex(t, f.index)
		etyp := ftyp

		if etyp.Kind() == reflect.Ptr {
			etyp = etyp.Elem()
		}
		if !isNilable(ftyp) {
			// Disable the omitnil option, to
			// eliminate a check at runtime.
			f.omitNil = false
		}
		// Generate instruction and empty func of the field.
		// Only strings, floats, integers, and booleans
		// types can be quoted.
		f.instr = newInstruction(ftyp, canAddr, f.quoted && isBasicType(etyp))
		if f.omitEmpty {
			f.empty = cachedEmptyFuncOf(ftyp)
		}
	}
	return func(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
		return encodeStruct(p, dst, opts, dupl)
	}
}

func newArrayInstr(t reflect.Type, canAddr bool) instruction {
	var (
		etyp = t.Elem()
		size = etyp.Size()
		isba = false
	)
	// Array elements are addressable if the
	// array itself is addressable.
	ins := newInstruction(etyp, canAddr, false)

	// Byte arrays does not encode as a string
	// by default, this behavior is defined by
	// the encoder's options during marshaling.
	if etyp.Kind() == reflect.Uint8 {
		pe := reflect.PtrTo(etyp)
		if !pe.Implements(jsonMarshalerType) && !pe.Implements(textMarshalerType) {
			isba = true
		}
	}
	return func(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
		return encodeArray(p, dst, opts, ins, size, t.Len(), isba)
	}
}

func newSliceInstr(t reflect.Type) instruction {
	etyp := t.Elem()

	if etyp.Kind() == reflect.Uint8 {
		pe := reflect.PtrTo(etyp)
		if !pe.Implements(jsonMarshalerType) && !pe.Implements(textMarshalerType) {
			return encodeByteSlice
		}
	}
	// Slice elements are always addressable.
	// see https://golang.org/pkg/reflect/#Value.CanAddr
	// for reference.
	var (
		ins  = newInstruction(etyp, true, false)
		size = etyp.Size()
	)
	return func(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
		return encodeSlice(p, dst, opts, ins, size)
	}
}

func newMapInstr(t reflect.Type) instruction {
	var (
		ki instruction
		vi instruction
	)
	kt := t.Key()
	et := t.Elem()

	if !isString(kt) && !isInteger(kt) && !kt.Implements(textMarshalerType) {
		return newUnsupportedTypeInstr(t)
	}
	// The standard library has a strict precedence order
	// for map key types, defined by the documentation of
	// the json.Marshal function. That's why we bypass the
	// newTypeInstr function if key type is string.
	if isString(kt) {
		ki = encodeString
	} else {
		ki = newInstruction(kt, false, false)
	}
	// Wrap the key instruction for types that
	// do not encode with quotes by default.
	if !isString(kt) && !kt.Implements(textMarshalerType) {
		ki = wrapQuotedInstr(ki)
	}
	// See issue golang.org/issue/33675 for reference.
	if kt.Implements(textMarshalerType) && kt.Kind() == reflect.Ptr {
		ki = wrapTextMarshalerNilCheck(ki)
	}
	vi = newInstruction(et, false, false)

	return func(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
		return encodeMap(p, dst, opts, t, ki, vi)
	}
}

func wrapInlineInstr(ins instruction) instruction {
	return func(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
		return ins(noescape(unsafe.Pointer(&p)), dst, opts)
	}
}

func wrapQuotedInstr(ins instruction) instruction {
	return func(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
		dst = append(dst, '"')
		var err error
		dst, err = ins(p, dst, opts)
		if err == nil {
			dst = append(dst, '"')
		}
		return dst, err
	}
}

func wrapTextMarshalerNilCheck(ins instruction) instruction {
	return func(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
		if *(*unsafe.Pointer)(p) == nil {
			return append(dst, `""`...), nil
		}
		return ins(p, dst, opts)
	}
}
