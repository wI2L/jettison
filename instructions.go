package jettison

import (
	"bytes"
	"encoding"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/modern-go/reflect2"
)

const hex = "0123456789abcdef"

var (
	timeTimeType      = reflect.TypeOf(time.Time{})
	timeDurationType  = reflect.TypeOf(time.Duration(0))
	jsonNumberType    = reflect.TypeOf(json.Number(""))
	marshalerType     = reflect.TypeOf((*Marshaler)(nil)).Elem()
	marshalerCtxType  = reflect.TypeOf((*MarshalerCtx)(nil)).Elem()
	textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
)

var instrCache sync.Map // map[reflect.Type]Instruction

var bpool = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

// cachedTypeInstr is the same as newTypeInstr, but
// uses a cache to avoid duplicate instructions.
func cachedTypeInstr(t reflect.Type, slow bool) (Instruction, error) {
	if instr, ok := instrCache.Load(t); ok {
		return instr.(Instruction), nil
	}
	// To deal with recursive types, populate the
	// instructions cache with an indirect func
	// before we build it. This type waits on the
	// real instruction (ins) to be ready and then
	// calls it. This indirect function is only
	// used for recursive types.
	var (
		wg  sync.WaitGroup
		err error
		ins Instruction
	)
	if slow {
		// Only used during testing to stack calls
		// before LoadAndStore.
		time.Sleep(25 * time.Millisecond)
	}
	wg.Add(1)
	i, ok := instrCache.LoadOrStore(t,
		Instruction(func(p unsafe.Pointer, w Writer, es *encodeState) error {
			wg.Wait()
			return ins(p, w, es)
		}),
	)
	if ok {
		return i.(Instruction), nil
	}
	// Generate the real instruction and replace
	// the indirect func with it.
	ins, err = newTypeInstr(t, false)
	if err != nil {
		// Remove the indirect function inserted
		// previously if the type is unsupported
		// to prevent the return of a nil error
		// on future calls for the same type.
		instrCache.Delete(t)
		return nil, err
	}
	wg.Done()
	instrCache.Store(t, ins)

	return ins, nil
}

// newTypeInstr returns the instruction to encode t.
func newTypeInstr(t reflect.Type, skipSpecialAndMarshalers bool) (Instruction, error) {
	if skipSpecialAndMarshalers {
		goto skip
	}
	// Special types must be checked first, because a Duration
	// is an int64, json.Number is a string, and both would be
	// interpreted as a basic type.
	// Also, time.Time implements the TextMarshaler interface,
	// but we want to use a special instruction instead.
	if isSpecialType(t) {
		return specialTypeInstr(t), nil
	}
	if instr := marshalerInstr(t); instr != nil {
		return instr, nil
	}
skip:
	if isBasicType(t) {
		return basicTypeInstr(t.Kind())
	}
	var (
		err error
		ins Instruction
	)
	switch t.Kind() {
	case reflect.Array:
		ins, err = newArrayInstr(t)
	case reflect.Slice:
		ins, err = newSliceInstr(t)
	case reflect.Struct:
		ins, err = newStructInstr(t)
	case reflect.Map:
		ins, err = newMapInstr(t)
	case reflect.Interface:
		return interfaceInstr, nil
	case reflect.Ptr:
		et := t.Elem()
		einstr, err := cachedTypeInstr(et, false)
		if err != nil {
			return nil, err
		}
		return func(p unsafe.Pointer, w Writer, es *encodeState) error {
			if p != nil {
				p = *(*unsafe.Pointer)(p)
			}
			if p == nil {
				_, err := w.WriteString("null")
				return err
			}
			return einstr(p, w, es)
		}, nil
	default:
		return nil, &UnsupportedTypeError{Typ: t}
	}
	if err != nil {
		return nil, err
	}
	return ins, nil
}

func marshalerInstr(t reflect.Type) Instruction {
	if t.Implements(marshalerCtxType) {
		return newMarshalerCtxInstr(t)
	}
	if t.Kind() != reflect.Ptr {
		if reflect.PtrTo(t).Implements(marshalerCtxType) {
			return newAddrMarshalerCtxInstr(t)
		}
	}
	if t.Implements(marshalerType) {
		return newMarshalerInstr(t)
	}
	if t.Kind() != reflect.Ptr {
		if reflect.PtrTo(t).Implements(marshalerType) {
			return newAddrMarshalerInstr(t)
		}
	}
	if t.Implements(jsonMarshalerType) {
		return newJSONMarshalerInstr(t)
	}
	if t.Kind() != reflect.Ptr {
		if reflect.PtrTo(t).Implements(jsonMarshalerType) {
			return newAddrJSONMarshalerInstr(t)
		}
	}
	if t.Implements(textMarshalerType) {
		return newTextMarshalerInstr(t)
	}
	if t.Kind() != reflect.Ptr {
		if reflect.PtrTo(t).Implements(textMarshalerType) {
			return newAddrTextMarshalerInstr(t)
		}
	}
	return nil
}

// newMarshalerInstr returns an instruction to
// encode a type which have a pointer receiver, by
// using its WriteJSON method.
func newMarshalerInstr(t reflect.Type) Instruction {
	return func(p unsafe.Pointer, w Writer, es *encodeState) error {
		v := reflect.NewAt(t, p).Elem()

		m, ok := v.Interface().(Marshaler)
		if !ok {
			_, err := w.WriteString("null")
			return err
		}
		if err := m.WriteJSON(w); err != nil {
			return &MarshalerError{err, t, marshalerFuncJettison}
		}
		return nil
	}
}

// newAddrMarshalerInstr returns an instruction to
// encode a type which have a non-pointer receiver, by
// using its WriteJSON method.
func newAddrMarshalerInstr(t reflect.Type) Instruction {
	// Fallback instruction for non-addressable values.
	finstr, _ := newTypeInstr(t, true)

	return func(p unsafe.Pointer, w Writer, es *encodeState) error {
		if !es.addressable && !es.ptrInput {
			return finstr(p, w, es)
		}
		v := reflect.NewAt(t, p)
		m := v.Interface().(Marshaler)
		if err := m.WriteJSON(w); err != nil {
			return &MarshalerError{err, reflect.PtrTo(t), marshalerFuncJettison}
		}
		return nil
	}
}

// newMarshalerCtxInstr returns an instruction to
// encode a type which have a pointer receiver, by
// using its WriteJSONContext method.
func newMarshalerCtxInstr(t reflect.Type) Instruction {
	return func(p unsafe.Pointer, w Writer, es *encodeState) error {
		v := reflect.NewAt(t, p).Elem()

		m, ok := v.Interface().(MarshalerCtx)
		if !ok {
			_, err := w.WriteString("null")
			return err
		}
		if err := m.WriteJSONContext(es.opts.ctx, w); err != nil {
			return &MarshalerError{err, t, marshalerFuncJettisonCtx}
		}
		return nil
	}
}

// newAddrMarshalerCtxInstr returns an instruction to
// encode a type which have a non-pointer receiver, by
// using its WriteJSONContext method.
func newAddrMarshalerCtxInstr(t reflect.Type) Instruction {
	// Fallback instruction for non-addressable values.
	finstr, _ := newTypeInstr(t, true)

	return func(p unsafe.Pointer, w Writer, es *encodeState) error {
		if !es.addressable && !es.ptrInput {
			return finstr(p, w, es)
		}
		v := reflect.NewAt(t, p)
		m := v.Interface().(MarshalerCtx)
		if err := m.WriteJSONContext(es.opts.ctx, w); err != nil {
			return &MarshalerError{err, reflect.PtrTo(t), marshalerFuncJettisonCtx}
		}
		return nil
	}
}

// newJSONMarshalerInstr returns an instruction to
// encode a type which have a pointer receiver, by
// using its MarshalJSON method.
func newJSONMarshalerInstr(t reflect.Type) Instruction {
	return func(p unsafe.Pointer, w Writer, es *encodeState) error {
		v := reflect.NewAt(t, p).Elem()

		m, ok := v.Interface().(json.Marshaler)
		if !ok {
			_, err := w.WriteString("null")
			return err
		}
		b, err := m.MarshalJSON()
		if err != nil {
			return &MarshalerError{err, t, marshalerFuncJSON}
		}
		_, err = w.Write(b)
		return err
	}
}

// newAddrJSONMarshalerInstr returns an instruction to
// encode a type which have a non-pointer receiver, by
// using its MarshalJSON method.
func newAddrJSONMarshalerInstr(t reflect.Type) Instruction {
	// Fallback instruction for non-addressable values.
	finstr, _ := newTypeInstr(t, true)

	return func(p unsafe.Pointer, w Writer, es *encodeState) error {
		if !es.addressable && !es.ptrInput {
			return finstr(p, w, es)
		}
		v := reflect.NewAt(t, p)
		m := v.Interface().(json.Marshaler)
		b, err := m.MarshalJSON()
		if err != nil {
			return &MarshalerError{err, reflect.PtrTo(t), marshalerFuncJSON}
		}
		_, err = w.Write(b)
		return err
	}
}

// newTextMarshalerInstr returns an instruction to
// encode a type which have a pointer receiver, by
// using its MarshalText method.
// The instruction quotes the result returned by
// the the method.
func newTextMarshalerInstr(t reflect.Type) Instruction {
	return func(p unsafe.Pointer, w Writer, es *encodeState) error {
		v := reflect.NewAt(t, p).Elem()

		m := v.Interface().(encoding.TextMarshaler)
		b, err := m.MarshalText()
		if err != nil {
			return &MarshalerError{err, t, marshalerFuncText}
		}
		if err := w.WriteByte('"'); err != nil {
			return err
		}
		if err := writeEscapedBytes(b, w, es); err != nil {
			return err
		}
		return w.WriteByte('"')
	}
}

// newAddrTextMarshalerInstr returns an instruction
// to encode a type which have a non-pointer receiver,
// by using its MarshalText method.
// The instruction quotes the result returned by the
// method.
func newAddrTextMarshalerInstr(t reflect.Type) Instruction {
	// Fallback instruction for non-addressable values.
	finstr, _ := newTypeInstr(t, true)

	return func(p unsafe.Pointer, w Writer, es *encodeState) error {
		if !es.addressable && !es.ptrInput {
			return finstr(p, w, es)
		}
		v := reflect.NewAt(t, p)
		m := v.Interface().(encoding.TextMarshaler)
		b, err := m.MarshalText()
		if err != nil {
			return &MarshalerError{err, reflect.PtrTo(t), marshalerFuncText}
		}
		if err := w.WriteByte('"'); err != nil {
			return err
		}
		if err := writeEscapedBytes(b, w, es); err != nil {
			return err
		}
		return w.WriteByte('"')
	}
}

func specialTypeInstr(t reflect.Type) Instruction {
	// Keep in sync with isSpecialType.
	switch t {
	case timeTimeType:
		return timeInstr
	case timeDurationType:
		return durationInstr
	case jsonNumberType:
		return numberInstr
	default:
		return nil
	}
}

// basicTypeInstr returns the instruction associated
// with the basic type that has the given kind.
func basicTypeInstr(k reflect.Kind) (Instruction, error) {
	// Keep in sync with isBasicType.
	switch k {
	case reflect.String:
		return stringInstr, nil
	case reflect.Bool:
		return boolInstr, nil
	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64:
		return newIntInstr(k)
	case reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr:
		return newUintInstr(k)
	case reflect.Float32,
		reflect.Float64:
		return newFloatInstr(k)
	default:
		return nil, fmt.Errorf("unknown basic kind: %s", k)
	}
}

// boolInstr writes a boolean string to w.
func boolInstr(p unsafe.Pointer, w Writer, _ *encodeState) error {
	v := *(*bool)(p)
	var err error
	if v {
		_, err = w.WriteString("true")
	} else {
		_, err = w.WriteString("false")
	}
	return err
}

// stringInstr writes a quoted string to w.
func stringInstr(p unsafe.Pointer, w Writer, es *encodeState) error {
	if err := w.WriteByte('"'); err != nil {
		return err
	}
	if err := writeEscapedBytes(spb(p), w, es); err != nil {
		return err
	}
	return w.WriteByte('"')
}

func quotedStringInstr(p unsafe.Pointer, w Writer, es *encodeState) error {
	if _, err := w.WriteString(`\"`); err != nil {
		return err
	}
	if err := writeEscapedBytes(spb(p), w, es); err != nil {
		return err
	}
	_, err := w.WriteString(`\"`)
	return err
}

func spb(p unsafe.Pointer) []byte {
	shdr := (*reflect.StringHeader)(p)
	return *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
		Data: shdr.Data,
		Len:  shdr.Len,
		Cap:  shdr.Len,
	}))
}

func writeEscapedBytes(b []byte, w Writer, es *encodeState) error {
	if len(b) == 0 {
		return nil
	}
	at := 0
	if es.opts.noStringEscape {
		goto end
	}
	for i := 0; i < len(b); {
		if c := b[i]; c < utf8.RuneSelf {
			if isSafeJSONChar(c) {
				if !es.opts.noHTMLEscape && isHTMLChar(c) {
					goto escape
				}
				// If the current character doesn't need
				// to be escaped, accumulate the bytes to
				// save some write operations.
				i++
				continue
			}
		escape:
			// Write accumulated single-byte characters.
			if at < i {
				if _, err := w.Write(b[at:i]); err != nil {
					return err
				}
			}
			// The encoding/json package implements only
			// a few of the special two-character escape
			// sequence described in the RFC 8259, Section 7.
			// \b and \f were ignored on purpose, see
			// https://codereview.appspot.com/4678046.
			buf := es.scratch[:0]
			switch c {
			case '"', '\\':
				buf = append(buf, '\\', c)
			case '\n': // 0xA, line feed
				buf = append(buf, '\\', 'n')
			case '\r': // 0xD, carriage return
				buf = append(buf, '\\', 'r')
			case '\t': // 0x9, horizontal tab
				buf = append(buf, '\\', 't')
			default:
				buf = append(buf, `\u00`...)
				buf = append(buf, hex[c>>4])
				buf = append(buf, hex[c&0xF])
			}
			if _, err := w.Write(buf); err != nil {
				return err
			}
			i++
			at = i
			continue
		}
		if !es.opts.noUTF8Coercion {
			c, size := utf8.DecodeRune(b[i:])

			// Coerce to valid UTF-8, by replacing invalid
			// bytes with the Unicode replacement rune.
			if c == utf8.RuneError && size == 1 {
				if at < i {
					if _, err := w.Write(b[at:i]); err != nil {
						return err
					}
				}
				if _, err := w.WriteString(`\ufffd`); err != nil {
					return err
				}
				i += size
				at = i
				continue
			}
			// U+2028 is LINE SEPARATOR.
			// U+2029 is PARAGRAPH SEPARATOR.
			// They are both technically valid characters in
			// JSON strings, but don't work in JSONP, which has
			// to be evaluated as JavaScript, and can lead to
			// security holes there. It is valid JSON to escape
			// them, so we do so unconditionally.
			// See http://timelessrepo.com/json-isnt-a-javascript-subset.
			if c == '\u2028' || c == '\u2029' {
				if at < i {
					if _, err := w.Write(b[at:i]); err != nil {
						return err
					}
				}
				buf := es.scratch[:0]
				buf = append(buf, `\u202`...)
				buf = append(buf, hex[c&0xF])
				if _, err := w.Write(buf); err != nil {
					return err
				}
				i += size
				at = i
				continue
			}
			i += size
			continue
		}
		i++
	}
end:
	// Write remaining bytes.
	if at < len(b) {
		if _, err := w.Write(b[at:]); err != nil {
			return err
		}
	}
	return nil
}

// isSafeJSONChar returns whether c can be
// used in a JSON string without escaping.
func isSafeJSONChar(c byte) bool {
	if c < 0x20 || c == '"' || c == '\\' {
		return false
	}
	return true
}

func isHTMLChar(c byte) bool {
	if c == '&' || c == '<' || c == '>' {
		return true
	}
	return false
}

type (
	intCastFn  func(p unsafe.Pointer) int64
	uintCastFn func(p unsafe.Pointer) uint64
)

func newIntInstr(k reflect.Kind) (Instruction, error) {
	var cast intCastFn
	switch k {
	case reflect.Int:
		cast = func(p unsafe.Pointer) int64 { return int64(*(*int)(p)) }
	case reflect.Int8:
		cast = func(p unsafe.Pointer) int64 { return int64(*(*int8)(p)) }
	case reflect.Int16:
		cast = func(p unsafe.Pointer) int64 { return int64(*(*int16)(p)) }
	case reflect.Int32:
		cast = func(p unsafe.Pointer) int64 { return int64(*(*int32)(p)) }
	case reflect.Int64:
		cast = func(p unsafe.Pointer) int64 { return *(*int64)(p) }
	default:
		return nil, fmt.Errorf("invalid kind: %s", k)
	}
	return func(p unsafe.Pointer, w Writer, es *encodeState) error {
		dst := es.scratch[:1]
		// Save one byte at the beginning of the
		// buffer for an eventual double-quote
		// char that may be added before writing
		// to the stream.
		dst = strconv.AppendInt(dst, cast(p), es.opts.integerBase)
		dst = maybeQuoteBytes(dst, es.opts.integerBase > 10)
		_, err := w.Write(dst)
		return err
	}, nil
}

func newUintInstr(k reflect.Kind) (Instruction, error) {
	var cast uintCastFn
	switch k {
	case reflect.Uint:
		cast = func(p unsafe.Pointer) uint64 { return uint64(*(*uint)(p)) }
	case reflect.Uint8:
		cast = func(p unsafe.Pointer) uint64 { return uint64(*(*uint8)(p)) }
	case reflect.Uint16:
		cast = func(p unsafe.Pointer) uint64 { return uint64(*(*uint16)(p)) }
	case reflect.Uint32:
		cast = func(p unsafe.Pointer) uint64 { return uint64(*(*uint32)(p)) }
	case reflect.Uint64:
		cast = func(p unsafe.Pointer) uint64 { return *(*uint64)(p) }
	case reflect.Uintptr:
		cast = func(p unsafe.Pointer) uint64 { return uint64(*(*uintptr)(p)) }
	default:
		return nil, fmt.Errorf("invalid kind: %s", k)
	}
	return func(p unsafe.Pointer, w Writer, es *encodeState) error {
		dst := es.scratch[:1]
		// Save one byte at the beginning of the
		// buffer for an eventual double-quote
		// char that may be added before writing
		// to the stream.
		dst = strconv.AppendUint(dst, cast(p), es.opts.integerBase)
		dst = maybeQuoteBytes(dst, es.opts.integerBase > 10)
		_, err := w.Write(dst)
		return err
	}, nil
}

func maybeQuoteBytes(b []byte, quote bool) []byte {
	if quote {
		b[0] = '"'
		b = append(b, '"')
	} else {
		b = b[1:]
	}
	return b
}

type floatCastFn func(p unsafe.Pointer) float64

func newFloatInstr(k reflect.Kind) (Instruction, error) {
	var (
		cast floatCastFn
		bs   int
	)
	switch k {
	case reflect.Float32:
		cast = func(p unsafe.Pointer) float64 { return float64(*(*float32)(p)) }
		bs = 32
	case reflect.Float64:
		cast = func(p unsafe.Pointer) float64 { return *(*float64)(p) }
		bs = 64
	default:
		return nil, fmt.Errorf("invalid bit size: %d", bs)
	}
	return func(p unsafe.Pointer, w Writer, es *encodeState) error {
		return writeFloat(cast(p), bs, w, es)
	}, nil
}

//nolint:interfacer
func writeFloat(f float64, bitSize int, w Writer, es *encodeState) error {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return &UnsupportedValueError{strconv.FormatFloat(f, 'g', -1, bitSize)}
	}
	// Convert as it was an ES6 number to string conversion.
	// This matches most other JSON generators. The following
	// code is taken from the floatEncoder implementation of
	// the encoding/json package of the Go std library.
	abs := math.Abs(f)
	format := byte('f')
	if abs != 0 {
		if bitSize == 64 && (abs < 1e-6 || abs >= 1e21) ||
			bitSize == 32 && (float32(abs) < 1e-6 || float32(abs) >= 1e21) {
			format = 'e'
		}
	}
	b := strconv.AppendFloat(es.scratch[:0], f, format, -1, bitSize)
	if format == 'e' {
		n := len(b)
		if n >= 4 && b[n-4] == 'e' && b[n-3] == '-' && b[n-2] == '0' {
			b[n-2] = b[n-1]
			b = b[:n-1]
		}
	}
	_, err := w.Write(b)
	return err
}

// writeByteArrayAsString writes a byte array to w as a JSON string.
func writeByteArrayAsString(p unsafe.Pointer, w Writer, es *encodeState, length int) error {
	if err := w.WriteByte('"'); err != nil {
		return err
	}
	// For byte type, size is guaranteed to be 1.
	// https://golang.org/ref/spec#Size_and_alignment_guarantees
	b := *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
		Data: uintptr(p),
		Len:  length,
		Cap:  length,
	}))
	if err := writeEscapedBytes(b, w, es); err != nil {
		return err
	}
	return w.WriteByte('"')
}

// byteSliceInstr writes a byte slice to w as a
// JSON string. If es.byteSliceAsString is true,
// the bytes are written directly to the stream,
// otherwise, it writes a base64 representation.
func byteSliceInstr(p unsafe.Pointer, w Writer, es *encodeState) error {
	shdr := (*reflect.SliceHeader)(p)
	if shdr.Data == 0 {
		_, err := w.WriteString("null")
		return err
	}
	b := *(*[]byte)(p)

	if err := w.WriteByte('"'); err != nil {
		return err
	}
	var err error
	if es.opts.noBase64Slice {
		err = writeEscapedBytes(b, w, es)
	} else {
		err = writeBase64ByteSlice(b, w, es)
	}
	if err != nil {
		return err
	}
	return w.WriteByte('"')
}

//nolint:interfacer
func writeBase64ByteSlice(b []byte, w Writer, es *encodeState) error {
	l := base64.StdEncoding.EncodedLen(len(b))
	if l <= 1024 {
		var dst []byte
		// If the length of bytes to encode fits in
		// st.scratch, avoid an extra allocation and
		// use the cheaper Encode method.
		if l <= len(es.scratch) {
			dst = es.scratch[:l]
		} else {
			// Allocate a new buffer.
			dst = make([]byte, l)
		}
		base64.StdEncoding.Encode(dst, b)
		_, err := w.Write(dst)
		return err
	}
	// Use a new encoder that writes
	// directly to the output stream.
	enc := base64.NewEncoder(base64.StdEncoding, w)
	if _, err := enc.Write(b); err != nil {
		return err
	}
	return enc.Close()
}

// numberInstr writes a json.Number value to w.
func numberInstr(p unsafe.Pointer, w Writer, _ *encodeState) error {
	// Cast pointer to string directly to
	// avoid a useless conversion.
	n := *(*string)(p)
	if !isValidNumber(n) {
		return fmt.Errorf("invalid number literal: %q", n)
	}
	_, err := w.WriteString(n)
	return err
}

// isValidNumber returns whether s is a valid JSON number literal.
// Taken from encoding/json.
func isValidNumber(s string) bool {
	// This function implements the JSON numbers grammar.
	// See https://tools.ietf.org/html/rfc7159#section-6
	// and https://www.json.org/img/number.png.
	if s == "" {
		return false
	}
	// Optional minus sign.
	if s[0] == '-' {
		s = s[1:]
		if s == "" {
			return false
		}
	}
	// Digits.
	switch {
	default:
		return false
	case s[0] == '0':
		s = s[1:]
	case '1' <= s[0] && s[0] <= '9':
		s = s[1:]
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	}
	// Dot followed by 1 or more digits.
	if len(s) >= 2 && s[0] == '.' && '0' <= s[1] && s[1] <= '9' {
		s = s[2:]
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	}
	// e or E followed by an optional - or + and
	// 1 or more digits.
	if len(s) >= 2 && (s[0] == 'e' || s[0] == 'E') {
		s = s[1:]
		if s[0] == '+' || s[0] == '-' {
			s = s[1:]
			if s == "" {
				return false
			}
		}
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	}
	// Make sure we are at the end.
	return s == ""
}

// timeInstr writes a time.Time value to w.
// It is formatted as a string using the layout defined
// by es.timeLayout, or as an integer representing a Unix
// timestamp if es.useTimestamps is true.
func timeInstr(p unsafe.Pointer, w Writer, es *encodeState) error {
	t := *(*time.Time)(p)
	if y := t.Year(); y < 0 || y >= 10000 {
		// see issue golang.org/issue/4556#c15
		return errors.New("time: year outside of range [0,9999]")
	}
	if es.opts.useTimestamps {
		ts := t.Unix()
		b := strconv.AppendInt(es.scratch[:0], ts, 10)
		_, err := w.Write(b)
		return err
	}
	b := append(es.scratch[:0], '"')
	b = t.AppendFormat(b, es.opts.timeLayout)
	b = append(b, '"')
	_, err := w.Write(b)
	return err
}

// durationInstr writes a time.Duration to w.
func durationInstr(p unsafe.Pointer, w Writer, es *encodeState) error {
	d := *(*time.Duration)(p)

	if es.opts.durationFmt == DurationString {
		// The largest representation of a duration
		// can take up to 25 bytes, plus 2 bytes
		// for quotes.
		s := 25 + 2
		b := es.scratch[:s]
		b[s-1] = '"'
		// appendDuration writes in the buffer
		// from right to left.
		rb := appendDuration(b[:s-1], d)
		b = b[s-1-len(rb)-1:]
		b[0] = '"'
		_, err := w.Write(b)
		return err
	}
	switch es.opts.durationFmt {
	case DurationMinutes:
		return writeFloat(d.Minutes(), 64, w, es)
	case DurationSeconds:
		return writeFloat(d.Seconds(), 64, w, es)
	case DurationMicroseconds:
		return writeIntDuration(int64(d)/1e3, w, es)
	case DurationMilliseconds:
		return writeIntDuration(int64(d)/1e6, w, es)
	case DurationNanoseconds:
		return writeIntDuration(d.Nanoseconds(), w, es)
	}
	return fmt.Errorf("unknown duration format: %v", es.opts.durationFmt)
}

//nolint:interfacer
func writeIntDuration(d int64, w Writer, es *encodeState) error {
	b := strconv.AppendInt(es.scratch[:0], d, 10)
	_, err := w.Write(b)
	return err
}

// interfaceInstr writes an interface value to w.
func interfaceInstr(p unsafe.Pointer, w Writer, es *encodeState) error {
	i := *(*interface{})(p)
	if i == nil {
		_, err := w.WriteString("null")
		return err
	}
	v := reflect.ValueOf(i)

	if !v.IsValid() {
		return fmt.Errorf("iface: invalid value: %v", v)
	}
	vt := v.Type()
	isPtr := vt.Kind() == reflect.Ptr
	if isPtr {
		if v.IsNil() {
			_, err := w.WriteString("null")
			return err
		}
		vt = vt.Elem()
	}
	instr, err := cachedTypeInstr(vt, false)
	if err != nil {
		return err
	}
	if !isPtr {
		vp := reflect.New(v.Type())
		vp.Elem().Set(v)
		v = vp
	}
	return instr(unsafe.Pointer(v.Pointer()), w, es)
}

// newArrayInstr returns a new instruction to encode
// a Go array. It returns an error if the given type
// is unexpected, or if the array value type is not
// supported.
func newArrayInstr(t reflect.Type) (Instruction, error) {
	et := t.Elem()

	// Byte arrays does not encode as a string
	// by default, the behavior is defined by
	// the encoder's options.
	isByteArray := false
	if et.Kind() == reflect.Uint8 {
		pe := reflect.PtrTo(et)
		if !pe.Implements(jsonMarshalerType) && !pe.Implements(textMarshalerType) {
			isByteArray = true
		}
	}
	isPtr := et.Kind() == reflect.Ptr
	if isPtr {
		et = et.Elem()
	}
	eins, err := cachedTypeInstr(et, false)
	if err != nil {
		return nil, &UnsupportedTypeError{Typ: t}
	}
	var esz uintptr
	if !isPtr {
		esz = et.Size()
	} else {
		esz = t.Elem().Size()
	}
	length := t.Len()

	return func(v unsafe.Pointer, w Writer, es *encodeState) error {
		if es.opts.byteArrayAsString && isByteArray {
			return writeByteArrayAsString(v, w, es, length)
		}
		if err := w.WriteByte('['); err != nil {
			return err
		}
		for i := 0; i < length; i++ {
			if i != 0 {
				if err := w.WriteByte(','); err != nil {
					return err
				}
			}
			p := unsafe.Pointer(uintptr(v) + uintptr(i)*esz)
			if isPtr {
				p = *(*unsafe.Pointer)(p)
				if p == nil {
					if _, err := w.WriteString("null"); err != nil {
						return err
					}
					continue
				}
			}
			// Encode the nth element of the array.
			if err := eins(p, w, es); err != nil {
				return err
			}
		}
		return w.WriteByte(']')
	}, nil
}

// newSliceInstr returns a new instruction to encode
// a Go slice. It returns an error if the given type
// is unexpected, or if the slice value type is not
// supported.
func newSliceInstr(t reflect.Type) (Instruction, error) {
	et := t.Elem()

	// Byte slices are encoded as a string to
	// comply with the std library behavior.
	if et.Kind() == reflect.Uint8 {
		pe := reflect.PtrTo(et)
		if !pe.Implements(jsonMarshalerType) && !pe.Implements(textMarshalerType) {
			return byteSliceInstr, nil
		}
	}
	isPtr := et.Kind() == reflect.Ptr
	if isPtr {
		et = et.Elem()
	}
	eins, err := cachedTypeInstr(et, false)
	if err != nil {
		return nil, &UnsupportedTypeError{Typ: t}
	}
	var esz uintptr
	if !isPtr {
		esz = et.Size()
	} else {
		esz = t.Elem().Size()
	}
	return func(v unsafe.Pointer, w Writer, es *encodeState) error {
		shdr := (*reflect.SliceHeader)(v)
		if shdr.Data == 0 {
			if es.opts.nilSliceEmpty {
				_, err := w.WriteString("[]")
				return err
			}
			// A nil slice cannot be treated like a basic
			// type because the pointer will always point
			// to a non-nil slice header which contains a
			// Data field with an empty memory address.
			_, err := w.WriteString("null")
			return err
		}
		if err := w.WriteByte('['); err != nil {
			return err
		}
		for i := 0; i < shdr.Len; i++ {
			if i != 0 {
				if err := w.WriteByte(','); err != nil {
					return err
				}
			}
			p := unsafe.Pointer(uintptr(unsafe.Pointer(shdr.Data)) + uintptr(i)*esz)
			if isPtr {
				p = *(*unsafe.Pointer)(p)
				if p == nil {
					if _, err := w.WriteString("null"); err != nil {
						return err
					}
					continue
				}
			}
			// Encode the nth element of the slice.
			if err := eins(p, w, es); err != nil {
				return err
			}
		}
		return w.WriteByte(']')
	}, nil
}

type kv struct {
	key []byte
	val []byte
}

type keyvalue []kv

func (kv keyvalue) Len() int           { return len(kv) }
func (kv keyvalue) Swap(i, j int)      { kv[i], kv[j] = kv[j], kv[i] }
func (kv keyvalue) Less(i, j int) bool { return bytes.Compare(kv[i].key, kv[j].key) < 0 }

// newMapInstr returns the instruction to
// encode a Go map. It returns an error if
// the given type is unexpected.
func newMapInstr(t reflect.Type) (Instruction, error) {
	kt := t.Key()

	if !isString(kt) && !isInteger(kt) && !kt.Implements(textMarshalerType) {
		return nil, &UnsupportedTypeError{Typ: t}
	}
	vinstr, err := cachedTypeInstr(t.Elem(), false)
	if err != nil {
		return nil, err
	}
	// The standard library has a strict precedence order
	// for map key types, defined in the documentation of
	// the json.Marshal function; that's why we bypass the
	// TextMarshaler instructions if the key is a string.
	bypassMarshaler := false
	if isString(kt) {
		bypassMarshaler = true
	}
	kinstr, err := newTypeInstr(kt, bypassMarshaler)
	if err != nil {
		return nil, err
	}
	// Wrap the key instruction for types that
	// do not encode with quotes by default.
	if !isString(kt) && !kt.Implements(textMarshalerType) {
		kinstr = wrapQuoteInstr(kinstr)
	}
	typ2 := reflect2.Type2(t)
	mtyp := typ2.(*reflect2.UnsafeMapType)

	return func(v unsafe.Pointer, w Writer, es *encodeState) error {
		p := *(*unsafe.Pointer)(v)
		if p == nil {
			if es.opts.nilMapEmpty {
				_, err := w.WriteString("{}")
				return err
			}
			_, err := w.WriteString("null")
			return err
		}
		if err := w.WriteByte('{'); err != nil {
			return err
		}
		it := mtyp.UnsafeIterate(v)
		if !es.opts.unsortedMap {
			if err := encodeSortedMap(it, w, es, kinstr, vinstr); err != nil {
				return err
			}
		} else {
			if err := encodeUnsortedMap(it, w, es, kinstr, vinstr); err != nil {
				return err
			}
		}
		return w.WriteByte('}')
	}, nil
}

func encodeSortedMap(it reflect2.MapIterator, w Writer, es *encodeState,
	ki, vi Instruction,
) error {
	var (
		kvs   keyvalue
		off   int
		kvbuf = bpool.Get().(*bytes.Buffer)
	)
	defer bpool.Put(kvbuf)
	kvbuf.Reset()

	for i := 0; it.HasNext(); i++ {
		key, val := it.UnsafeNext()
		kv := kv{}

		// Encode the key and store the buffer
		// portion to use during sort.
		if err := ki(key, kvbuf, es); err != nil {
			return err
		}
		kv.key = kvbuf.Bytes()[off:kvbuf.Len()]

		// Write separator after key.
		if err := kvbuf.WriteByte(':'); err != nil {
			return err
		}
		// Encode the value and store the buffer
		// portion corresponding to the semicolon
		// delimited key/value pair.
		if err := vi(val, kvbuf, es); err != nil {
			return err
		}
		kv.val = kvbuf.Bytes()[off:kvbuf.Len()]
		off = kvbuf.Len()
		kvs = append(kvs, kv)
	}
	sort.Sort(kvs) // lexicographical sort by keys

	// Write each k/v couple to the output buffer,
	// comma-separated, except for the first.
	for i, kv := range kvs {
		if i != 0 {
			if err := w.WriteByte(','); err != nil {
				return err
			}
		}
		if _, err := w.Write(kv.val); err != nil {
			return err
		}
	}
	return nil
}

func encodeUnsortedMap(it reflect2.MapIterator, w Writer, es *encodeState,
	ki, vi Instruction,
) error {
	for i := 0; it.HasNext(); i++ {
		if i != 0 {
			if err := w.WriteByte(','); err != nil {
				return err
			}
		}
		key, val := it.UnsafeNext()
		if err := ki(key, w, es); err != nil {
			return err
		}
		if err := w.WriteByte(':'); err != nil {
			return err
		}
		if err := vi(val, w, es); err != nil {
			return err
		}
	}
	return nil
}

func isSpecialType(t reflect.Type) bool {
	// Keep in sync with specialTypeInstr.
	return t == timeTimeType || t == timeDurationType || t == jsonNumberType
}

func isBasicType(t reflect.Type) bool {
	// Keep in sync with basicTypeInstr.
	return isInteger(t) || isFloatingPoint(t) || isString(t) || isBoolean(t)
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
