package jettison

import (
	"bytes"
	"reflect"
	"sync"
)

var encoderCache sync.Map // map[reflect.Type]*cachedEncoder

// Marshaler is a variant of the json.Marshaler
// interface, implemented by types that can write
// a valid JSON representation of themselves to
// a Writer. If a type implements both interfaces,
// Jettison will use its own interface in priority.
type Marshaler interface {
	WriteJSON(Writer) error
}

// Register records a new compiled encoder for the given
// type to the cache used by the global functions Marshal
// and MarshalTo. This may be used during the initialization
// of a program to speed up the first calls to previously
// mentioned functions.
func Register(typ reflect.Type) error {
	if _, ok := encoderCache.Load(typ); ok {
		return nil
	}
	enc, err := NewEncoder(typ)
	if err != nil {
		return err
	}
	_, _ = encoderCache.LoadOrStore(typ, &cachedEncoder{
		wg:  sync.WaitGroup{},
		enc: enc,
	})
	return nil
}

// Marshal returns the JSON encoding of v.
// It uses a pre-compiled encoder from the
// package's cache, or create and store one
// on the fly.
func Marshal(v interface{}) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	typ := reflect.TypeOf(v)
	enc, err := getCachedEncoder(typ)
	if err != nil {
		return nil, err
	}
	buf := bpool.Get().(*bytes.Buffer)
	defer bpool.Put(buf)
	buf.Reset()

	if err := enc.encode(typ, v, buf); err != nil {
		return nil, err
	}
	b := append([]byte(nil), buf.Bytes()...)
	return b, nil
}

// MarshalTo writes the JSON encoding of v to w.
// It uses a pre-compiled encoder from the
// package's cache, or create and store one
// on the fly.
func MarshalTo(v interface{}, w Writer) error {
	if w == nil {
		return ErrInvalidWriter
	}
	if v == nil {
		_, err := w.WriteString("null")
		return err
	}
	typ := reflect.TypeOf(v)
	enc, err := getCachedEncoder(typ)
	if err != nil {
		return err
	}
	return enc.encode(typ, v, w)
}

type cachedEncoder struct {
	wg  sync.WaitGroup
	enc *Encoder
	err error
}

// getCachedEncoder returns a suitable encoder for
// the type of i. If the encoder does not exists in
// the cache, a new one is created, making sure that
// only one is created at a time. If the function is
// called concurently for the same type, the caller
// waits for the original to complete and receives
// the encoder that was created by the first call.
func getCachedEncoder(typ reflect.Type) (*Encoder, error) {
	v, exists := encoderCache.Load(typ)
	if exists {
		ce := v.(*cachedEncoder)
		ce.wg.Wait()
		return ce.enc, ce.err
	}
	return cacheEncoder(typ)
}

func cacheEncoder(typ reflect.Type) (*Encoder, error) {
	ce := new(cachedEncoder)
	ce.wg.Add(1)

	v, loaded := encoderCache.LoadOrStore(typ, ce)
	if loaded {
		// Another goroutine stored an encoder
		// for this type, so we wait for it to
		// be ready to use and return it.
		ce := v.(*cachedEncoder)
		ce.wg.Wait()
		return ce.enc, ce.err
	}
	ce.enc, ce.err = &Encoder{typ: typ}, nil
	ce.err = ce.enc.compile()
	ce.wg.Done()

	return ce.enc, ce.err
}
