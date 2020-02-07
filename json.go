package jettison

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
)

// AppendMarshaler is a variant of the json.Marshaler
// interface, implemented by types that can append a
// valid and compact JSON representation of themselves
// to a buffer. If a type implements both interfaces,
// this one will be used in priority by the package.
type AppendMarshaler interface {
	AppendJSON([]byte) ([]byte, error)
}

// AppendMarshalerCtx is similar to AppendMarshaler,
// but the method implemented also takes a context.
// The use case for this interface is to dynamically
// control the marshaling of the type implementing it
// through the values encapsulated by the context,
// that may be provided at runtime using WithContext.
type AppendMarshalerCtx interface {
	AppendJSONContext(context.Context, []byte) ([]byte, error)
}

const (
	marshalerJSON          = "MarshalJSON"
	marshalerText          = "MarshalText"
	marshalerAppendJSONCtx = "AppendJSONContext"
	marshalerAppendJSON    = "AppendJSON"
)

// MarshalerError represents an error from calling
// the methods MarshalJSON or MarshalText.
type MarshalerError struct {
	Type     reflect.Type
	Err      error
	funcName string
}

// Error implements the builtin error interface.
func (e *MarshalerError) Error() string {
	return fmt.Sprintf("json: error calling %s for type %s: %s",
		e.funcName, e.Type, e.Err.Error())
}

// Unwrap returns the error wrapped by e.
// This doesn't implement a public interface, but
// allow to use the errors.Unwrap function released
// in Go1.13 with a MarshalerError.
func (e *MarshalerError) Unwrap() error {
	return e.Err
}

// UnsupportedTypeError is the error returned
// by Marshal when attempting to encode an
// unsupported value type.
type UnsupportedTypeError struct {
	Type reflect.Type
}

// Error implements the bultin error interface.
func (e *UnsupportedTypeError) Error() string {
	return fmt.Sprintf("json: unsupported type: %s", e.Type)
}

// UnsupportedValueError is the error returned
// by Marshal when attempting to encode an
// unsupported value.
type UnsupportedValueError struct {
	Value reflect.Value
	Str   string
}

// Error implements the builtin error interface.
func (e *UnsupportedValueError) Error() string {
	return fmt.Sprintf("json: unsupported value: %s", e.Str)
}

// A SyntaxError is a description of a JSON syntax error.
// Unlike its equivalent in the encoding/json package, the
// Error method implemented does not return a meaningful
// message, and the Offset field is always zero.
// It is present merely for consistency.
type SyntaxError struct {
	msg    string
	Offset int64
}

// Error implements the builtin error interface.
func (e *SyntaxError) Error() string { return e.msg }

// InvalidOptionError is the error returned by
// MarshalOpts when one of the given options is
// invalid.
type InvalidOptionError struct {
	Err error
}

// Error implements the builtin error interface.
func (e *InvalidOptionError) Error() string {
	return fmt.Sprintf("json: invalid option: %s", e.Err.Error())
}

// Marshal returns the JSON encoding of v.
// The full documentation can be found at
// https://golang.org/pkg/encoding/json/#Marshal.
func Marshal(v interface{}) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	return marshalJSON(v, defaultEncOpts())
}

// Append is similar to Marshal but appends the JSON
// representation of v to dst instead of returning a
// new allocated slice.
func Append(dst []byte, v interface{}) ([]byte, error) {
	if v == nil {
		return append(dst, "null"...), nil
	}
	return appendJSON(dst, v, defaultEncOpts())
}

// MarshalOpts is similar to Marshal, but also accepts
// a list of options to configure the encoding behavior.
func MarshalOpts(v interface{}, opts ...Option) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	eo := defaultEncOpts()

	if len(opts) != 0 {
		(&eo).apply(opts...)
		if err := eo.validate(); err != nil {
			return nil, &InvalidOptionError{err}
		}
	}
	return marshalJSON(v, eo)
}

// AppendOpts is similar to Append, but also accepts
// a list of options to configure the encoding behavior.
func AppendOpts(dst []byte, v interface{}, opts ...Option) ([]byte, error) {
	if v == nil {
		return append(dst, "null"...), nil
	}
	eo := defaultEncOpts()

	if len(opts) != 0 {
		(&eo).apply(opts...)
		if err := eo.validate(); err != nil {
			return nil, &InvalidOptionError{err}
		}
	}
	return appendJSON(dst, v, eo)
}

func marshalJSON(v interface{}, opts encOpts) ([]byte, error) {
	ins := cachedInstr(reflect.TypeOf(v))
	buf := cachedBuffer()

	var err error
	buf.B, err = ins(unpackEface(v).word, buf.B, opts)

	// Ensure that v is reachable until
	// the instruction has returned.
	runtime.KeepAlive(v)

	var b []byte
	if err == nil {
		// Make a copy of the buffer's content
		// before its returned to the pool.
		b = make([]byte, len(buf.B))
		copy(b, buf.B)
	}
	bufferPool.Put(buf)

	return b, err
}

func appendJSON(dst []byte, v interface{}, opts encOpts) ([]byte, error) {
	ins := cachedInstr(reflect.TypeOf(v))
	var err error
	dst, err = ins(unpackEface(v).word, dst, opts)
	runtime.KeepAlive(v)

	return dst, err
}
