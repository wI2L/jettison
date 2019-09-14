package jettison

import (
	"bytes"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

// TestMarshal tests that Marshal can be called
// concurrently for the same input value.
func TestMarshal(t *testing.T) {
	t.Parallel()

	type x struct {
		A string
		B int
	}
	tox := reflect.TypeOf(x{})
	if _, ok := encoderCache.Load(tox); ok {
		t.Errorf("expected value to not exist")
	}
	xx := x{
		A: "Loreum",
		B: 42,
	}
	t.Run("nil", func(t *testing.T) {
		b, err := Marshal(nil)
		if err != nil {
			t.Error(err)
		}
		const want = "null"
		if s := string(b); s != want {
			t.Errorf("got %#q, want %#q", s, want)
		}
	})
	t.Run("parallel", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				t.Parallel()

				b, err := Marshal(xx)
				if err != nil {
					t.Fatal(err)
				}
				const want = `{"A":"Loreum","B":42}`
				if s := string(b); s != want {
					t.Errorf("got %#q, want %#q", s, want)
				}
			})
		}
	})
}

// TestMarshalTo tests that MarshalTo can be called
// concurrently for the same input value.
func TestMarshalTo(t *testing.T) {
	t.Parallel()

	type x struct {
		A string
		B int
	}
	tox := reflect.TypeOf(x{})
	if _, ok := encoderCache.Load(tox); ok {
		t.Errorf("expected value to not exist")
	}
	xx := x{
		A: "Loreum",
		B: 42,
	}
	t.Run("nil input", func(t *testing.T) {
		var buf bytes.Buffer
		if err := MarshalTo(nil, &buf); err != nil {
			t.Error(err)
		}
		const want = "null"
		if s := buf.String(); s != want {
			t.Errorf("got %#q, want %#q", s, want)
		}
	})
	t.Run("invalid writer", func(t *testing.T) {
		err := MarshalTo(int(42), nil)
		if err != ErrInvalidWriter {
			t.Errorf("got %s, want ErrInvalidWriter", err)
		}
	})
	t.Run("parallel", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				t.Parallel()
				var buf bytes.Buffer

				if err := MarshalTo(xx, &buf); err != nil {
					t.Fatal(err)
				}
				const want = `{"A":"Loreum","B":42}`
				if s := buf.String(); s != want {
					t.Errorf("got %#q, want %#q", s, want)
				}
			})
		}
	})
}

func TestRegister(t *testing.T) {
	t.Parallel()

	type x struct {
		S string
	}
	typ := reflect.TypeOf(x{})

	// Register a new type.
	// Ensure that the encoder can be found
	// in the cache afterward.
	if err := Register(typ); err != nil {
		t.Fatal(err)
	}
	v, ok := encoderCache.Load(typ)
	if !ok {
		t.Fatalf("encoder not found in cache")
	}
	ce := v.(*cachedEncoder)
	if ce.enc == nil {
		t.Fatal("nil encoder found in cache")
	}
	if ce.enc.typ != typ {
		t.Fatalf("encoder's type mismatch, got %v, want %v", ce.enc.typ, typ)
	}
	// Register again the same type.
	// We should find the same encoder.
	if err := Register(typ); err != nil {
		t.Fatal(err)
	}
	v, ok = encoderCache.Load(typ)
	if !ok {
		t.Fatalf("encoder not found in cache")
	}
	ce2 := v.(*cachedEncoder)
	if ce2 == nil || ce2.enc != ce.enc {
		t.Errorf("encoder has been recreated")
	}
}

func TestCacheEncoder(t *testing.T) {
	t.Parallel()

	var (
		mu = sync.Mutex{}
		wg = sync.WaitGroup{}
		cd = sync.NewCond(&mu)
	)
	typ := reflect.TypeOf(timeType)
	esl := make([]*Encoder, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cd.L.Lock()
			cd.Wait()
			enc, err := cacheEncoder(typ)
			if err != nil {
				t.Error(err)
			}
			esl[idx] = enc
			cd.L.Unlock()
		}(i)
	}
	time.Sleep(100 * time.Millisecond)

	// Notify all goroutines to wake up.
	cd.L.Lock()
	cd.Broadcast()
	cd.L.Unlock()

	wg.Wait()

	// Ensure that all pointers points to the
	// same encoder instance.
	for i := 0; i < len(esl); i++ {
		if i != len(esl)-1 {
			if esl[i] != esl[i+1] {
				t.Errorf("encoders %d and %d are not equal", i, i+1)
			}
		}
	}
}
