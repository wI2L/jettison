package jettison

import (
	"encoding"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"
	"unsafe"
)

const hex = "0123456789abcdef"

//nolint:unparam
func encodeBool(p unsafe.Pointer, dst []byte, _ encOpts) ([]byte, error) {
	if *(*bool)(p) {
		return append(dst, "true"...), nil
	}
	return append(dst, "false"...), nil
}

// encodeString appends the escaped bytes of the string
// pointed by p to dst. If quoted is true, escaped double
// quote characters are added at the beginning and the
// end of the JSON string.
// nolint:unparam
func encodeString(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
	dst = append(dst, '"')
	dst = appendEscapedBytes(dst, sp2b(p), opts)
	dst = append(dst, '"')

	return dst, nil
}

//nolint:unparam
func encodeQuotedString(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
	dst = append(dst, `"\"`...)
	dst = appendEscapedBytes(dst, sp2b(p), opts)
	dst = append(dst, `\""`...)

	return dst, nil
}

// encodeFloat32 appends the textual representation of
// the 32-bits floating point number pointed by p to dst.
func encodeFloat32(p unsafe.Pointer, dst []byte, _ encOpts) ([]byte, error) {
	return appendFloat(dst, float64(*(*float32)(p)), 32)
}

// encodeFloat64 appends the textual representation of
// the 64-bits floating point number pointed by p to dst.
func encodeFloat64(p unsafe.Pointer, dst []byte, _ encOpts) ([]byte, error) {
	return appendFloat(dst, *(*float64)(p), 64)
}

func encodeInterface(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
	v := *(*interface{})(p)
	if v == nil {
		return append(dst, "null"...), nil
	}
	typ := reflect.TypeOf(v)
	ins := cachedInstr(typ)

	return ins(unpackEface(v).word, dst, opts)
}

func encodeNumber(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
	// Cast pointer to string directly to avoid
	// a useless conversion.
	num := *(*string)(p)

	// In Go1.5 the empty string encodes to "0".
	// While this is not a valid number literal,
	// we keep compatibility, so check validity
	// after this.
	if num == "" {
		num = "0" // Number's zero-val
	}
	if !opts.flags.has(noNumberValidation) && !isValidNumber(num) {
		return dst, fmt.Errorf("json: invalid number literal %q", num)
	}
	return append(dst, num...), nil
}

func encodeRawMessage(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
	v := *(*json.RawMessage)(p)
	if v == nil {
		return append(dst, "null"...), nil
	}
	if opts.flags.has(noCompact) {
		return append(dst, v...), nil
	}
	return appendCompactJSON(dst, v, !opts.flags.has(noHTMLEscaping))
}

// encodeTime appends the time.Time value pointed by
// p to dst based on the format configured in opts.
func encodeTime(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
	t := *(*time.Time)(p)
	y := t.Year()

	if y < 0 || y >= 10000 {
		// See comment golang.org/issue/4556#c15.
		return dst, errors.New("time: year outside of range [0,9999]")
	}
	if opts.flags.has(unixTime) {
		return strconv.AppendInt(dst, t.Unix(), 10), nil
	}
	switch opts.timeLayout {
	case time.RFC3339:
		return appendRFC3339Time(t, dst, false), nil
	case time.RFC3339Nano:
		return appendRFC3339Time(t, dst, true), nil
	default:
		dst = append(dst, '"')
		dst = t.AppendFormat(dst, opts.timeLayout)
		dst = append(dst, '"')
		return dst, nil
	}
}

// encodeDuration appends the time.Duration value pointed
// by p to dst based on the format configured in opts.
func encodeDuration(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
	d := *(*time.Duration)(p)

	switch opts.durationFmt {
	default: // DurationNanoseconds
		return strconv.AppendInt(dst, d.Nanoseconds(), 10), nil
	case DurationMinutes:
		return appendFloat(dst, d.Minutes(), 64)
	case DurationSeconds:
		return appendFloat(dst, d.Seconds(), 64)
	case DurationMicroseconds:
		return strconv.AppendInt(dst, int64(d)/1e3, 10), nil
	case DurationMilliseconds:
		return strconv.AppendInt(dst, int64(d)/1e6, 10), nil
	case DurationString:
		dst = append(dst, '"')
		dst = appendDuration(dst, d)
		dst = append(dst, '"')
		return dst, nil
	}
}

func appendFloat(dst []byte, f float64, bs int) ([]byte, error) {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return dst, &UnsupportedValueError{
			reflect.ValueOf(f),
			strconv.FormatFloat(f, 'g', -1, bs),
		}
	}
	// Convert as it was an ES6 number to string conversion.
	// This matches most other JSON generators. The following
	// code is taken from the floatEncoder implementation of
	// the encoding/json package of the Go standard library.
	abs := math.Abs(f)
	format := byte('f')
	if abs != 0 {
		if bs == 64 && (abs < 1e-6 || abs >= 1e21) ||
			bs == 32 && (float32(abs) < 1e-6 || float32(abs) >= 1e21) {
			format = 'e'
		}
	}
	dst = strconv.AppendFloat(dst, f, format, -1, bs)
	if format == 'e' {
		n := len(dst)
		if n >= 4 && dst[n-4] == 'e' && dst[n-3] == '-' && dst[n-2] == '0' {
			dst[n-2] = dst[n-1]
			dst = dst[:n-1]
		}
	}
	return dst, nil
}

func encodePointer(p unsafe.Pointer, dst []byte, opts encOpts, ins instruction) ([]byte, error) {
	if p = *(*unsafe.Pointer)(p); p != nil {
		return ins(p, dst, opts)
	}
	return append(dst, "null"...), nil
}

func encodeStruct(
	p unsafe.Pointer, dst []byte, opts encOpts, flds []field,
) ([]byte, error) {
	var (
		nxt = byte('{')
		key []byte // key of the field
	)
	noHTMLEscape := opts.flags.has(noHTMLEscaping)

fieldLoop:
	for i := 0; i < len(flds); i++ {
		f := &flds[i] // get pointer to prevent copy
		if opts.isDeniedField(f.name) {
			continue
		}
		v := p

		// Find the nested struct field by following
		// the offset sequence, indirecting encountered
		// pointers as needed.
		for _, s := range f.embedSeq {
			v = unsafe.Pointer(uintptr(v) + s.offset)
			if s.indir {
				if v = *(*unsafe.Pointer)(v); v == nil {
					// When we encounter a nil pointer
					// in the chain, we have no choice
					// but to ignore the field.
					continue fieldLoop
				}
			}
		}
		// Ignore the field if it is a nil pointer and has
		// the omitnil option in his tag.
		if f.omitNil && *(*unsafe.Pointer)(v) == nil {
			continue
		}
		// Ignore the field if it represents the zero-value
		// of its type and has the omitempty option in his tag.
		// Empty func is non-nil only if the field has the
		// omitempty option in its tag.
		if f.omitEmpty && f.empty(v) {
			continue
		}
		if noHTMLEscape {
			key = f.keyNonEsc
		} else {
			key = f.keyEscHTML
		}
		dst = append(dst, nxt)
		nxt = ','
		dst = append(dst, key...)

		var err error
		if dst, err = f.instr(v, dst, opts); err != nil {
			return dst, err
		}
	}
	if nxt == '{' {
		return append(dst, "{}"...), nil
	}
	return append(dst, '}'), nil
}

func encodeSlice(
	p unsafe.Pointer, dst []byte, opts encOpts, ins instruction, es uintptr,
) ([]byte, error) {
	shdr := (*sliceHeader)(p)
	if shdr.Data == nil {
		if opts.flags.has(nilSliceEmpty) {
			return append(dst, "[]"...), nil
		}
		return append(dst, "null"...), nil
	}
	if shdr.Len == 0 {
		return append(dst, "[]"...), nil
	}
	return encodeArray(shdr.Data, dst, opts, ins, es, shdr.Len, false)
}

// encodeByteSlice appends a byte slice to dst as
// a JSON string. If the options flag rawByteSlice
// is set, the escaped bytes are appended to the
// buffer directly, otherwise in base64 form.
// nolint:unparam
func encodeByteSlice(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
	b := *(*[]byte)(p)
	if b == nil {
		return append(dst, "null"...), nil
	}
	dst = append(dst, '"')

	if opts.flags.has(rawByteSlice) {
		dst = appendEscapedBytes(dst, b, opts)
	} else {
		n := base64.StdEncoding.EncodedLen(len(b))
		if a := cap(dst) - len(dst); a < n {
			new := make([]byte, cap(dst)+(n-a))
			copy(new, dst)
			dst = new[:len(dst)]
		}
		end := len(dst) + n
		base64.StdEncoding.Encode(dst[len(dst):end], b)

		dst = dst[:end]
	}
	return append(dst, '"'), nil
}

func encodeArray(
	p unsafe.Pointer, dst []byte, opts encOpts, ins instruction, es uintptr, len int, isByteArray bool,
) ([]byte, error) {
	if isByteArray && opts.flags.has(byteArrayAsString) {
		return encodeByteArrayAsString(p, dst, opts, len), nil
	}
	var err error
	nxt := byte('[')

	for i := 0; i < len; i++ {
		dst = append(dst, nxt)
		nxt = ','
		v := unsafe.Pointer(uintptr(p) + (uintptr(i) * es))
		if dst, err = ins(v, dst, opts); err != nil {
			return dst, err
		}
	}
	if nxt == '[' {
		return append(dst, "[]"...), nil
	}
	return append(dst, ']'), nil
}

// encodeByteArrayAsString appends the escaped
// bytes of the byte array pointed by p to dst
// as a JSON string.
func encodeByteArrayAsString(p unsafe.Pointer, dst []byte, opts encOpts, len int) []byte {
	// For byte type, size is guaranteed to be 1,
	// so the slice length is the same as the array's.
	// see golang.org/ref/spec#Size_and_alignment_guarantees
	b := *(*[]byte)(unsafe.Pointer(&sliceHeader{
		Data: p,
		Len:  len,
		Cap:  len,
	}))
	dst = append(dst, '"')
	dst = appendEscapedBytes(dst, b, opts)
	dst = append(dst, '"')

	return dst
}

func encodeMap(
	p unsafe.Pointer, dst []byte, opts encOpts, t reflect.Type, ki, vi instruction,
) ([]byte, error) {
	m := *(*unsafe.Pointer)(p)
	if m == nil {
		if opts.flags.has(nilMapEmpty) {
			return append(dst, "{}"...), nil
		}
		return append(dst, "null"...), nil
	}
	ml := maplen(m)
	if ml == 0 {
		return append(dst, "{}"...), nil
	}
	dst = append(dst, '{')

	rt := unpackEface(t).word
	it := newHiter(rt, m)

	var err error
	if opts.flags.has(unsortedMap) {
		dst, err = encodeUnsortedMap(it, dst, opts, ki, vi)
	} else {
		dst, err = encodeSortedMap(it, dst, opts, ki, vi, ml)
	}
	hiterPool.Put(it)

	if err != nil {
		return dst, err
	}
	return append(dst, '}'), err
}

// encodeUnsortedMap appends the elements of the map
// pointed by p as comma-separated k/v pairs to dst,
// in unspecified order.
func encodeUnsortedMap(
	it *hiter, dst []byte, opts encOpts, ki, vi instruction,
) ([]byte, error) {
	var (
		n   int
		err error
	)
	for ; it.key != nil; mapiternext(it) {
		if n != 0 {
			dst = append(dst, ',')
		}
		// Encode entry's key.
		if dst, err = ki(it.key, dst, opts); err != nil {
			return dst, err
		}
		dst = append(dst, ':')

		// Encode entry's value.
		if dst, err = vi(it.val, dst, opts); err != nil {
			return dst, err
		}
		n++
	}
	return dst, nil
}

// encodeUnsortedMap appends the elements of the map
// pointed by p as comma-separated k/v pairs to dst,
// sorted by key in lexicographical order.
func encodeSortedMap(
	it *hiter, dst []byte, opts encOpts, ki, vi instruction, ml int,
) ([]byte, error) {
	var (
		off int
		err error
		buf = cachedBuffer()
		mel *mapElems
	)
	if v := mapElemsPool.Get(); v != nil {
		mel = v.(*mapElems)
	} else {
		mel = &mapElems{s: make([]kv, 0, ml)}
	}
	for ; it.key != nil; mapiternext(it) {
		kv := kv{}

		// Encode the key and store the buffer
		// portion to use during sort.
		if buf.B, err = ki(it.key, buf.B, opts); err != nil {
			break
		}
		// Omit quotes of keys.
		kv.key = buf.B[off+1 : len(buf.B)-1]

		// Add separator after key.
		buf.B = append(buf.B, ':')

		// Encode the value and store the buffer
		// portion corresponding to the semicolon
		// delimited key/value pair.
		if buf.B, err = vi(it.val, buf.B, opts); err != nil {
			break
		}
		kv.keyval = buf.B[off:len(buf.B)]
		mel.s = append(mel.s, kv)
		off = len(buf.B)
	}
	if err == nil {
		// Sort map entries by key in
		// lexicographical order.
		sort.Sort(mel)

		// Append sorted comma-delimited k/v
		// pairs to the given buffer.
		for i, kv := range mel.s {
			if i != 0 {
				dst = append(dst, ',')
			}
			dst = append(dst, kv.keyval...)
		}
	}
	// The map elements must be released before
	// the buffer, because each k/v pair holds
	// two sublices that points to the buffer's
	// backing array.
	releaseMapElems(mel)
	bufferPool.Put(buf)

	return dst, err
}

// encodeSyncMap appends the elements of a sync.Map pointed
// to by p to dst and returns the extended buffer.
// This function replicates the behavior of encoding Go maps,
// by returning an error for keys that are not of type string
// or int, or that does not implement encoding.TextMarshaler.
func encodeSyncMap(p unsafe.Pointer, dst []byte, opts encOpts) ([]byte, error) {
	sm := (*sync.Map)(p)
	dst = append(dst, '{')

	// The sync.Map type does not have a Len() method to
	// determine if it has no entries, to bail out early,
	// so we just range over it to encode all available
	// entries.
	// If an error arises while encoding a key or a value,
	// the error is stored and the method used by Range()
	// returns false to stop the map's iteration.
	var err error
	if opts.flags.has(unsortedMap) {
		dst, err = encodeUnsortedSyncMap(sm, dst, opts)
	} else {
		dst, err = encodeSortedSyncMap(sm, dst, opts)
	}
	if err != nil {
		return dst, err
	}
	return append(dst, '}'), nil
}

// encodeUnsortedSyncMap is similar to encodeUnsortedMap
// but operates on a sync.Map type instead of a Go map.
func encodeUnsortedSyncMap(sm *sync.Map, dst []byte, opts encOpts) ([]byte, error) {
	var (
		n   int
		err error
	)
	sm.Range(func(key, value interface{}) bool {
		if n != 0 {
			dst = append(dst, ',')
		}
		// Encode the key.
		if dst, err = appendSyncMapKey(dst, key, opts); err != nil {
			return false
		}
		dst = append(dst, ':')

		// Encode the value.
		if dst, err = appendJSON(dst, value, opts); err != nil {
			return false
		}
		n++
		return true
	})
	return dst, err
}

// encodeSortedSyncMap is similar to encodeSortedMap
// but operates on a sync.Map type instead of a Go map.
func encodeSortedSyncMap(sm *sync.Map, dst []byte, opts encOpts) ([]byte, error) {
	var (
		off int
		err error
		buf = cachedBuffer()
		mel *mapElems
	)
	if v := mapElemsPool.Get(); v != nil {
		mel = v.(*mapElems)
	} else {
		mel = &mapElems{s: make([]kv, 0)}
	}
	sm.Range(func(key, value interface{}) bool {
		kv := kv{}

		// Encode the key and store the buffer
		// portion to use during the later sort.
		if buf.B, err = appendSyncMapKey(buf.B, key, opts); err != nil {
			return false
		}
		// Omit quotes of keys.
		kv.key = buf.B[off+1 : len(buf.B)-1]

		// Add separator after key.
		buf.B = append(buf.B, ':')

		// Encode the value and store the buffer
		// portion corresponding to the semicolon
		// delimited key/value pair.
		if buf.B, err = appendJSON(buf.B, value, opts); err != nil {
			return false
		}
		kv.keyval = buf.B[off:len(buf.B)]
		mel.s = append(mel.s, kv)
		off = len(buf.B)

		return true
	})
	if err == nil {
		// Sort map entries by key in
		// lexicographical order.
		sort.Sort(mel)

		// Append sorted comma-delimited k/v
		// pairs to the given buffer.
		for i, kv := range mel.s {
			if i != 0 {
				dst = append(dst, ',')
			}
			dst = append(dst, kv.keyval...)
		}
	}
	releaseMapElems(mel)
	bufferPool.Put(buf)

	return dst, err
}

func appendSyncMapKey(dst []byte, key interface{}, opts encOpts) ([]byte, error) {
	if key == nil {
		return dst, errors.New("unsupported nil key in sync.Map")
	}
	kt := reflect.TypeOf(key)
	var (
		isStr = isString(kt)
		isInt = isInteger(kt)
		isTxt = kt.Implements(textMarshalerType)
	)
	if !isStr && !isInt && !isTxt {
		return dst, fmt.Errorf("unsupported key of type %s in sync.Map", kt)
	}
	var err error

	// Quotes the key if the type is not
	// encoded with quotes by default.
	quoted := !isStr && !isTxt

	// Ensure map key precedence for keys of type
	// string by using the encodeString function
	// directly instead of the generic appendJSON.
	if isStr {
		dst, err = encodeString(unpackEface(key).word, dst, opts)
		runtime.KeepAlive(key)
	} else {
		if quoted {
			dst = append(dst, '"')
		}
		dst, err = appendJSON(dst, key, opts)
	}
	if err != nil {
		return dst, err
	}
	if quoted {
		dst = append(dst, '"')
	}
	return dst, nil
}

func encodeMarshaler(
	p unsafe.Pointer, dst []byte, opts encOpts, t reflect.Type, canAddr bool, fn marshalerEncodeFunc,
) ([]byte, error) {
	// The content of this function and packEface
	// is similar to the following code using the
	// reflect package.
	//
	// v := reflect.NewAt(t, p)
	// if !canAddr {
	// 	v = v.Elem()
	// 	k := v.Kind()
	// 	if (k == reflect.Ptr || k == reflect.Interface) && v.IsNil() {
	// 		return append(dst, "null"...), nil
	// 	}
	// } else if v.IsNil() {
	// 	return append(dst, "null"...), nil
	// }
	// return fn(v.Interface(), dst, opts, t)
	//
	if !canAddr {
		if t.Kind() == reflect.Ptr || t.Kind() == reflect.Interface {
			if *(*unsafe.Pointer)(p) == nil {
				return append(dst, "null"...), nil
			}
		}
	} else {
		if p == nil {
			return append(dst, "null"...), nil
		}
		t = reflect.PtrTo(t)
	}
	var i interface{}

	if t.Kind() == reflect.Interface {
		// Special case: return the element inside the
		// interface. The empty interface has one layout,
		// all interfaces with methods have another one.
		if t.NumMethod() == 0 {
			i = *(*interface{})(p)
		} else {
			i = *(*interface{ M() })(p)
		}
	} else {
		i = packEface(p, t, t.Kind() == reflect.Ptr && !canAddr)
	}
	return fn(i, dst, opts, t)
}

func encodeAppendMarshalerCtx(
	i interface{}, dst []byte, opts encOpts, t reflect.Type,
) ([]byte, error) {
	dst2, err := i.(AppendMarshalerCtx).AppendJSONContext(opts.ctx, dst)
	if err != nil {
		return dst, &MarshalerError{t, err, marshalerAppendJSONCtx}
	}
	return dst2, nil
}

func encodeAppendMarshaler(
	i interface{}, dst []byte, _ encOpts, t reflect.Type,
) ([]byte, error) {
	dst2, err := i.(AppendMarshaler).AppendJSON(dst)
	if err != nil {
		return dst, &MarshalerError{t, err, marshalerAppendJSON}
	}
	return dst2, nil
}

func encodeJSONMarshaler(i interface{}, dst []byte, opts encOpts, t reflect.Type) ([]byte, error) {
	b, err := i.(json.Marshaler).MarshalJSON()
	if err != nil {
		return dst, &MarshalerError{t, err, marshalerJSON}
	}
	if opts.flags.has(noCompact) {
		return append(dst, b...), nil
	}
	// This is redundant with the parsing done
	// by appendCompactJSON, but for the time
	// being, we can't use the scanner of the
	// standard library.
	if !json.Valid(b) {
		return dst, &MarshalerError{t, &SyntaxError{
			msg: "json: invalid value",
		}, marshalerJSON}
	}
	return appendCompactJSON(dst, b, !opts.flags.has(noHTMLEscaping))
}

func encodeTextMarshaler(i interface{}, dst []byte, _ encOpts, t reflect.Type) ([]byte, error) {
	b, err := i.(encoding.TextMarshaler).MarshalText()
	if err != nil {
		return dst, &MarshalerError{t, err, marshalerText}
	}
	dst = append(dst, '"')
	dst = append(dst, b...)
	dst = append(dst, '"')

	return dst, nil
}

// appendCompactJSON appends to dst the JSON-encoded src
// with insignificant space characters elided. If escHTML
// is true, HTML-characters are also escaped.
func appendCompactJSON(dst, src []byte, escHTML bool) ([]byte, error) {
	var (
		inString bool
		skipNext bool
	)
	at := 0 // accumulated bytes start index

	for i, c := range src {
		if escHTML {
			// Escape HTML characters.
			if c == '<' || c == '>' || c == '&' {
				if at < i {
					dst = append(dst, src[at:i]...)
				}
				dst = append(dst, `\u00`...)
				dst = append(dst, hex[c>>4], hex[c&0xF])
				at = i + 1
				continue
			}
		}
		// Convert U+2028 and U+2029.
		// (E2 80 A8 and E2 80 A9).
		if c == 0xE2 && i+2 < len(src) && src[i+1] == 0x80 && src[i+2]&^1 == 0xA8 {
			if at < i {
				dst = append(dst, src[at:i]...)
			}
			dst = append(dst, `\u202`...)
			dst = append(dst, hex[src[i+2]&0xF])
			at = i + 3
			continue
		}
		if !inString {
			switch c {
			case '"':
				// Within a string, we don't elide
				// insignificant space characters.
				inString = true
			case ' ', '\n', '\r', '\t':
				// Append the accumulated bytes,
				// and skip the current character.
				if at < i {
					dst = append(dst, src[at:i]...)
				}
				at = i + 1
			}
			continue
		}
		// Next character is escaped, and must
		// not be interpreted as the end of a
		// string by mistake.
		if skipNext {
			skipNext = false
			continue
		}
		// Next character must be skipped.
		if c == '\\' {
			skipNext = true
			continue
		}
		// Leaving a string value.
		if c == '"' {
			inString = false
		}
	}
	if at < len(src) {
		dst = append(dst, src[at:]...)
	}
	return dst, nil
}

// isSafeJSONChar returns whether c can be used
// in a JSON string without escaping.
func isSafeJSONChar(c byte) bool {
	return c >= ' ' && c != '\\' && c != '"'
}

// isHTMLChar returns whether c is a problematic
// HTML cgaracter that must be escaped.
func isHTMLChar(c byte) bool {
	return c == '&' || c == '<' || c == '>'
}

func appendEscapedBytes(dst []byte, b []byte, opts encOpts) []byte {
	if opts.flags.has(noStringEscaping) {
		return append(dst, b...)
	}
	var (
		i  = 0
		at = 0
	)
	noCoerce := opts.flags.has(noUTF8Coercion)
	noEscape := opts.flags.has(noHTMLEscaping)

	for i < len(b) {
		if c := b[i]; c < utf8.RuneSelf {
			if isSafeJSONChar(c) && (noEscape || !isHTMLChar(c)) {
				// If the current character doesn't need
				// to be escaped, accumulate the bytes to
				// save some operations.
				i++
				continue
			}
			// Write accumulated single-byte characters.
			if at < i {
				dst = append(dst, b[at:i]...)
			}
			// The encoding/json package implements only
			// a few of the special two-character escape
			// sequence described in the RFC 8259, Section 7.
			// \b and \f were ignored on purpose, see
			// https://codereview.appspot.com/4678046.
			switch c {
			case '"', '\\':
				dst = append(dst, '\\', c)
			case '\n': // 0xA, line feed
				dst = append(dst, '\\', 'n')
			case '\r': // 0xD, carriage return
				dst = append(dst, '\\', 'r')
			case '\t': // 0x9, horizontal tab
				dst = append(dst, '\\', 't')
			default:
				dst = append(dst, `\u00`...)
				dst = append(dst, hex[c>>4])
				dst = append(dst, hex[c&0xF])
			}
			i++
			at = i
			continue
		}
		r, size := utf8.DecodeRune(b[i:])

		if !noCoerce {
			// Coerce to valid UTF-8, by replacing invalid
			// bytes with the Unicode replacement rune.
			if r == utf8.RuneError && size == 1 {
				if at < i {
					dst = append(dst, b[at:i]...)
				}
				dst = append(dst, `\ufffd`...)
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
			if r == '\u2028' || r == '\u2029' {
				if at < i {
					dst = append(dst, b[at:i]...)
				}
				dst = append(dst, `\u202`...)
				dst = append(dst, hex[r&0xF])
				i += size
				at = i
				continue
			}
			i += size
			continue
		}
		i += size
	}
	if at < len(b) {
		dst = append(dst, b[at:]...)
	}
	return dst
}
