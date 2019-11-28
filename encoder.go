package jettison

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
	"time"
	"unsafe"

	"github.com/modern-go/reflect2"
)

const (
	// defaultBase is the base used by default to encode
	// signed and unsigned integers, unless otherwise specified.
	defaultBase = 10

	// defaultTimeLayout is the layout used by default
	// to format a time.Time, unless otherwise specified.
	// This is compliant with the ECMA specification and
	// the JavaScript Date's toJSON method implementation.
	defaultTimeLayout = time.RFC3339Nano
)

var (
	todoCtx   = context.TODO()
	statePool = sync.Pool{}
)

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
	root Instruction
	once sync.Once
}

type encodeState struct {
	opts encodeOpts

	// scratch is used as temporary buffer
	// for types conversions using Append*
	// like functions.
	scratch [64]byte

	// ptrInput indicates if the input
	// value to encode is a pointer.
	ptrInput bool

	// firstField tracks whether the first
	// field of an object has been written.
	firstField bool

	// addressable tracks whether the value
	// to encode is addressable.
	addressable bool

	// depthLevel tracks the depth level of
	// the field being encoded within a struct.
	// The value 1 represents the top-level.
	depthLevel int
}

// encodeOpts represents the runtime options
// of an encoder. All options are opt-in and
// have a default value that comply with the
// standard library behavior.
type encodeOpts struct {
	ctx               context.Context
	timeLayout        string
	integerBase       int
	durationFmt       DurationFmt
	fieldsWhitelist   map[string]struct{}
	useTimestamps     bool
	unsortedMap       bool
	noBase64Slice     bool
	byteArrayAsString bool
	nilMapEmpty       bool
	nilSliceEmpty     bool
	noStringEscape    bool
	noUTF8Coercion    bool
	noHTMLEscape      bool
}

var zeroOpts = &encodeOpts{
	ctx:         todoCtx,
	timeLayout:  defaultTimeLayout,
	integerBase: defaultBase,
	// The remaining fields are set
	// to their zero-value.
}

func newState() *encodeState {
	if v := statePool.Get(); v != nil {
		s := v.(*encodeState)
		s.reset()
		return s
	}
	return &encodeState{opts: encodeOpts{
		ctx:         todoCtx,
		timeLayout:  defaultTimeLayout,
		integerBase: defaultBase,
	}}
}

func (s *encodeState) reset() {
	// The fields addressable and ptrInput
	// are always set prior to encoding
	// so they don't need to be reset.
	s.firstField = false
	s.depthLevel = 0

	s.opts.reset()
}

func (opts *encodeOpts) reset() {
	*opts = *zeroOpts
}

func (opts *encodeOpts) check() error {
	if opts.ctx == nil {
		return fmt.Errorf("nil context")
	}
	if opts.timeLayout == "" {
		return fmt.Errorf("empty time layout")
	}
	if opts.integerBase < 2 || opts.integerBase > 36 {
		return fmt.Errorf("illegal base: %d", opts.integerBase)
	}
	if opts.durationFmt < DurationString || opts.durationFmt > DurationNanoseconds {
		return fmt.Errorf("unknown duration format")
	}
	return nil
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

type marshalerKind string

const (
	marshalerKindJSON        marshalerKind = "MarshalJSON"
	marshalerKindText        marshalerKind = "MarshalText"
	marshalerKindJettison    marshalerKind = "WriteJSON"
	marshalerKindJettisonCtx marshalerKind = "WriteJSONContext"
)

// MarshalerError represents an error from calling
// a MarshalJSON or MarshalText method.
type MarshalerError struct {
	Err  error
	Typ  reflect.Type
	kind marshalerKind
}

// Error implements the builtin error interface.
func (e *MarshalerError) Error() string {
	return fmt.Sprintf("error calling %s for type %s: %s", e.kind, e.Typ.String(), e.Err)
}

// Unwrap returns the wrapped error.
// This doesn't implement a public interface, but
// allow to use the errors.Unwrap function released
// in Go 1.13 with a MarshalerError.
func (e *MarshalerError) Unwrap() error { return e.Err }

// NewEncoder returns a new encoder that can marshal the
// values of the given type. The Encoder can be explicitly
// initialized by calling its Compile method, otherwise the
// operation is done on first call to Encode.
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
		err = e.build(e.typ)
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
		// The exception for the struct type comes from the
		// fact that the pointer may points to an anonymous
		// struct field that should still be serialized as
		// part of the struct, or has the omitempty option.
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
	es.ptrInput = isPtr

	// Apply options to state.
	for _, o := range opts {
		if o != nil {
			o(&es.opts)
		}
	}
	if err := es.opts.check(); err != nil {
		return fmt.Errorf("invalid option: %v", err)
	}
	// Execute the instruction with the state
	// and the given writer.
	if err := e.root(p, w, es); err != nil {
		return err
	}
	statePool.Put(es)
	return nil
}

// build generates the instructions-set required to encode
// the given type. It returns an error if the type is not
// supported, such as channel, complex and function values.
func (e *Encoder) build(t reflect.Type) error {
	ins, err := cachedTypeInstr(t)
	if err != nil {
		return err
	}
	e.root = ins
	return nil
}
