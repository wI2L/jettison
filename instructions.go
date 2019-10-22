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

// defaultTimeLayout is the layout used by default
// to format a time.Time, unless otherwise specified.
// This is compliant with the ECMA specification and
// the JavaScript Date's toJSON method implementation.
const defaultTimeLayout = time.RFC3339Nano

var (
	timeType          = reflect.TypeOf(time.Time{})
	durationType      = reflect.TypeOf(time.Duration(0))
	numberType        = reflect.TypeOf(json.Number(""))
	marshalerType     = reflect.TypeOf((*Marshaler)(nil)).Elem()
	textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
)

var instrCache sync.Map // map[reflect.Type]Instruction

var bpool = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

// cachedTypeInstr is the same as typeInstr, but
// uses a cache to avoid duplicate instructions.
func cachedTypeInstr(t reflect.Type) (Instruction, error) {
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
// It creates a new Encoder instance to encode some
// composite types, such as struct and map.
func newTypeInstr(t reflect.Type, skipSpecialAndMarshalers bool) (Instruction, error) {
	if skipSpecialAndMarshalers {
		goto def
	}
	// Special types must be checked first, because a Duration
	// is an int64, json.Number is a string, and both would be
	// interpreted as a primitive.
	// Also, time.Time implements the TextMarshaler interface,
	// but we want to use a special instruction instead.
	if isSpecialType(t) {
		return specialTypeInstr(t), nil
	}
	if instr := marshalerInstr(t); instr != nil {
		return instr, nil
	}
def:
	if isPrimitiveType(t) {
		return primitiveInstr(t.Kind()), nil
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
		einstr, err := cachedTypeInstr(et)
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
			return &MarshalerError{err, t, jettisonMarshalerCtx}
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
		if !es.addressable && !es.inputPtr {
			return finstr(p, w, es)
		}
		v := reflect.NewAt(t, p)
		m := v.Interface().(Marshaler)
		if err := m.WriteJSON(w); err != nil {
			return &MarshalerError{err, reflect.PtrTo(t), jettisonMarshalerCtx}
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
			return &MarshalerError{err, t, jsonMarshalerCtx}
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
		if !es.addressable && !es.inputPtr {
			return finstr(p, w, es)
		}
		v := reflect.NewAt(t, p)
		m := v.Interface().(json.Marshaler)
		b, err := m.MarshalJSON()
		if err != nil {
			return &MarshalerError{err, reflect.PtrTo(t), jsonMarshalerCtx}
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
			return &MarshalerError{err, t, textMarshalerCtx}
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
		if !es.addressable && !es.inputPtr {
			return finstr(p, w, es)
		}
		v := reflect.NewAt(t, p)
		m := v.Interface().(encoding.TextMarshaler)
		b, err := m.MarshalText()
		if err != nil {
			return &MarshalerError{err, reflect.PtrTo(t), textMarshalerCtx}
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
	case timeType:
		return timeInstr
	case durationType:
		return durationInstr
	case numberType:
		return numberInstr
	default:
		return nil
	}
}

// primitiveInstr returns the instruction associated
// with the primitive type that has the given kind.
func primitiveInstr(k reflect.Kind) Instruction {
	switch k {
	case reflect.String:
		return stringInstr
	case reflect.Bool:
		return boolInstr
	case reflect.Int:
		return intInstr
	case reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64:
		return func(p unsafe.Pointer, w Writer, es *encodeState) error {
			return integerInstr(p, w, es, k)
		}
	case reflect.Uint:
		return uintInstr
	case reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr:
		return func(p unsafe.Pointer, w Writer, es *encodeState) error {
			return unsignedIntegerInstr(p, w, es, k)
		}
	case reflect.Float32:
		return func(p unsafe.Pointer, w Writer, es *encodeState) error {
			return floatInstr(p, w, es, 32)
		}
	case reflect.Float64:
		return func(p unsafe.Pointer, w Writer, es *encodeState) error {
			return floatInstr(p, w, es, 64)
		}
	default:
		return nil
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
		c := b[i]
		if c < utf8.RuneSelf {
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
			if err := w.WriteByte('\\'); err != nil {
				return err
			}
			var err error
			// The encoding/json package implements only
			// a few of the special two-character escape
			// sequence described in the RFC 8259, Section 7.
			// \b and \f were ignored on purpose, see
			// https://codereview.appspot.com/4678046.
			switch c {
			case '"', '\\':
				err = w.WriteByte(c)
			case '\n': // 0xA, line feed
				err = w.WriteByte('n')
			case '\r': // 0xD, carriage return
				err = w.WriteByte('r')
			case '\t': // 0x9, horizontal tab
				err = w.WriteByte('t')
			default:
				buf := es.scratch[:0]
				buf = append(buf, "u00"...)
				buf = append(buf, hex[c>>4])
				buf = append(buf, hex[c&0xF])
				_, err = w.Write(buf)
			}
			if err != nil {
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

//nolint:interfacer
func integerInstr(p unsafe.Pointer, w Writer, es *encodeState, k reflect.Kind) error {
	var i int64
	switch k {
	case reflect.Int8:
		i = int64(*(*int8)(p))
	case reflect.Int16:
		i = int64(*(*int16)(p))
	case reflect.Int32:
		i = int64(*(*int32)(p))
	case reflect.Int64:
		i = *(*int64)(p)
	default:
		return fmt.Errorf("invalid integer kind: %s", k)
	}
	b := strconv.AppendInt(es.scratch[:0], i, 10)
	_, err := w.Write(b)
	return err
}

func intInstr(p unsafe.Pointer, w Writer, es *encodeState) error {
	b := strconv.AppendInt(es.scratch[:0], int64(*(*int)(p)), 10)
	_, err := w.Write(b)
	return err
}

//nolint:interfacer
func unsignedIntegerInstr(p unsafe.Pointer, w Writer, es *encodeState, k reflect.Kind) error {
	var i uint64
	switch k {
	case reflect.Uint8:
		i = uint64(*(*uint8)(p))
	case reflect.Uint16:
		i = uint64(*(*uint16)(p))
	case reflect.Uint32:
		i = uint64(*(*uint32)(p))
	case reflect.Uint64:
		i = *(*uint64)(p)
	case reflect.Uintptr:
		i = uint64(*(*uintptr)(p))
	default:
		return fmt.Errorf("invalid unsigned integer kind: %s", k)
	}
	b := strconv.AppendUint(es.scratch[:0], i, 10)
	_, err := w.Write(b)
	return err
}

func uintInstr(p unsafe.Pointer, w Writer, es *encodeState) error {
	b := strconv.AppendUint(es.scratch[:0], uint64(*(*uint)(p)), 10)
	_, err := w.Write(b)
	return err
}

//nolint:interfacer
func floatInstr(p unsafe.Pointer, w Writer, es *encodeState, bitSize int) error {
	var f float64
	switch bitSize {
	case 32:
		f = float64(*(*float32)(p))
	case 64:
		f = *(*float64)(p)
	default:
		return fmt.Errorf("invalid float bit size: %d", bitSize)
	}
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
	if shdr.Data == uintptr(0) {
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
		return fmt.Errorf("invalid number literal %q", n)
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
	// Optional -
	if s[0] == '-' {
		s = s[1:]
		if s == "" {
			return false
		}
	}
	// Digits
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
	// . followed by 1 or more digits.
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
	b := t.AppendFormat(es.scratch[:0], es.opts.timeLayout)
	if err := w.WriteByte('"'); err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	return w.WriteByte('"')
}

// durationInstr writes a time.Duration to w.
func durationInstr(p unsafe.Pointer, w Writer, es *encodeState) error {
	d := *(*time.Duration)(p)

	if es.opts.durationFmt == DurationString {
		s := d.String()
		b := *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
			Data: (*reflect.StringHeader)(unsafe.Pointer(&s)).Data,
			Len:  len(s),
			Cap:  len(s),
		}))
		if err := w.WriteByte('"'); err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
		return w.WriteByte('"')
	}
	switch es.opts.durationFmt {
	case DurationMinutes:
		f := d.Minutes()
		return floatInstr(unsafe.Pointer(&f), w, es, 64)
	case DurationSeconds:
		f := d.Seconds()
		return floatInstr(unsafe.Pointer(&f), w, es, 64)
	case DurationMicroseconds:
		i := int64(d) / 1e3
		return integerInstr(unsafe.Pointer(&i), w, es, reflect.Int64)
	case DurationMilliseconds:
		i := int64(d) / 1e6
		return integerInstr(unsafe.Pointer(&i), w, es, reflect.Int64)
	case DurationNanoseconds:
		i := d.Nanoseconds()
		return integerInstr(unsafe.Pointer(&i), w, es, reflect.Int64)
	}
	return fmt.Errorf("unknown duration format: %v", es.opts.durationFmt)
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
	instr, err := cachedTypeInstr(vt)
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
	eins, err := cachedTypeInstr(et)
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
	eins, err := cachedTypeInstr(et)
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
		if shdr.Data == uintptr(0) {
			if es.opts.nilSliceEmpty {
				_, err := w.WriteString("[]")
				return err
			}
			// A nil slice cannot be treated like a primitive
			// type because the pointer will always point to a
			// non-nil slice header which contains a Data field
			// with an empty memory address.
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
	vinstr, err := cachedTypeInstr(t.Elem())
	if err != nil {
		return nil, err
	}
	// The standard library has a strict precedence order
	// for map key types, defined in the documentation of
	// the json.Marshal functions; that's why we bypass
	// the TextMarshaler instructions if the key is a string.
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
	// Keep in sync with types handled by specialTypeInstr.
	return t == timeType || t == durationType || t == numberType
}

func isPrimitiveType(t reflect.Type) bool {
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
