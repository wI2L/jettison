package jettison

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
	"unsafe"

	"github.com/modern-go/reflect2"
)

var statePool = sync.Pool{}

// Writer is an interface that groups the
// io.Writer, io.StringWriter and io.ByteWriter
// interfaces.
type Writer interface {
	io.Writer
	io.StringWriter
	io.ByteWriter
}

// Instruction represents a function that writes
// the JSON representation of a value to a stream.
type Instruction func(unsafe.Pointer, Writer, *encodeState) error

// Encoder writes the JSON values of a specific
// type using a set of instructions compiled when
// the encoder is instantiated.
type Encoder struct {
	typ  reflect.Type
	ins  Instruction
	once sync.Once
}

// Option represents an option that
// defines the behavior of an encoder.
type Option func(*encodeState)

// TimeLayout sets the time layout used to
// encode a time.Time value.
func TimeLayout(layout string) Option {
	return func(es *encodeState) { es.timeLayout = layout }
}

// DurationFormat sets the format used to
// encode a time.Duration value.
func DurationFormat(df DurationFmt) Option {
	return func(es *encodeState) { es.durationFmt = df }
}

// UnixTimestamp configures the encoder to encode
// time.Time value as Unix timestamps. This setting
// has precedence over any time layout.
func UnixTimestamp(es *encodeState) {
	es.useTimestamps = true
}

// UnsortedMap disables map keys sort.
func UnsortedMap(es *encodeState) {
	es.unsortedMap = true
}

// ByteArrayAsString encodes byte arrays as
// raw JSON strings.
func ByteArrayAsString(es *encodeState) {
	es.byteArrayAsString = true
}

// RawByteSlices disables the default behavior that
// encodes byte slices as base64-encoded strings.
func RawByteSlices(es *encodeState) {
	es.noBase64Slice = true
}

// NilMapEmpty encodes a nil Go map as an
// empty JSON object, rather than null.
func NilMapEmpty(es *encodeState) {
	es.nilMapEmpty = true
}

// NilSliceEmpty encodes a nil Go slice as
// an empty JSON array, rather than null.
func NilSliceEmpty(es *encodeState) {
	es.nilSliceEmpty = true
}

// NoStringEscaping disables string escaping.
func NoStringEscaping(es *encodeState) {
	es.noStringEscape = true
}

// DurationFmt represents the format used
// to encode a time.Duration.
type DurationFmt int

// Duration formats.
const (
	DurationString = iota
	DurationMinutes
	DurationSeconds
	DurationMilliseconds
	DurationMicroseconds
	DurationNanoseconds
)

// String implements the fmt.Stringer interface.
func (df DurationFmt) String() string {
	if df < DurationString || df > DurationNanoseconds {
		return "unknown"
	}
	names := []string{
		"str", "min", "s", "ms", "μs", "nanosecond",
	}
	return names[df]
}

type encodeState struct {
	// inputPrt indicates if the input
	// value to encode is a pointer.
	inputPtr bool

	// scratch is used as temporary buffer
	// for types conversions using Append*
	// like functions.
	scratch [64]byte

	// firstField tracks whether the first
	// field of an object has been written.
	firstField bool

	// Runtime options.
	// All are optin-in only or have default
	// values to comply with the std library
	// behavior.
	timeLayout        string
	useTimestamps     bool
	durationFmt       DurationFmt
	unsortedMap       bool
	noBase64Slice     bool
	byteArrayAsString bool
	nilMapEmpty       bool
	nilSliceEmpty     bool
	noStringEscape    bool
}

func newState() *encodeState {
	if v := statePool.Get(); v != nil {
		s := v.(*encodeState)
		s.Reset()
		return s
	}
	return &encodeState{timeLayout: defaultTimeLayout}
}

func (s *encodeState) Reset() {
	s.firstField = false
	s.timeLayout = defaultTimeLayout
	s.useTimestamps = false
	s.durationFmt = DurationString
	s.unsortedMap = false
	s.noBase64Slice = false
	s.byteArrayAsString = false
	s.nilMapEmpty = false
	s.nilSliceEmpty = false
	s.noStringEscape = false
}

// UnsupportedTypeError is the error returned by
// an Encoder's Encode method when attempting to
// encode an unsupported value type.
type UnsupportedTypeError struct {
	Typ     reflect.Type
	Context string
}

// Error implements the bultin error interface.
func (e *UnsupportedTypeError) Error() string {
	return fmt.Sprintf("unsupported type: %s", e.Typ)
}

// UnsupportedValueError is the error returned by
// an Encoder's Encode method when attempting to
// encode an unsupported value.
type UnsupportedValueError struct {
	Str string
}

// Error implements the builtin error interface.
func (e *UnsupportedValueError) Error() string {
	return fmt.Sprintf("unsupported value: %s", e.Str)
}

// MarshalerError represents an error from calling
// a MarshalJSON or MarshalText method.
type MarshalerError struct {
	Typ reflect.Type
	Err error
}

// Error implements the builtin error interface.
func (e *MarshalerError) Error() string {
	return fmt.Sprintf("error calling Marshal* for type %v: %s", e.Typ, e.Err)
}

// NewEncoder returns a new encoder that can marshal the
// values of the given type. The Encoder can be explicitly
// initialized by calling its Compile method, otherwise the
// operation is done on first call to Marshal.
func NewEncoder(i interface{}) (*Encoder, error) {
	if i == nil {
		return nil, errors.New("nil interface")
	}
	return &Encoder{typ: reflect.TypeOf(i)}, nil
}

// Compile generates the encoder's instructions.
// Calling this method more than once is a noop.
func (e *Encoder) Compile() error {
	return e.compile()
}

// Encode writes the JSON encoding of i to w.
func (e *Encoder) Encode(i interface{}, w Writer, opts ...Option) error {
	if i == nil {
		_, err := w.WriteString("null")
		return err
	}
	// Ensure that the instructions are already
	// compiled, or do it once, immediately.
	if err := e.compile(); err != nil {
		return err
	}
	typ := reflect.TypeOf(i)

	var p unsafe.Pointer

	if typ.Kind() == reflect.Map {
		// Value is not addressable, create a new
		// pointer of the type and assign the value.
		v := reflect.ValueOf(i)
		vp := reflect.New(v.Type())
		vp.Elem().Set(v)
		v = vp
		p = unsafe.Pointer(v.Elem().UnsafeAddr())
	} else {
		// Unpack eface and use the data pointer.
		p = reflect2.PtrOf(i)
	}
	if p == nilptr {
		// The exception for the struct type comes
		// from the fact that the pointer may points
		// to an anonymous struct field that should
		// still be serialized as part of the struct,
		// or has the omitempty option.
		if e.typ.Kind() != reflect.Struct {
			_, err := w.WriteString("null")
			return err
		}
	}
	isPtr := typ.Kind() == reflect.Ptr
	if isPtr {
		typ = typ.Elem()
	}
	if typ != e.typ {
		return fmt.Errorf("for %s encoder, incompatible value type: %v", e.typ, typ)
	}
	es := newState()
	es.inputPtr = isPtr

	// Apply options to state.
	for _, o := range opts {
		o(es)
	}
	// Execute the instruction with the state
	// and the given writer.
	if err := e.ins(p, w, es); err != nil {
		return err
	}
	statePool.Put(es)
	return nil
}

func (e *Encoder) compile() error {
	var err error
	e.once.Do(func() {
		if e.typ.Kind() == reflect.Ptr {
			e.typ = e.typ.Elem()
		}
		err = e.encodeType(e.typ)
	})
	return err
}

// encodeType generates the instruction required to encode
// the given type. It returns an error if the type is not
// supported, such as channel, complex and function values.
func (e *Encoder) encodeType(t reflect.Type) error {
	ins, err := cachedTypeInstr(t)
	if err != nil {
		return err
	}
	e.ins = ins
	return nil
}
