package jettison

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"reflect"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"
)

type (
	jmr string
	jmv []string
)

func (*jmr) MarshalJSON() ([]byte, error) { return []byte(`"XYZ"`), nil }
func (jmv) MarshalJSON() ([]byte, error)  { return []byte(`"ZYX"`), nil }

type (
	mapss struct {
		M map[string]string
	}
	inner struct {
		M map[string]string
	}
	outer struct {
		M map[string]inner
	}
	z struct {
		S []byte
	}
	y struct {
		P float64 `json:"p,omitempty"`
		Q uint64  `json:"q,omitempty"`
		R uint8
	}
	x struct {
		A  *string `json:"a,string"`
		B1 int64   `json:"b1,string"`
		B2 uint16  `json:"b2"`
		C  *bool   `json:"c,string"`
		D  float32
		E1 *[]int
		E2 []string
		E3 []jmr
		F1 [4]string
		F2 [1]jmr
		F3 *[1]jmr
		G1 map[int]*string
		G2 map[string]*map[string]string
		G3 map[int]map[string]map[int]string
		G4 map[string]mapss
		G5 outer
		G6 map[int]jmr
		G7 map[int]*jmr
		G8 map[int]bool `json:",omitempty"`
		H1 jmr
		H2 *jmr
		H3 jmv
		H4 *jmv
		I  time.Time
		J  time.Duration
		K  json.Number
		L  json.RawMessage
		M1 interface{}
		M2 interface{}
		N  struct{}
		X  *x
		*y
		z `json:"z"`
	}
)

var (
	s  = "Loreum"
	b  = true
	m  = map[string]string{"b": "c"}
	xx = x{
		A:  &s,
		B1: -42,
		B2: 42,
		C:  &b,
		D:  math.MaxFloat32,
		E1: &[]int{1, 2, 3},
		E2: []string{"x", "y", "z"},
		E3: []jmr{"1"},
		F1: [4]string{"a", "b", "c", "d"},
		F2: [1]jmr{"1"},
		F3: &[1]jmr{"1"},
		G1: map[int]*string{2: &s, 3: new(string)},
		G2: map[string]*map[string]string{"a": &m},
		G3: map[int]map[string]map[int]string{1: {"a": {2: "b"}}},
		G4: map[string]mapss{"1": {M: map[string]string{"2": "3"}}},
		G5: outer{map[string]inner{"outer": {map[string]string{"key": "val"}}}},
		G6: map[int]jmr{1: "jmr"},
		G7: map[int]*jmr{1: new(jmr)},
		G8: map[int]bool{},
		H1: "jmp",
		H2: nil,
		H3: nil,
		H4: nil,
		I:  time.Now(),
		J:  3 * time.Minute,
		K:  "3.14",
		L:  []byte(`{ "a":"b" }`),
		M1: uint32(255),
		M2: &s,
		X:  &x{H1: "jmv"},
		y:  &y{R: math.MaxUint8},
		z:  z{S: []byte("Loreum")},
	}
)

// marshalCompare compares the JSON encoding
// of v between Jettison and encoding/json.
func marshalCompare(t *testing.T, v interface{}, name string) {
	jb1, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	jb2, err := MarshalOpts(v)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(jb1, jb2) {
		t.Error("non-equal outputs for Marshal and MarshalOpts")
	}
	jb3, err := Append([]byte(nil), v)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(jb1, jb3) {
		t.Error("non-equal outputs for Marshal and Append")
	}
	sb, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("standard: %s", string(sb))
	t.Logf("jettison: %s", string(jb1))

	if !bytes.Equal(jb1, sb) {
		t.Errorf("%s: non-equal outputs", name)
	}
}

func marshalCompareError(t *testing.T, v interface{}, name string) {
	_, errj := Marshal(v)
	if errj == nil {
		t.Fatalf("expected non-nil error")
	}
	_, errs := json.Marshal(v)
	if errs == nil {
		t.Fatalf("expected non-nil error")
	}
	t.Logf("standard: %s", errs)
	t.Logf("jettison: %s", errj)

	if errs.Error() != errj.Error() {
		t.Errorf("%s: non-equal outputs", name)
	}
}

func TestAll(t *testing.T) {
	marshalCompare(t, nil, "nil")
	marshalCompare(t, xx, "non-pointer")
	marshalCompare(t, &xx, "pointer")
}

func TestInvalidEncodeOpts(t *testing.T) {
	for _, opt := range []Option{
		TimeLayout(""),
		DurationFormat(DurationFmt(-1)),
		DurationFormat(DurationFmt(6)),
		WithContext(nil), // nolint:staticcheck
	} {
		_, err1 := MarshalOpts(struct{}{}, opt)
		_, err2 := AppendOpts([]byte(nil), struct{}{}, opt)

		for _, err := range []error{err1, err2} {
			if err != nil {
				e, ok := err.(*InvalidOptionError)
				if !ok {
					t.Errorf("got %T, want InvalidOptionError", err)
				}
				if e.Error() == "" {
					t.Errorf("expected non-empty error message")
				}
			} else {
				t.Error("expected non-nil error")
			}
		}
	}
}

// TestBasicTypes tests the marshaling of basic types.
func TestBasicTypes(t *testing.T) {
	testdata := []interface{}{
		true,
		false,
		"Loreum",
		int8(math.MaxInt8),
		int16(math.MaxInt16),
		int32(math.MaxInt32),
		int64(math.MaxInt64),
		uint8(math.MaxUint8),
		uint16(math.MaxUint16),
		uint32(math.MaxUint32),
		uint64(math.MaxUint64),
		uintptr(0xBEEF),
		(*bool)(nil),
		(*int)(nil),
		(*string)(nil),
	}
	for _, v := range testdata {
		marshalCompare(t, v, "")
	}
}

// TestCompositeTypes tests the marshaling of composite types.
func TestCompositeTypes(t *testing.T) {
	var (
		jmref = jmr("jmr")
		jmval = jmv([]string{"a", "b", "c"})
	)
	testdata := []interface{}{
		[]uint{},
		[]int{1, 2, 3},
		[]int(nil),
		(*[]int)(nil),
		[]string{"a", "b", "c"},
		[2]bool{true, false},
		(*[4]string)(nil),
		map[string]int{"a": 1, "b": 2},
		&map[int]string{1: "a", 2: "b"},
		(map[string]int)(nil),
		time.Now(),
		3*time.Minute + 35*time.Second,
		jmref,
		&jmref,
		jmval,
		&jmval,
	}
	for _, v := range testdata {
		marshalCompare(t, v, "")
	}
}

// TestUnsupportedTypes tests that marshaling an
// unsupported type such as channel, complex, and
// function value returns an UnsupportedTypeError.
// The error message is compared with the one that
// is returned by json.Marshal.
func TestUnsupportedTypes(t *testing.T) {
	testdata := []interface{}{
		make(chan int),
		func() {},
		complex64(0),
		complex128(0),
		make([]chan int, 1),
		[1]complex64{},
		&[1]complex128{},
		map[int]chan bool{1: make(chan bool)},
		struct{ F func() }{func() {}},
		&struct{ C complex64 }{0},
	}
	for _, v := range testdata {
		marshalCompareError(t, v, "")
	}
}

// TestInvalidFloatValues tests that encoding an
// invalid float value returns UnsupportedValueError.
func TestInvalidFloatValues(t *testing.T) {
	for _, v := range []float64{
		math.NaN(),
		math.Inf(-1),
		math.Inf(1),
	} {
		_, err := Marshal(v)
		if err != nil {
			if _, ok := err.(*UnsupportedValueError); !ok {
				t.Errorf("got %T, want UnsupportedValueError", err)
			}
		} else {
			t.Error("got nil, want non-nil error")
		}
		// Error message must be the same as
		// the one of the standard library.
		marshalCompareError(t, v, "")
	}
}

// TestJSONNumber tests that a json.Number literal value
// can be marshaled, and that an error is returned if it
// isn't a valid number according to the JSON grammar.
func TestJSONNumber(t *testing.T) {
	valid := []json.Number{
		"42",
		"-42",
		"24.42",
		"-666.66",
		"3.14",
		"-3.14",
		"1e3",
		"1E-6",
		"1E+42",
		// Special case to keep backward
		// compatibility with Go1.5, that
		// encodes the empty string as "0".
		"",
	}
	for _, v := range valid {
		marshalCompare(t, v, "valid")
	}
	invalid := []json.Number{
		"1E+4.0",
		"084",
		"-03.14",
		"-",
		"invalid",
	}
	for _, v := range invalid {
		marshalCompareError(t, v, "invalid")
	}
}

func TestInvalidTime(t *testing.T) {
	// Special case to test error when the year
	// of the date is outside of range [0.9999].
	// see golang.org/issue/4556#c15.
	for _, tm := range []time.Time{
		time.Date(-1, time.January, 1, 0, 0, 0, 0, time.UTC),
		time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC),
	} {
		_, err := Marshal(tm)
		if err != nil {
			want := "time: year outside of range [0,9999]"
			if err.Error() != want {
				t.Errorf("got %q, want %q", err.Error(), want)
			}
		} else {
			t.Error("got nil, want non-nil error")
		}
	}
}

// TestRenamedByteSlice tests that a name type
// that represents a slice of bytes is marshaled
// the same way as a regular byte slice.
func TestRenamedByteSlice(t *testing.T) {
	type (
		b  byte
		b1 []byte
		b2 []b
	)
	testdata := []interface{}{
		b1("byte slice 1"),
		b2("byte slice 2"),
	}
	for _, v := range testdata {
		marshalCompare(t, v, "")
	}
}

func TestByteSliceSizes(t *testing.T) {
	makeSlice := func(size int) []byte {
		b := make([]byte, size)
		if _, err := rand.Read(b); err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, v := range []interface{}{
		makeSlice(0),
		makeSlice(1024),
		makeSlice(2048),
		makeSlice(4096),
		makeSlice(8192),
	} {
		marshalCompare(t, v, "")
	}
}

// TestSortedSyncMap tests the marshaling
// of a sorted sync.Map value.
func TestSortedSyncMap(t *testing.T) {
	var sm sync.Map

	sm.Store(1, "one")
	sm.Store("a", 42)
	sm.Store("b", false)
	sm.Store(mkvstrMarshaler("c"), -42)
	sm.Store(mkrstrMarshaler("d"), true)
	sm.Store(mkvintMarshaler(42), 1)
	sm.Store(mkrintMarshaler(42), 2)

	b, err := Marshal(&sm)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"1":"one","42":2,"MKVINT":1,"a":42,"b":false,"c":-42,"d":true}`

	if !bytes.Equal(b, []byte(want)) {
		t.Errorf("got %#q, want %#q", b, want)
	}
}

// TestUnsortedSyncMap tests the marshaling
// of an unsorted sync.Map value.
func TestUnsortedSyncMap(t *testing.T) {
	// entries maps each interface k/v
	// pair to the string representation
	// of the key in payload.
	entries := map[string]struct {
		key interface{}
		val interface{}
	}{
		"1":      {1, "one"},
		"a":      {"a", 42},
		"b":      {"b", false},
		"c":      {mkvstrMarshaler("c"), -42},
		"d":      {mkrstrMarshaler("d"), true},
		"MKVINT": {mkvintMarshaler(42), 1},
		"42":     {mkrintMarshaler(42), 2},
	}
	var sm sync.Map
	for _, e := range entries {
		sm.Store(e.key, e.val)
	}
	bts, err := MarshalOpts(&sm, UnsortedMap())
	if err != nil {
		t.Fatal(err)
	}
	m := make(map[string]interface{})
	if err := json.Unmarshal(bts, &m); err != nil {
		t.Fatal(err)
	}
	// Unmarshaled map must contains exactly the
	// number of entries added to the sync map.
	if g, w := len(m), len(entries); g != w {
		t.Errorf("invalid lengths: got %d, want %d", g, w)
	}
	for k, v := range m {
		// Compare the marshaled representation
		// of each value to avoid false-positive
		// between integer and float types.
		b1, err1 := json.Marshal(v)
		b2, err2 := json.Marshal(entries[k].val)
		if err1 != nil {
			t.Fatal(err)
		}
		if err2 != nil {
			t.Fatal(err2)
		}
		if !bytes.Equal(b1, b2) {
			t.Errorf("for key %s: got %v, want %v", k, b1, b2)
		}
	}
}

// TestInvalidSyncMapKeys tests that marshaling a
// sync.Map with unsupported key types returns an
// error.
func TestInvalidSyncMapKeys(t *testing.T) {
	testInvalidSyncMapKeys(t, true)
	testInvalidSyncMapKeys(t, false)
}

func testInvalidSyncMapKeys(t *testing.T, sorted bool) {
	for _, f := range []func(sm *sync.Map){
		func(sm *sync.Map) { sm.Store(false, nil) },
		func(sm *sync.Map) { sm.Store(new(int), nil) },
		func(sm *sync.Map) { sm.Store(nil, nil) },
	} {
		var (
			sm  sync.Map
			err error
		)
		f(&sm) // add entries to sm
		if sorted {
			_, err = Marshal(&sm)
		} else {
			_, err = MarshalOpts(&sm, UnsortedMap())
		}
		if err == nil {
			t.Error("expected a non-nil error")
		}
	}
}

// TestCompositeMapValue tests the marshaling
// of maps with composite values.
func TestCompositeMapValue(t *testing.T) {
	type x struct {
		A string `json:"a"`
		B int    `json:"b"`
		C bool   `json:"c"`
	}
	type y []uint32

	for _, v := range []interface{}{
		map[string]x{
			"1": {A: "A", B: 42, C: true},
			"2": {A: "A", B: 84, C: false},
		},
		map[string]y{
			"3": {7, 8, 9},
			"2": {4, 5, 6},
			"1": nil,
		},
		map[string]*x{
			"b": {A: "A", B: 128, C: true},
			"a": nil,
			"c": {},
		},
		map[string]interface{}{
			"1": 42,
			"2": "two",
			"3": nil,
			"4": (*int64)(nil),
			"5": x{A: "A"},
			"6": &x{A: "A", B: 256, C: true},
		},
	} {
		marshalCompare(t, v, "")
	}
}

type (
	mkstr           string
	mkint           int64
	mkvstrMarshaler string
	mkrstrMarshaler string
	mkvintMarshaler uint64
	mkrintMarshaler int
	mkvcmpMarshaler struct{}
)

func (mkvstrMarshaler) MarshalText() ([]byte, error)  { return []byte("MKVSTR"), nil }
func (*mkrstrMarshaler) MarshalText() ([]byte, error) { return []byte("MKRSTR"), nil }
func (mkvintMarshaler) MarshalText() ([]byte, error)  { return []byte("MKVINT"), nil }
func (*mkrintMarshaler) MarshalText() ([]byte, error) { return []byte("MKRINT"), nil }
func (mkvcmpMarshaler) MarshalText() ([]byte, error)  { return []byte("MKVCMP"), nil }

// TestMapKeyPrecedence tests that the precedence
// order of map key types is respected during marshaling.
func TestMapKeyPrecedence(t *testing.T) {
	testdata := []interface{}{
		map[mkstr]string{"K": "V"},
		map[mkint]string{1: "V"},
		map[mkvstrMarshaler]string{"K": "V"},
		map[mkrstrMarshaler]string{"K": "V"},
		map[mkvintMarshaler]string{42: "V"},
		map[mkrintMarshaler]string{1: "one"},
		map[mkvcmpMarshaler]string{{}: "V"},
	}
	for _, v := range testdata {
		marshalCompare(t, v, "")
	}
}

// TestJSONMarshaler tests that a type implementing the
// json.Marshaler interface is marshaled using the result
// of its MarshalJSON method call result.
// Because the types big.Int and time.Time also implements
// the encoding.TextMarshaler interface, the test ensures
// that MarshalJSON has priority.
func TestJSONMarshaler(t *testing.T) {
	type x struct {
		T1 time.Time  `json:""`
		T2 time.Time  `json:",omitempty"`
		T3 *time.Time `json:""`
		T4 *time.Time `json:""`           // nil
		T5 *time.Time `json:",omitempty"` // nil
		S1 bvjm       `json:",omitempty"`
		S2 bvjm       `json:",omitempty"`
		S3 bvjm       `json:""`
		S4 *bvjm      `json:""`
		S5 *bvjm      `json:""`           // nil
		S6 *bvjm      `json:",omitempty"` // nil
		I1 big.Int    `json:""`
		I2 big.Int    `json:",omitempty"`
		I3 *big.Int   `json:""`
		I4 *big.Int   `json:""`           // nil
		I5 *big.Int   `json:",omitempty"` // nil
		P1 brjm       `json:",omitempty"`
		P2 brjm       `json:",omitempty"`
		P3 brjm       `json:""`
		P4 *brjm      `json:""`
		P5 *brjm      `json:""`           // nil
		P6 *brjm      `json:",omitempty"` // nil

		// NOTE
		// time.Time = Non-pointer receiver of composite type.
		// bvjm = Non-pointer receiver of basic type.
		// big.Int = Pointer receiver of composite type.
		// brjm = Pointer receiver of basic type.
	}
	var (
		now  = time.Now()
		bval = bvjm("bval")
		bref = brjm("bref")
		xx   = x{
			T1: now,
			T3: &now,
			S1: "S1",
			S4: &bval,
			I1: *big.NewInt(math.MaxInt64),
			I3: big.NewInt(math.MaxInt64),
			P1: "P1",
			P4: &bref,
		}
	)
	marshalCompare(t, xx, "non-pointer")
	marshalCompare(t, &xx, "pointer")
}

// TestTextMarshaler tests that a type implementing
// the encoding.TextMarshaler interface encodes to a
// quoted string of its MashalText method result.
func TestTextMarshaler(t *testing.T) {
	type x struct {
		S1 net.IP     `json:""`
		S2 net.IP     `json:",omitempty"`
		S3 *net.IP    `json:""`
		S4 *net.IP    `json:""`           // nil
		S5 *net.IP    `json:",omitempty"` // nil
		I1 bvtm       `json:",omitempty"`
		I2 bvtm       `json:",omitempty"`
		I3 bvtm       `json:""`
		I4 *bvtm      `json:""`
		I5 *bvtm      `json:""`           // nil
		I6 *bvtm      `json:",omitempty"` // nil
		F1 big.Float  `json:""`
		F2 big.Float  `json:",omitempty"`
		F3 *big.Float `json:""`
		F4 *big.Float `json:""`           // nil
		F5 *big.Float `json:",omitempty"` // nil
		P1 brtm       `json:",omitempty"`
		P2 brtm       `json:",omitempty"`
		P3 brtm       `json:""`
		P4 *brtm      `json:""`
		P5 *brtm      `json:""`           // nil
		P6 *brtm      `json:",omitempty"` // nil

		// NOTE
		// net.IP = Non-pointer receiver of composite type.
		// bvtm = Non-pointer receiver of basic type.
		// big.Float = Pointer receiver of composite type.
		// brtm = Pointer receiver of basic type.
	}
	var (
		bval = bvtm(42)
		bref = brtm(42)
		xx   = x{
			S1: net.IP{192, 168, 0, 1},
			S3: &net.IP{127, 0, 0, 1},
			I1: 42,
			I4: &bval,
			F1: *big.NewFloat(math.MaxFloat64),
			F3: big.NewFloat(math.MaxFloat64),
			P1: 42,
			P4: &bref,
		}
	)
	marshalCompare(t, xx, "non-pointer")
	marshalCompare(t, &xx, "pointer")
}

type (
	bvm string
	brm string
	cvm struct{}
	crm struct{}
)

func (m bvm) AppendJSON(dst []byte) ([]byte, error) {
	return append(dst, strconv.Quote(string(m))...), nil
}
func (m *brm) AppendJSON(dst []byte) ([]byte, error) {
	return append(dst, strconv.Quote(string(*m))...), nil
}
func (m bvm) MarshalJSON() ([]byte, error)         { return []byte(strconv.Quote(string(m))), nil }
func (m *brm) MarshalJSON() ([]byte, error)        { return []byte(strconv.Quote(string(*m))), nil }
func (cvm) AppendJSON(dst []byte) ([]byte, error)  { return append(dst, `"X"`...), nil }
func (cvm) MarshalJSON() ([]byte, error)           { return []byte(`"X"`), nil }
func (*crm) AppendJSON(dst []byte) ([]byte, error) { return append(dst, `"Y"`...), nil }
func (*crm) MarshalJSON() ([]byte, error)          { return []byte(`"Y"`), nil }

//nolint:dupl
func TestMarshaler(t *testing.T) {
	type x struct {
		S1 cvm  `json:""`
		S2 cvm  `json:",omitempty"`
		S3 *cvm `json:""`
		S4 *cvm `json:""`           // nil
		S5 *cvm `json:",omitempty"` // nil
		I1 bvm  `json:",omitempty"`
		I2 bvm  `json:",omitempty"`
		I3 bvm  `json:""`
		I4 *bvm `json:""`
		I5 *bvm `json:""`           // nil
		I6 *bvm `json:",omitempty"` // nil
		F1 crm  `json:""`
		F2 crm  `json:",omitempty"`
		F3 *crm `json:""`
		F4 *crm `json:""`           // nil
		F5 *crm `json:",omitempty"` // nil
		P1 brm  `json:",omitempty"`
		P2 brm  `json:",omitempty"`
		P3 brm  `json:""`
		P4 *brm `json:""`
		P5 *brm `json:""`           // nil
		P6 *brm `json:",omitempty"` // nil

		// NOTE
		// cvm = Non-pointer receiver of composite type.
		// bvm = Non-pointer receiver of basic type.
		// crm = Pointer receiver of composite type.
		// brm = Pointer receiver of basic type.
	}
	var (
		bval = bvm("bval")
		bref = brm("bref")
		xx   = x{
			S1: cvm{},
			S3: &cvm{},
			I1: "I1",
			I4: &bval,
			F1: crm{},
			F3: &crm{},
			P1: "P1",
			P4: &bref,
		}
	)
	marshalCompare(t, xx, "non-pointer")
	marshalCompare(t, &xx, "pointer")
}

type (
	bvmctx string
	brmctx string
	cvmctx struct{}
	crmctx struct{}
)

func (m bvmctx) AppendJSONContext(_ context.Context, dst []byte) ([]byte, error) {
	return append(dst, strconv.Quote(string(m))...), nil
}
func (m bvmctx) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(string(m))), nil
}
func (m *brmctx) AppendJSONContext(_ context.Context, dst []byte) ([]byte, error) {
	return append(dst, strconv.Quote(string(*m))...), nil
}
func (m *brmctx) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(string(*m))), nil
}
func (cvmctx) AppendJSONContext(_ context.Context, dst []byte) ([]byte, error) {
	return append(dst, `"X"`...), nil
}
func (cvmctx) MarshalJSON() ([]byte, error) {
	return []byte(`"X"`), nil
}
func (*crmctx) AppendJSONContext(_ context.Context, dst []byte) ([]byte, error) {
	return append(dst, `"Y"`...), nil
}
func (*crmctx) MarshalJSON() ([]byte, error) {
	return []byte(`"Y"`), nil
}

//nolint:dupl
func TestMarshalerCtx(t *testing.T) {
	type x struct {
		S1 cvmctx  `json:""`
		S2 cvmctx  `json:",omitempty"`
		S3 *cvmctx `json:""`
		S4 *cvmctx `json:""`           // nil
		S5 *cvmctx `json:",omitempty"` // nil
		I1 bvmctx  `json:",omitempty"`
		I2 bvmctx  `json:",omitempty"`
		I3 bvmctx  `json:""`
		I4 *bvmctx `json:""`
		I5 *bvmctx `json:""`           // nil
		I6 *bvmctx `json:",omitempty"` // nil
		F1 crmctx  `json:""`
		F2 crmctx  `json:",omitempty"`
		F3 *crmctx `json:""`
		F4 *crmctx `json:""`           // nil
		F5 *crmctx `json:",omitempty"` // nil
		P1 brmctx  `json:",omitempty"`
		P2 brmctx  `json:",omitempty"`
		P3 brmctx  `json:""`
		P4 *brmctx `json:""`
		P5 *brmctx `json:""`           // nil
		P6 *brmctx `json:",omitempty"` // nil

		// NOTE
		// cvmctx = Non-pointer receiver of composite type.
		// bvmctx = Non-pointer receiver of basic type.
		// crmctx = Pointer receiver of composite type.
		// brmctx = Pointer receiver of basic type.
	}
	var (
		bval = bvmctx("bval")
		bref = brmctx("bref")
		xx   = x{
			S1: cvmctx{},
			S3: &cvmctx{},
			I1: "I1",
			I4: &bval,
			F1: crmctx{},
			F3: &crmctx{},
			P1: "P1",
			P4: &bref,
		}
	)
	marshalCompare(t, xx, "non-pointer")
	marshalCompare(t, &xx, "pointer")
}

type (
	niljetim string // jettison.Marshaler
	nilmjctx string // jettison.MarshalerCtx
	niljsonm string // json.Marshaler
	niltextm string // encoding.TextMarshaler
)

// comboMarshaler combines the json.Marshaler
// and jettison.AppendMarshaler interfaces so
// that tests outputs can be compared.
type comboMarshaler interface {
	AppendMarshaler
	json.Marshaler
}

// comboMarshalerCtx combines the json.Marshaler
// and jettison.AppendMarshalerCtx interfaces so
// that tests outputs can be compared.
type comboMarshalerCtx interface {
	AppendMarshalerCtx
	json.Marshaler
}

func (*niljetim) MarshalJSON() ([]byte, error) { return []byte(`"W"`), nil }
func (*nilmjctx) MarshalJSON() ([]byte, error) { return []byte(`"X"`), nil }
func (*niljsonm) MarshalJSON() ([]byte, error) { return []byte(`"Y"`), nil }
func (*niltextm) MarshalText() ([]byte, error) { return []byte("Z"), nil }

func (*niljetim) AppendJSON(dst []byte) ([]byte, error) {
	return append(dst, `"W"`...), nil
}
func (*nilmjctx) AppendJSONContext(_ context.Context, dst []byte) ([]byte, error) {
	return append(dst, `"X"`...), nil
}

type (
	errvjm   struct{}
	errrjm   struct{}
	errvtm   struct{}
	errrtm   struct{}
	errvm    struct{}
	errrm    struct{}
	errvmctx struct{}
	errrmctx struct{}
)

var errMarshaler = errors.New("error")

func (errvjm) MarshalJSON() ([]byte, error)          { return nil, errMarshaler }
func (*errrjm) MarshalJSON() ([]byte, error)         { return nil, errMarshaler }
func (errvtm) MarshalText() ([]byte, error)          { return nil, errMarshaler }
func (*errrtm) MarshalText() ([]byte, error)         { return nil, errMarshaler }
func (errvm) AppendJSON(dst []byte) ([]byte, error)  { return dst, errMarshaler }
func (*errrm) AppendJSON(dst []byte) ([]byte, error) { return dst, errMarshaler }

func (errvmctx) AppendJSONContext(_ context.Context, dst []byte) ([]byte, error) {
	return dst, errMarshaler
}
func (*errrmctx) AppendJSONContext(_ context.Context, dst []byte) ([]byte, error) {
	return dst, errMarshaler
}

// TestMarshalerError tests that a MarshalerError is
// returned when a MarshalText, MarshalJSON, WriteJSON
// or WriteJSONContext method returns an error.
func TestMarshalerError(t *testing.T) {
	testdata := []interface{}{
		errvjm{},
		&errrjm{},
		errvtm{},
		&errrtm{},
		errvm{},
		&errrm{},
		errvmctx{},
		&errrmctx{},
	}
	for _, v := range testdata {
		_, err := Marshal(v)
		if err != nil {
			me, ok := err.(*MarshalerError)
			if !ok {
				t.Fatalf("got %T, want MarshalerError", err)
			}
			typ := reflect.TypeOf(v)
			if me.Type != typ {
				t.Errorf("got %s, want %s", me.Type, typ)
			}
			if err := me.Unwrap(); err == nil {
				t.Error("expected non-nil error")
			}
			if me.Error() == "" {
				t.Error("expected non-empty error message")
			}
		} else {
			t.Error("got nil, want non-nil error")
		}
	}
}

// TestStructFieldName tests that invalid struct
// field names are ignored during marshaling.
func TestStructFieldName(t *testing.T) {
	//nolint:staticcheck
	type x struct {
		A  string `json:"   "`         // valid, spaces
		B  string `json:"0123"`        // valid, digits
		C  int    `json:","`           // invalid, comma
		D  int8   `json:"\\"`          // invalid, backslash,
		E  int16  `json:"\""`          // invalid, quotation mark
		F  int    `json:"Вилиам"`      // valid, UTF-8 runes
		G  bool   `json:"<ben&jerry>"` // valid, HTML-escaped chars
		Aβ int
	}
	marshalCompare(t, x{}, "")
}

// TestStructFieldOmitempty tests that the fields of
// a struct with the omitempty option are not encoded
// when they have the zero-value of their type.
func TestStructFieldOmitempty(t *testing.T) {
	type x struct {
		A  string      `json:",omitempty"`
		B  string      `json:",omitempty"`
		C  *string     `json:",omitempty"`
		Ca *string     `json:"a,omitempty"`
		D  *string     `json:",omitempty"`
		E  bool        `json:",omitempty"`
		F  int         `json:",omitempty"`
		F1 int8        `json:",omitempty"`
		F2 int16       `json:",omitempty"`
		F3 int32       `json:",omitempty"`
		F4 int64       `json:",omitempty"`
		G1 uint        `json:",omitempty"`
		G2 uint8       `json:",omitempty"`
		G3 uint16      `json:",omitempty"`
		G4 uint32      `json:",omitempty"`
		G5 uint64      `json:",omitempty"`
		G6 uintptr     `json:",omitempty"`
		H  float32     `json:",omitempty"`
		I  float64     `json:",omitempty"`
		J1 map[int]int `json:",omitempty"`
		J2 map[int]int `json:",omitempty"`
		J3 map[int]int `json:",omitempty"`
		K1 []string    `json:",omitempty"`
		K2 []string    `json:",omitempty"`
		L1 [0]int      `json:",omitempty"`
		L2 [2]int      `json:",omitempty"`
		M1 interface{} `json:",omitempty"`
		M2 interface{} `json:",omitempty"`
	}
	var (
		s1 = "Loreum"
		s2 = ""
		xx = &x{
			A:  "A",
			B:  "",
			C:  &s1,
			Ca: &s2,
			D:  nil,
			J2: map[int]int{},
			J3: map[int]int{1: 42},
			K2: []string{"K2"},
			M2: (*int)(nil),
		}
	)
	marshalCompare(t, xx, "")
}

// TestStructFieldOmitnil tests that the fields of a
// struct with the omitnil option are not encoded
// when they have a nil value.
func TestStructFieldOmitnil(t *testing.T) {
	// nolint:staticcheck
	type x struct {
		Sn  string                 `json:"sn,omitnil"`
		In  int                    `json:"in,omitnil"`
		Un  uint                   `json:"un,omitnil"`
		Fn  float64                `json:"fn,omitnil"`
		Bn  bool                   `json:"bn,omitnil"`
		Sln []string               `json:"sln,omitnil"`
		Mpn map[string]interface{} `json:"mpn,omitnil"`
		Stn struct{}               `json:"stn,omitnil"`
		Ptn *string                `json:"ptn,omitnil"`
		Ifn interface{}            `json:"ifn,omitnil"`
	}
	var (
		xx     = x{}
		before = `{"sn":"","in":0,"un":0,"fn":0,"bn":false,"stn":{}}`
		after  = `{"sn":"","in":0,"un":0,"fn":0,"bn":false,"sln":[],"mpn":{},"stn":{},"ptn":"Loreum","ifn":42}`
	)
	b, err := Marshal(xx)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != before {
		t.Errorf("before: got: %#q, want: %#q", got, before)
	}
	s := "Loreum"

	xx.Sln = make([]string, 0)
	xx.Mpn = map[string]interface{}{}
	xx.Stn = struct{}{}
	xx.Ptn = &s
	xx.Ifn = 42

	b, err = Marshal(xx)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != after {
		t.Errorf("after: got: %#q, want: %#q", got, after)
	}
}

// TestQuotedStructFields tests that the fields of
// a struct with the string option are quoted during
// marshaling if the type support it.
//nolint:staticcheck
func TestQuotedStructFields(t *testing.T) {
	type x struct {
		A1 int         `json:",string"`
		A2 *int        `json:",string"`
		A3 *int        `json:",string"`
		B  uint        `json:",string"`
		C1 bool        `json:",string"`
		C2 *bool       `json:",string"`
		D  float32     `json:",string"`
		E  string      `json:",string"`
		F  []int       `json:",string"`
		G  map[int]int `json:",string"`
	}
	var (
		i  = 84
		b  = false
		xx = &x{
			A1: -42,
			A2: nil,
			A3: &i,
			B:  42,
			C1: true,
			C2: &b,
			D:  math.Pi,
			E:  "E",
			F:  []int{1, 2, 3},
			G:  map[int]int{1: 2},
		}
	)
	marshalCompare(t, xx, "")
}

// TestBasicStructFieldTypes tests that struct
// fields of basic types can be marshaled.
func TestBasicStructFieldTypes(t *testing.T) {
	type x struct {
		A  string  `json:"a"`
		B1 int     `json:"b1"`
		B2 int8    `json:"b2"`
		B3 int16   `json:"b3"`
		B4 int32   `json:"b4"`
		B5 int64   `json:"b5"`
		C1 uint    `json:"c1"`
		C2 uint8   `json:"c2"`
		C3 uint16  `json:"c3"`
		C4 uint32  `json:"c4"`
		C5 uint64  `json:"c5"`
		D1 bool    `json:"d1"`
		D2 bool    `json:"d2"`
		E  float32 `json:"e"`
		F  float64 `json:"f"`
		G  string  `json:"-"`  // ignored
		H  string  `json:"-,"` // use "-" as key
		i  string
	}
	xx := &x{
		A:  "A",
		B1: -42,
		B2: math.MinInt8,
		B3: math.MinInt16,
		B4: math.MinInt32,
		B5: math.MinInt64,
		C1: 42,
		C2: math.MaxUint8,
		C3: math.MaxUint16,
		C4: math.MaxUint32,
		C5: math.MaxUint64,
		D1: true,
		D2: false,
		E:  3.14169,
		F:  math.MaxFloat64,
		G:  "ignored",
		H:  "not-ignored",
		i:  "unexported",
	}
	marshalCompare(t, xx, "non-pointer")
	marshalCompare(t, &xx, "pointer")
}

// TestBasicStructFieldPointerTypes tests
// that nil and non-nil struct field pointers
// of basic types can be marshaled.
func TestBasicStructFieldPointerTypes(t *testing.T) {
	type x struct {
		A *string  `json:"a"`
		B *int     `json:"b"`
		C *uint64  `json:"c"`
		D *bool    `json:"d"`
		E *float32 `json:"e"`
		F *float64 `json:"f"`
	}
	var (
		a  = "a"
		b  = 42
		d  = true
		f  = math.MaxFloat64
		xx = x{A: &a, B: &b, C: nil, D: &d, E: nil, F: &f}
	)
	marshalCompare(t, xx, "non-pointer")
	marshalCompare(t, &xx, "pointer")
}

// TestCompositeStructFieldTypes tests that struct
// fields of composite types, such as struct, slice,
// array and map can be marshaled.
func TestCompositeStructFieldTypes(t *testing.T) {
	type y struct {
		X string `json:"x"`
	}
	type x struct {
		A  y `json:"a"`
		B1 *y
		B2 *y
		b3 *y
		c1 []string
		C2 []string
		D  []int
		E  []bool
		F  []float32
		G  []*uint
		H  [3]string
		I  [1]int
		J  [0]bool
		K1 []byte
		K2 []byte
		L  []*int
		M1 []y
		M2 *[]y
		N1 []*y
		N2 []*y
		O1 [3]*int
		O2 *[3]*bool
		P  [3]*y
		Q  [][]int
		R  [2][2]string
		S1 map[int]string
		S2 map[int]string
		S3 map[int]string
		S4 map[string]interface{}
		T1 *map[string]int
		T2 *map[string]int
		T3 *map[string]int
		U1 interface{}
		U2 interface{}
		U3 interface{}
		U4 interface{}
		U5 interface{}
		U6 interface{}
		u7 interface{}
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Error(err)
	}
	var (
		l1 = 0
		l2 = 42
		m1 = y{X: "X"}
		m2 = y{}
		i0 = 42
		i1 = &i0
		i2 = &i1
		i3 = &i2
		xx = x{
			A:  y{X: "X"},
			B1: nil,
			B2: &y{X: "Ipsum"},
			b3: nil,
			c1: nil,
			C2: []string{"one", "two", "three"},
			D:  []int{1, 2, 3},
			E:  []bool{},
			H:  [3]string{"alpha", "beta", "gamma"},
			I:  [1]int{42},
			K1: k,
			K2: []byte(nil),
			L:  []*int{&l1, &l2, nil},
			M1: []y{m1, m2},
			N1: []*y{&m1, &m2, nil},
			N2: []*y{},
			O1: [3]*int{&l1, &l2, nil},
			P:  [3]*y{&m1, &m2, nil},
			Q:  [][]int{{1, 2}, {3, 4}},
			R:  [2][2]string{{"a", "b"}, {"c", "d"}},
			S1: nil,
			S3: map[int]string{1: "x", 2: "y", 3: "z"},
			S4: map[string]interface{}{"a": 1, "b": "2"},
			T3: &map[string]int{"x": 1, "y": 2, "z": 3},
			U1: "U1",
			U2: &l2,
			U3: nil,
			U4: false,
			U5: (*int)(nil), // typed nil
			U6: i3,          // chain of pointers
			u7: nil,
		}
	)
	marshalCompare(t, xx, "non-pointer")
	marshalCompare(t, &xx, "pointer")
}

// TestEmbeddedTypes tests that composite and basic
// embedded struct fields types are encoded whether
// they are exported.
func TestEmbeddedTypes(t *testing.T) {
	type (
		P1 int
		P2 string
		P3 bool
		p4 uint32
		C1 map[string]int
		C2 [3]string
		C3 []int
		c4 []bool
	)
	type x struct {
		P1
		P2
		P3
		p4
		C1
		C2
		C3
		c4 `json:"c4"`
	}
	xx := &x{
		P1: P1(42),
		P2: P2("P2"),
		P3: P3(true),
		p4: p4(math.MaxUint32),
		C1: C1{"A": 1, "B": 2},
		C2: C2{"A", "B", "C"},
		C3: C3{1, 2, 3},
		c4: c4{true, false},
	}
	marshalCompare(t, xx, "")
}

// TestRecursiveType tests the marshaling of
// recursive types.
func TestRecursiveType(t *testing.T) {
	type x struct {
		A string `json:"a"`
		X *x     `json:"x"`
	}
	xx := &x{
		A: "A1",
		X: &x{A: "A2"},
	}
	marshalCompare(t, xx, "")
}

// TestTaggedFieldDominates tests that a struct
// field with a tag dominates untagged fields.
func TestTaggedFieldDominates(t *testing.T) {
	type (
		A struct{ S string }
		D struct {
			XXX string `json:"S"`
		}
		Y struct {
			A
			D
		}
	)
	y := Y{
		A{"A"},
		D{"D"},
	}
	marshalCompare(t, y, "")
}

// TestDuplicatedFieldDisappears tests that
// duplicate struct field at the same level
// of embedding are ignored.
func TestDuplicatedFieldDisappears(t *testing.T) {
	type (
		A struct{ S string }
		C struct{ S string }
		D struct {
			XXX string `json:"S"`
		}
		Y struct {
			A
			D
		}
		Z struct {
			A
			C
			Y
		}
	)
	z := Z{A{"A"}, C{"C"}, Y{A{"S"}, D{"D"}}}

	marshalCompare(t, z, "")
}

// TestEmbeddedStructs tests that named and unnamed
// embedded structs fields can be marshaled.
func TestEmbeddedStructs(t *testing.T) {
	type (
		r struct {
			J string `json:"j"`
		}
		v struct {
			H bool   `json:"h,omitempty"`
			I string `json:"i"`
		}
		y struct {
			D int8  `json:"d"`
			E uint8 `json:"e,omitempty"`
			r
			v
		}
		z struct {
			F int16  `json:"f,omitempty"`
			G uint16 `json:"g"`
			y
			v
		}
		// According to the Go rules for embedded fields,
		// y.r.J should be encoded while z.y.r.J is not,
		// because is one-level up.
		// However, y.v.H and z.v.H are present at the same
		// level, and therefore are both hidden.
		x1 struct {
			A string `json:"a,omitempty"`
			y
			B string `json:"b"`
			v `json:"v"`
			C string `json:"c,omitempty"`
			z `json:",omitempty"`
			*x1
		}
		// x2 is a variant of the x1 type without
		// the omitempty option on the first field.
		x2 struct {
			A int16 `json:"a"`
			v `json:"v"`
		}
	)
	xx1 := &x1{
		A: "A",
		y: y{
			D: math.MinInt8,
			r: r{J: "J"},
			v: v{H: false},
		},
		z: z{
			G: math.MaxUint16,
			y: y{D: 21, r: r{J: "J"}},
			v: v{H: true},
		},
		x1: &x1{
			A: "A",
		},
	}
	xx2 := &x2{A: 42, v: v{I: "I"}}

	marshalCompare(t, xx1, "")
	marshalCompare(t, xx2, "")
}

// TestAnonymousFields tests the marshaling of
// advanced cases for anonymous struct fields.
// Adapted from the encoding/json testsuite.
func TestAnonymousFields(t *testing.T) {
	testdata := []struct {
		label string
		input func() []interface{}
	}{{
		// Both S1 and S2 have a field named X.
		// From the perspective of S, it is
		// ambiguous which one X refers to.
		// This should not encode either field.
		label: "AmbiguousField",
		input: func() []interface{} {
			type (
				S1 struct{ x, X int }
				S2 struct{ x, X int }
				S  struct {
					S1
					S2
				}
			)
			return []interface{}{
				S{S1{1, 2}, S2{3, 4}},
				&S{S1{5, 6}, S2{7, 8}},
			}
		},
	}, {
		// Both S1 and S2 have a field named X, but
		// since S has an X field as well, it takes
		// precedence over S1.X and S2.X.
		label: "DominantField",
		input: func() []interface{} {
			type (
				S1 struct{ x, X int }
				S2 struct{ x, X int }
				S  struct {
					S1
					S2
					x, X int
				}
			)
			return []interface{}{
				S{S1{1, 2}, S2{3, 4}, 5, 6},
				&S{S1{6, 5}, S2{4, 3}, 2, 1},
			}
		},
	}, {
		// Unexported embedded field of non-struct type
		// should not be serialized.
		label: "UnexportedEmbeddedInt",
		input: func() []interface{} {
			type (
				i int
				S struct{ i }
			)
			return []interface{}{S{5}, &S{6}}
		},
	}, {
		// Exported embedded field of non-struct type
		// should be serialized.
		label: "ExportedEmbeddedInt",
		input: func() []interface{} {
			type (
				I int
				S struct{ I }
			)
			return []interface{}{S{5}, &S{6}}
		},
	}, {
		// Unexported embedded field of pointer to
		// non-struct type should not be serialized.
		label: "UnexportedEmbeddedIntPointer",
		input: func() []interface{} {
			type (
				i int
				S struct{ *i }
			)
			s := S{new(i)}
			*s.i = 5
			return []interface{}{s, &s}
		},
	}, {
		// Exported embedded field of pointer to
		// non-struct type should be serialized.
		label: "ExportedEmbeddedIntPointer",
		input: func() []interface{} {
			type (
				I int
				S struct{ *I }
			)
			s := S{new(I)}
			*s.I = 5
			return []interface{}{s, &s}
		},
	}, {
		// Exported embedded field of nil pointer
		// to non-struct type should be serialized.
		label: "ExportedEmbeddedNilIntPointer",
		input: func() []interface{} {
			type (
				I int
				S struct{ *I }
			)
			s := S{new(I)}
			s.I = nil
			return []interface{}{s, &s}
		},
	}, {
		// Exported embedded field of nil pointer to
		// non-struct type should not be serialized
		// if it has the omitempty option.
		label: "ExportedEmbeddedNilIntPointerOmitempty",
		input: func() []interface{} {
			type (
				I int
				S struct {
					*I `json:",omitempty"`
				}
			)
			s := S{new(I)}
			s.I = nil
			return []interface{}{s, &s}
		},
	}, {
		// Exported embedded field of pointer to
		// struct type should be serialized.
		label: "ExportedEmbeddedStructPointer",
		input: func() []interface{} {
			type (
				S struct{ X string }
				T struct{ *S }
			)
			t := T{S: &S{
				X: "X",
			}}
			return []interface{}{t, &t}
		},
	}, {
		// Exported fields of embedded structs should
		// have their exported fields be serialized
		// regardless of whether the struct types
		// themselves are exported.
		label: "EmbeddedStructNonPointer",
		input: func() []interface{} {
			type (
				s1 struct{ x, X int }
				S2 struct{ y, Y int }
				S  struct {
					s1
					S2
				}
			)
			return []interface{}{
				S{s1{1, 2}, S2{3, 4}},
				&S{s1{5, 6}, S2{7, 8}},
			}
		},
	}, {
		// Exported fields of pointers to embedded
		// structs should have their exported fields
		// be serialized regardless of whether the
		// struct types themselves are exported.
		label: "EmbeddedStructPointer",
		input: func() []interface{} {
			type (
				s1 struct{ x, X int }
				S2 struct{ y, Y int }
				S  struct {
					*s1
					*S2
				}
			)
			return []interface{}{
				S{&s1{1, 2}, &S2{3, 4}},
				&S{&s1{5, 6}, &S2{7, 8}},
			}
		},
	}, {
		// Exported fields on embedded unexported
		// structs at multiple levels of nesting
		// should still be serialized.
		label: "NestedStructAndInts",
		input: func() []interface{} {
			type (
				I1 int
				I2 int
				i  int
				s2 struct {
					I2
					i
				}
				s1 struct {
					I1
					i
					s2
				}
				S struct {
					s1
					i
				}
			)
			return []interface{}{
				S{s1{1, 2, s2{3, 4}}, 5},
				&S{s1{5, 4, s2{3, 2}}, 1},
			}
		},
	}, {
		// If an anonymous struct pointer field is nil,
		// we should ignore the embedded fields behind it.
		// Not properly doing so may result in the wrong
		// output or a panic.
		label: "EmbeddedFieldBehindNilPointer",
		input: func() []interface{} {
			type (
				S2 struct{ Field string }
				S  struct{ *S2 }
			)
			return []interface{}{S{}, &S{}}
		},
	}, {
		// A field behind a chain of pointer and
		// non-pointer embedded fields should be
		// accessible and serialized.
		label: "BasicEmbeddedFieldChain",
		input: func() []interface{} {
			type (
				A struct {
					X1 string
					X2 *string
				}
				B struct{ *A }
				C struct{ B }
				D struct{ *C }
				E struct{ D }
				F struct{ *E }
			)
			s := "Loreum"
			f := F{E: &E{D: D{C: &C{B: B{A: &A{X1: "X1", X2: &s}}}}}}
			return []interface{}{f, &f}
		},
	}, {
		// Variant of the test above, with embedded
		// fields of type struct that contain one or
		// more fields themselves.
		label: "ComplexEmbeddedFieldChain",
		input: func() []interface{} {
			type (
				A struct {
					X1 string `json:",omitempty"`
					X2 string
				}
				B struct {
					Z3 *bool
					A
				}
				C struct{ B }
				D struct {
					*C
					Z2 int
				}
				E struct{ *D }
				F struct {
					Z1 string `json:",omitempty"`
					*E
				}
			)
			f := F{Z1: "Z1", E: &E{D: &D{C: &C{B: B{A: A{X2: "X2"}, Z3: new(bool)}}, Z2: 1}}}
			return []interface{}{f, &f}
		},
	}}
	for i := range testdata {
		e := testdata[i]
		t.Run(e.label, func(t *testing.T) {
			for i, input := range e.input() {
				input := input
				var label string
				if i == 0 {
					label = "non-pointer"
				} else {
					label = "pointer"
				}
				t.Run(label, func(t *testing.T) {
					marshalCompare(t, input, label)
				})
			}
		})
	}
}

func TestBytesEscaping(t *testing.T) {
	testdata := []struct {
		in, out string
	}{
		{"\x00", `"\u0000"`},
		{"\x01", `"\u0001"`},
		{"\x02", `"\u0002"`},
		{"\x03", `"\u0003"`},
		{"\x04", `"\u0004"`},
		{"\x05", `"\u0005"`},
		{"\x06", `"\u0006"`},
		{"\x07", `"\u0007"`},
		{"\x08", `"\u0008"`},
		{"\x09", `"\t"`},
		{"\x0a", `"\n"`},
		{"\x0b", `"\u000b"`},
		{"\x0c", `"\u000c"`},
		{"\x0d", `"\r"`},
		{"\x0e", `"\u000e"`},
		{"\x0f", `"\u000f"`},
		{"\x10", `"\u0010"`},
		{"\x11", `"\u0011"`},
		{"\x12", `"\u0012"`},
		{"\x13", `"\u0013"`},
		{"\x14", `"\u0014"`},
		{"\x15", `"\u0015"`},
		{"\x16", `"\u0016"`},
		{"\x17", `"\u0017"`},
		{"\x18", `"\u0018"`},
		{"\x19", `"\u0019"`},
		{"\x1a", `"\u001a"`},
		{"\x1b", `"\u001b"`},
		{"\x1c", `"\u001c"`},
		{"\x1d", `"\u001d"`},
		{"\x1e", `"\u001e"`},
		{"\x1f", `"\u001f"`},
	}
	for _, tt := range testdata {
		b, err := Marshal(tt.in)
		if err != nil {
			t.Error(err)
		}
		if s := string(b); s != tt.out {
			t.Errorf("got %#q, want %#q", s, tt.out)
		}
	}
}

// TestStringEscaping tests that control and reserved
// JSON characters are properly escaped when a string
// is marshaled.
func TestStringEscaping(t *testing.T) {
	b := []byte{
		'A', 1, 2, 3,
		'"', '\\', '/', '\b', '\f', '\n', '\r', '\t',
		0xC7, 0xA3, 0xE2, 0x80, 0xA8, 0xE2, 0x80, 0xA9,
	}
	testdata := []struct {
		b   []byte
		s   string
		opt Option
		cmp bool
	}{
		{b, `"A\u0001\u0002\u0003\"\\/\u0008\u000c\n\r\tǣ\u2028\u2029"`, nil, true},
		{b, `"` + string(b) + `"`, NoStringEscaping(), false},
	}
	for _, tt := range testdata {
		b, err := MarshalOpts(string(tt.b), tt.opt)
		if err != nil {
			t.Error(err)
		}
		if s := string(b); s != tt.s {
			t.Errorf("got %#q, want %#q", s, tt.s)
		}
		if tt.cmp {
			bs, err := json.Marshal(string(tt.b))
			if err != nil {
				t.Error(err)
			}
			if !bytes.Equal(bs, b) {
				t.Logf("standard: %s", bs)
				t.Logf("jettison: %s", b)
				t.Errorf("expected equal outputs")
			}
		}
	}
}

// TestStringHTMLEscaping tests that HTML characters
// are properly escaped when a string is marshaled.
func TestStringHTMLEscaping(t *testing.T) {
	htmlChars := []byte{'<', '>', '&'}
	testdata := []struct {
		b    []byte
		s    string
		opts []Option
	}{
		{htmlChars, `"\u003c\u003e\u0026"`, nil},
		{htmlChars, `"<>&"`, []Option{NoHTMLEscaping()}},

		// NoHTMLEscaping is ignored when NoStringEscaping
		// is set, because it's part of the escaping options.
		{htmlChars, `"<>&"`, []Option{NoStringEscaping()}},
		{htmlChars, `"<>&"`, []Option{NoStringEscaping(), NoHTMLEscaping()}},
	}
	for _, tt := range testdata {
		b, err := MarshalOpts(string(tt.b), tt.opts...)
		if err != nil {
			t.Error(err)
		}
		if s := string(b); s != tt.s {
			t.Errorf("got %#q, want %#q", s, tt.s)
		}
	}
}

// TestStringUTF8Coercion tests that invalid bytes
// are replaced by the Unicode replacement rune when
// a string is marshaled.
func TestStringUTF8Coercion(t *testing.T) {
	utf8Seq := string([]byte{'H', 'e', 'l', 'l', 'o', ',', ' ', 0xff, 0xfe, 0xff})
	testdata := []struct {
		b   string
		s   string
		opt Option
	}{
		{utf8Seq, `"Hello, \ufffd\ufffd\ufffd"`, nil},
		{utf8Seq, `"` + utf8Seq + `"`, NoUTF8Coercion()},
	}
	for _, tt := range testdata {
		b, err := MarshalOpts(tt.b, tt.opt)
		if err != nil {
			t.Error(err)
		}
		if s := string(b); s != tt.s {
			t.Errorf("got %#q, want %#q", s, tt.s)
		}
	}
}

func TestMarshalFloat(t *testing.T) {
	// Taken from encoding/json.
	t.Parallel()

	nf := 0
	mc := regexp.MustCompile
	re := []*regexp.Regexp{
		mc(`p`),
		mc(`^\+`),
		mc(`^-?0[^.]`),
		mc(`^-?\.`),
		mc(`\.(e|$)`),
		mc(`\.[0-9]+0(e|$)`),
		mc(`^-?(0|[0-9]{2,})\..*e`),
		mc(`e[0-9]`),
		mc(`e[+-]0`),
		mc(`e-[1-6]$`),
		mc(`e+(.|1.|20)$`),
		mc(`^-?0\.0000000`),
		mc(`^-?[0-9]{22}`),
		mc(`[1-9][0-9]{16}[1-9]`),
		mc(`[1-9][0-9.]{17}[1-9]`),
		mc(`[1-9][0-9]{8}[1-9]`),
		mc(`[1-9][0-9.]{9}[1-9]`),
	}
	fn := func(f float64, bits int) {
		vf := interface{}(f)
		if bits == 32 {
			f = float64(float32(f)) // round
			vf = float32(f)
		}
		bout, err := Marshal(vf)
		if err != nil {
			t.Errorf("Encode(%T(%g)): %v", vf, vf, err)
			nf++
			return
		}
		out := string(bout)

		// Result must convert back to the same float.
		g, err := strconv.ParseFloat(out, bits)
		if err != nil {
			t.Errorf("%T(%g) = %q, cannot parse back: %v", vf, vf, out, err)
			nf++
			return
		}
		if f != g || fmt.Sprint(f) != fmt.Sprint(g) { // fmt.Sprint handles ±0
			t.Errorf("%T(%g) = %q (is %g, not %g)", vf, vf, out, float32(g), vf)
			nf++
			return
		}
		bad := re
		if bits == 64 {
			// Last two regexps are for 32-bits values only.
			bad = bad[:len(bad)-2]
		}
		for _, re := range bad {
			if re.MatchString(out) {
				t.Errorf("%T(%g) = %q, must not match /%s/", vf, vf, out, re)
				nf++
				return
			}
		}
	}
	fn(0, 64)
	fn(math.Copysign(0, -1), 64)
	fn(0, 32)
	fn(math.Copysign(0, -1), 32)

	var (
		bigger  = math.Inf(+1)
		smaller = math.Inf(-1)
		digits  = "1.2345678901234567890123"
	)
	for i := len(digits); i >= 2; i-- {
		if testing.Short() && i < len(digits)-4 {
			break
		}
		for exp := -30; exp <= 30; exp++ {
			for _, sign := range "+-" {
				for bits := 32; bits <= 64; bits += 32 {
					s := fmt.Sprintf("%c%se%d", sign, digits[:i], exp)
					f, err := strconv.ParseFloat(s, bits)
					if err != nil {
						t.Fatal(err)
					}
					next := math.Nextafter
					if bits == 32 {
						next = func(g, h float64) float64 {
							return float64(math.Nextafter32(float32(g), float32(h)))
						}
					}
					fn(f, bits)
					fn(next(f, bigger), bits)
					fn(next(f, smaller), bits)

					if nf > 50 {
						t.Fatalf("too many fails, stopping tests early")
					}
				}
			}
		}
	}
}
