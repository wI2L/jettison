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

// ErrInvalidWriter is the error returned by an
// Encoder's Encode method when the given Writer
// is invalid.
var ErrInvalidWriter = errors.New("invalid writer")

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

type encodeState struct {
	opts encodeOpts

	// inputPtr indicates if the input
	// value to encode is a pointer.
	inputPtr bool

	// scratch is used as temporary buffer
	// for types conversions using Append*
	// like functions.
	scratch [64]byte

	// firstField tracks whether the first
	// field of an object has been written.
	firstField bool

	// addressable tracks whether the value
	// to encode is addressable.
	addressable bool
}

// encodeOpts represents the runtime options
// of an encoder. All options are opt-in and
// have a default value that comply with the
// standard library behavior.
type encodeOpts struct {
	timeLayout        string
	useTimestamps     bool
	durationFmt       DurationFmt
	unsortedMap       bool
	noBase64Slice     bool
	byteArrayAsString bool
	nilMapEmpty       bool
	nilSliceEmpty     bool
	noStringEscape    bool
	noUTF8Coercion    bool
	noHTMLEscape      bool
}

func newState() *encodeState {
	if v := statePool.Get(); v != nil {
		s := v.(*encodeState)
		s.Reset()
		return s
	}
	return &encodeState{opts: encodeOpts{
		timeLayout: defaultTimeLayout},
	}
}

func (s *encodeState) Reset() {
	s.firstField = false
	s.opts.timeLayout = defaultTimeLayout
	s.opts.useTimestamps = false
	s.opts.durationFmt = DurationString
	s.opts.unsortedMap = false
	s.opts.noBase64Slice = false
	s.opts.byteArrayAsString = false
	s.opts.nilMapEmpty = false
	s.opts.nilSliceEmpty = false
	s.opts.noStringEscape = false
	s.opts.noUTF8Coercion = false
	s.opts.noHTMLEscape = false
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

// TypeMismatchError is the error returned by an
// Encoder's Encode method whhen the type of the
// input value does not match the type for which
// the encoder was compiled.
type TypeMismatchError struct {
	SrcType reflect.Type
	EncType reflect.Type
}

// Error implements the builtin error interface.
func (e *TypeMismatchError) Error() string {
	return fmt.Sprintf("incompatible value type: %v", e.SrcType)
}

type marshalerCtx string

const (
	jsonMarshalerCtx     marshalerCtx = "JSON"
	textMarshalerCtx     marshalerCtx = "Text"
	jettisonMarshalerCtx marshalerCtx = "Jettison"
)

// MarshalerError represents an error from calling
// a MarshalJSON or MarshalText method.
type MarshalerError struct {
	Err error
	Typ reflect.Type
	ctx marshalerCtx
}

// Error implements the builtin error interface.
func (e *MarshalerError) Error() string {
	return fmt.Sprintf("error calling Marshal%s for type %s: %s", e.ctx, e.Typ.String(), e.Err)
}

// Unwrap returns the wrapped error.
// This doesn't implement a public interface, but
// allow to use the errors.Unwrap function released
// in Go 1.13 with a MarshalerError.
func (e *MarshalerError) Unwrap() error { return e.Err }

// NewEncoder returns a new encoder that can marshal the
// values of the given type. The Encoder can be explicitly
// initialized by calling its Compile method, otherwise the
// operation is done on first call to Marshal.
func NewEncoder(rt reflect.Type) (*Encoder, error) {
	if rt == nil {
		return nil, errors.New("invalid type: nil")
	}
	return &Encoder{typ: rt}, nil
}

// Encode writes the JSON encoding of i to w.
func (e *Encoder) Encode(i interface{}, w Writer, opts ...Option) error {
	if w == nil {
		return ErrInvalidWriter
	}
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

	return e.encode(typ, i, w, opts...)
}

// Compile generates the encoder's instructions.
// Calling this method more than once is a noop.
func (e *Encoder) Compile() error {
	return e.compile()
}

func (e *Encoder) compile() error {
	var err error
	e.once.Do(func() {
		if e.typ.Kind() == reflect.Ptr {
			e.typ = e.typ.Elem()
		}
		err = e.genInstr(e.typ)
	})
	return err
}

func (e *Encoder) encode(t reflect.Type, i interface{}, w Writer, opts ...Option) error {
	var p unsafe.Pointer

	if t.Kind() == reflect.Map {
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
	if p == nil {
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
	isPtr := t.Kind() == reflect.Ptr
	if isPtr {
		t = t.Elem()
	}
	if t != e.typ {
		return &TypeMismatchError{SrcType: t, EncType: e.typ}
	}
	es := newState()
	es.inputPtr = isPtr

	// Apply options to state.
	for _, o := range opts {
		if o != nil {
			o(&es.opts)
		}
	}
	// Execute the instruction with the state
	// and the given writer.
	if err := e.ins(p, w, es); err != nil {
		return err
	}
	statePool.Put(es)
	return nil
}

// genInstr generates the instruction required to encode
// the given type. It returns an error if the type is not
// supported, such as channel, complex and function values.
func (e *Encoder) genInstr(t reflect.Type) error {
	ins, err := cachedTypeInstr(t)
	if err != nil {
		return err
	}
	e.ins = ins
	return nil
}
