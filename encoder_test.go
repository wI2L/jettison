package jettison

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/francoispqt/gojay"
	jsoniter "github.com/json-iterator/go"
)

var jsoniterStd = jsoniter.ConfigCompatibleWithStandardLibrary

// TestNewEncoderNilInterface tests that creating a new
// encoder for a nil interface returns an error.
func TestNewEncoderNilInterface(t *testing.T) {
	_, err := NewEncoder(nil)
	if err == nil {
		t.Error("expected non-nil error")
	}
}

// TestEncodeWithIncompatibleType tests that invoking the
// Encode method of an encoder with a type that differs from
// the one for which is was created returns an error.
func TestEncodeWithIncompatibleType(t *testing.T) {
	type x struct{}
	enc, err := NewEncoder(x{})
	if err != nil {
		t.Fatal(err)
	}
	err = enc.Encode("Loreum", nil)
	if err == nil {
		t.Error("expected non-nil error")
	}
}

// TestUnsupportedTypeError tests that UnsupportedTypeError
// type implements the error builtin interface and that it
// returns an appropriate error message.
func TestUnsupportedTypeError(t *testing.T) {
	ute := &UnsupportedTypeError{Typ: reflect.TypeOf("Loreum")}
	const want = "unsupported type: string"
	if s := ute.Error(); s != want {
		t.Errorf("got %s, want %s", s, want)
	}
}

// TestUnsupportedValueError tests that UnsupportedValueError
// type implements the error builtin interface and that it
// returns an appropriate error message.
func TestUnsupportedValueError(t *testing.T) {
	ute := &UnsupportedValueError{Str: "foobar"}
	const want = "unsupported value: foobar"
	if s := ute.Error(); s != want {
		t.Errorf("got %s, want %s", s, want)
	}
}

// TestNilValues tests the behavior of an encoder's
// Encode method for typed and untyped nil values.
func TestNilValues(t *testing.T) {
	enc, err := NewEncoder(int(0))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	// Encode typed nil value.
	if err := enc.Encode((*int)(nil), &buf); err != nil {
		t.Error(err)
	}
	if s := buf.String(); s != "null" {
		t.Errorf("got %s, want null", s)
	}
	buf.Reset()

	// Encode untyped nil value.
	if err := enc.Encode(nil, &buf); err != nil {
		t.Error(err)
	}
	if s := buf.String(); s != "null" {
		t.Errorf("got %s, want null", s)
	}
}

// TestPrimitiveTypes tests that primitive
// types can be encoded.
func TestPrimitiveTypes(t *testing.T) {
	testdata := []struct {
		Iface interface{}
		Str   string
	}{
		{bool(true), "true"},
		{bool(false), "false"},
		{string("Loreum"), `"Loreum"`},
		{int8(math.MaxInt8), "127"},
		{int16(math.MaxInt16), "32767"},
		{int32(math.MaxInt32), "2147483647"},
		{int64(math.MaxInt64), "9223372036854775807"},
		{uint8(math.MaxUint8), "255"},
		{uint16(math.MaxUint16), "65535"},
		{uint32(math.MaxUint32), "4294967295"},
		{uint64(math.MaxUint64), "18446744073709551615"},
		{uintptr(0xBEEF), "48879"},
		{(*int)(nil), "null"},
	}
	for _, tt := range testdata {
		enc, err := NewEncoder(tt.Iface)
		if err != nil {
			t.Error(err)
		}
		var buf bytes.Buffer
		if err := enc.Encode(tt.Iface, &buf); err != nil {
			t.Error(err)
		}
		if s := buf.String(); s != tt.Str {
			t.Errorf("got `%s`, want `%s`", s, tt.Str)
		}
	}
}

// TestCompositeTypes tests that composite
// types can be encoded.
func TestCompositeTypes(t *testing.T) {
	testdata := []struct {
		Iface interface{}
		Str   string
	}{
		{[]uint{}, "[]"},
		{[]int{1, 2, 3}, "[1,2,3]"},
		{[]int(nil), "null"},
		{(*[]int)(nil), "null"},
		{[]string{"a", "b", "c"}, `["a","b","c"]`},
		{[2]bool{true, false}, "[true,false]"},
		{(*[4]string)(nil), "null"},
		{map[string]int{"a": 1, "b": 2}, `{"a":1,"b":2}`},
		{&map[int]string{1: "a", 2: "b"}, `{"1":"a","2":"b"}`},
		{(map[string]int)(nil), "null"},
	}
	for _, tt := range testdata {
		enc, err := NewEncoder(tt.Iface)
		if err != nil {
			t.Error(err)
		}
		var buf bytes.Buffer
		if err := enc.Encode(tt.Iface, &buf); err != nil {
			t.Error(err)
		}
		if s := buf.String(); s != tt.Str {
			t.Errorf("got `%s`, want `%s`", s, tt.Str)
		}
	}
}

// TestUnsupportedTypes tests that encoding an
// unsupported type returns UnsupportedTypeError.
func TestUnsupportedTypes(t *testing.T) {
	testdata := []interface{}{
		make(chan int),
		func() {},
		complex64(0),
		complex128(0),
	}
	for _, tt := range testdata {
		enc, _ := NewEncoder(tt)
		err := enc.Compile()
		if err != nil {
			e, ok := err.(*UnsupportedTypeError)
			if !ok {
				t.Errorf("got %T, want UnsupportedTypeError", err)
			}
			if typ := reflect.TypeOf(tt); e.Typ != typ {
				t.Errorf("got %v, want %v", e.Typ, typ)
			}
		} else {
			t.Error("got nil, want non-nil error")
		}
	}
}

// TestUnsupportedCompositeElemTypes tests that encoding
// a composite type with an unsupported element type
// returns UnsupportedTypeError.
func TestUnsupportedCompositeElemTypes(t *testing.T) {
	for _, tt := range []interface{}{
		[]chan int{},
		[2]complex64{},
	} {
		enc, _ := NewEncoder(tt)
		err := enc.Compile()
		if err != nil {
			e, ok := err.(*UnsupportedTypeError)
			if !ok {
				t.Errorf("got %T, want UnsupportedTypeError", err)
			}
			if typ := reflect.TypeOf(tt); e.Typ != typ {
				t.Errorf("got %v, want %v", e.Typ, typ)
			}
		} else {
			t.Error("got nil, want non-nil error")
		}
	}
}

// TestMap tests the encoding of sorted and unsorted
// maps. See the BenchmarkMap benchmar for a performance
// comparison between the two cases.
func TestMap(t *testing.T) {
	testdata := []struct {
		Val    map[string]int
		Str    string
		NoSort bool
		NME    bool // NilMapEmpty
	}{
		{nil, "null", false, false},
		{nil, "{}", false, true},
		{map[string]int{"a": 1, "b": 2, "c": 3}, `{"a":1,"b":2,"c":3}`, false, false},
		{map[string]int{"c": 3, "a": 1, "b": 2}, `{"a":1,"b":2,"c":3}`, false, false},
		{map[string]int{"a": 1, "b": 2, "c": 3}, "", true, false},
		{map[string]int{"c": 3, "a": 1, "b": 2}, "", true, false},
	}
	for _, tt := range testdata {
		enc, err := NewEncoder(tt.Val)
		if err != nil {
			t.Error(err)
		}
		var buf bytes.Buffer
		var opts []Option
		if tt.NoSort {
			opts = append(opts, UnsortedMap)
		}
		if tt.NME {
			opts = append(opts, NilMapEmpty)
		}
		if err := enc.Encode(tt.Val, &buf, opts...); err != nil {
			t.Error(err)
		}
		if !tt.NoSort {
			if s := buf.String(); s != tt.Str {
				t.Errorf("got `%s`, want `%s`", s, tt.Str)
			}
		} else {
			// Cannot compare the result to a
			// static string, since the iteration
			// order is undefined.
			m := make(map[string]int)
			if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
				t.Error(err)
			}
			if !reflect.DeepEqual(tt.Val, m) {
				t.Errorf("expected maps to be deeply equal, got %v, want %v", m, tt.Val)
			}
		}
	}
}

func TestSlice(t *testing.T) {
	testdata := []struct {
		Val []string
		Str string
		NME bool // NilSliceEmpty
	}{
		{nil, "null", false},
		{nil, "[]", true},
		{[]string{}, "[]", false},
		{[]string{}, "[]", true},
		{[]string{"a", "b", "c"}, `["a","b","c"]`, false},
		{[]string{"a", "b", "c"}, `["a","b","c"]`, true},
	}
	for _, tt := range testdata {
		enc, err := NewEncoder(tt.Val)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		var opts []Option
		if tt.NME {
			opts = append(opts, NilSliceEmpty)
		}
		if err := enc.Encode(tt.Val, &buf, opts...); err != nil {
			t.Error(err)
		}
		if s := buf.String(); s != tt.Str {
			t.Errorf("got `%s`, want `%s`", s, tt.Str)
		}
	}
}

// TestCompositeMapValue tests that a map
// with composite value types can be encoded.
func TestCompositeMapValue(t *testing.T) {
	type x struct {
		A string `json:"a"`
		B int    `json:"b"`
		C bool   `json:"c"`
	}
	type y []uint32

	for _, tt := range []interface{}{
		map[string]x{
			"1": {A: "Loreum", B: 42, C: true},
			"2": {A: "Loream", B: 84, C: false},
		},
		map[string]y{
			"3": {7, 8, 9},
			"2": {4, 5, 6},
			"1": nil,
		},
		map[string]*x{
			"b": {A: "Loreum", B: 128, C: true},
			"a": nil,
			"c": {},
		},
		map[string]interface{}{
			"1": 42,
			"2": "Loreum",
			"3": nil,
			"4": (*int64)(nil),
			"5": x{A: "Ipsem"},
			"6": &x{A: "Sit Amet", B: 256, C: true},
		},
	} {
		enc, err := NewEncoder(tt)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := enc.Encode(tt, &buf); err != nil {
			t.Error(err)
		}
		if !equalStdLib(t, tt, buf.Bytes()) {
			t.Error("expected outputs to be equal")
		}
	}
}

type structTextMarshaler struct {
	L string
	R string
}

func (stm structTextMarshaler) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("%s:%s", stm.L, stm.R)), nil
}

type intMarshaler int

func (im intMarshaler) MarshalText() ([]byte, error) {
	return []byte(strconv.Itoa(int(im))), nil
}

// TestTextMarshalerMapKey tests that a map with
// key types implemeting the text.Marshaler interface
// can be encoded.
func TestTextMarshalerMapKey(t *testing.T) {
	var (
		im intMarshaler = 42
		ip              = &net.IP{127, 0, 0, 1}
	)
	testdata := []interface{}{
		map[time.Time]string{
			time.Now(): "now",
			{}:         "",
		},
		map[*net.IP]string{
			ip: "localhost",
			// The nil key case, although supported by
			// this library isn't tested because the
			// standard library panics on it, and thus,
			// the results cannot be compared.
			// nil: "",
		},
		map[structTextMarshaler]string{
			{L: "A", R: "B"}: "ab",
		},
		map[*intMarshaler]string{
			&im: "42",
		},
	}
	for _, tt := range testdata {
		enc, err := NewEncoder(tt)
		if err != nil {
			t.Error(err)
		}
		var buf bytes.Buffer
		if err := enc.Encode(tt, &buf); err != nil {
			t.Error(err)
		}
		if !equalStdLib(t, tt, buf.Bytes()) {
			t.Error("expected outputs to be equal")
		}
	}
}

// TestPrimitiveStructFieldTypes tests that struct
// fields of primitive types can be encoded.
func TestPrimitiveStructFieldTypes(t *testing.T) {
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
	enc, err := NewEncoder(x{})
	if err != nil {
		t.Fatal(err)
	}
	xx := &x{
		A:  "Loreum",
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
	var buf bytes.Buffer
	if err := enc.Encode(xx, &buf); err != nil {
		t.Fatal(err)
	}
	if !equalStdLib(t, xx, buf.Bytes()) {
		t.Error("expected outputs to be equal")
	}
}

// TestPrimitiveStructFieldPointerTypes tests
// that nil and non-nil struct field pointers of
// primitive types can be encoded.
func TestPrimitiveStructFieldPointerTypes(t *testing.T) {
	type x struct {
		A *string  `json:"a"`
		B *int     `json:"b"`
		C *uint64  `json:"c"`
		D *bool    `json:"d"`
		E *float32 `json:"e"`
		F *float64 `json:"f"`
		g *int64
	}
	enc, err := NewEncoder(x{})
	if err != nil {
		t.Fatal(err)
	}
	var (
		a = "Loreum"
		b = 42
		d = true
		f = math.MaxFloat64
	)
	xx := x{A: &a, B: &b, C: nil, D: &d, E: nil, F: &f, g: nil}

	var buf bytes.Buffer
	if err := enc.Encode(xx, &buf); err != nil {
		t.Fatal(err)
	}
	if !equalStdLib(t, xx, buf.Bytes()) {
		t.Error("expected outputs to be equal")
	}
}

// TestUnsupportedStructFieldTypes tests that encoding
// a struct with unsupported field types returns
// UnsupportedTypeError.
func TestUnsupportedStructFieldTypes(t *testing.T) {
	type x struct {
		C chan struct{}
	}
	enc, err := NewEncoder(x{})
	if err != nil {
		t.Fatal(err)
	}
	err = enc.Compile()
	if err != nil {
		e, ok := err.(*UnsupportedTypeError)
		if !ok {
			t.Errorf("got %T, want UnsupportedTypeError", err)
		}
		ch := make(chan struct{})
		if typ := reflect.TypeOf(ch); e.Typ != typ {
			t.Errorf("got %v, want %v", e.Typ, typ)
		}
	} else {
		t.Error("got nil, want non-nil error")
	}
}

// TestStructFieldName tests that invalid struct
// field names are ignored during encoding.
func TestStructFieldName(t *testing.T) {
	type x struct {
		A  string `json:" "`    // valid name
		B  string `json:"0123"` // valid name
		C  int    `json:","`    // invalid name, comma
		D  int8   `json:"\\"`   // invalid name, backslash
		E  int16  `json:"\""`   // invalid name, quotation mark
		F  int    `json:"虚拟"`
		Aβ int
	}
	enc, err := NewEncoder(x{})
	if err != nil {
		t.Fatal(err)
	}
	xx := new(x)
	var buf bytes.Buffer
	if err := enc.Encode(xx, &buf); err != nil {
		t.Fatal(err)
	}
	if !equalStdLib(t, xx, buf.Bytes()) {
		t.Error("expected outputs to be equal")
	}
}

// TestStructFieldOmitempty tests that the fields of
// a struct with the omitempty option are not encoded
// when they have the zero-value of their type.
func TestStructFieldOmitempty(t *testing.T) {
	type x struct {
		A  string      `json:"a,omitempty"`
		B  string      `json:"b,omitempty"`
		C  *string     `json:"c,omitempty"`
		Ca *string     `json:"ca,omitempty"`
		D  *string     `json:"d,omitempty"`
		E  bool        `json:"e,omitempty"`
		F  int         `json:"f,omitempty"`
		F1 int8        `json:"f1,omitempty"`
		F2 int16       `json:"f2,omitempty"`
		F3 int32       `json:"f3,omitempty"`
		F4 int64       `json:"f4,omitempty"`
		G  uint        `json:"g,omitempty"`
		G1 uint8       `json:"g1,omitempty"`
		G2 uint16      `json:"g2,omitempty"`
		G3 uint32      `json:"g3,omitempty"`
		G4 uint64      `json:"g4,omitempty"`
		G5 uintptr     `json:"g5,omitempty"`
		H  float32     `json:"h,omitempty"`
		I  float64     `json:"i,omitempty"`
		J1 map[int]int `json:"j1,omitempty"`
		J2 map[int]int `json:"j2,omitempty"`
		J3 map[int]int `json:"j3,omitempty"`
		K1 []string    `json:"k1,omitempty"`
		K2 []string    `json:"k2,omitempty"`
	}
	enc, err := NewEncoder(x{})
	if err != nil {
		t.Fatal(err)
	}
	s1 := "Loreum Ipsum"
	s2 := ""
	xx := &x{
		A:  "Loreum",
		B:  "",
		C:  &s1,
		Ca: &s2,
		D:  nil,
		J2: map[int]int{},
		J3: map[int]int{1: 42},
		K2: []string{"Loreum"},
	}
	var buf bytes.Buffer
	if err := enc.Encode(xx, &buf); err != nil {
		t.Fatal(err)
	}
	if !equalStdLib(t, xx, buf.Bytes()) {
		t.Error("expected outputs to be equal")
	}
}

// TestQuotedStructField tests that the fields
// of a struct with the string option are quoted
// during encoding.
func TestQuotedStructField(t *testing.T) {
	type x struct {
		A1 int     `json:"a1,string"`
		A2 *int    `json:"a2,string"`
		A3 *int    `json:"a3,string"`
		B  uint    `json:"b,string"`
		C  bool    `json:"c,string"`
		D  float32 `json:",string"`
		E  string  `json:"e,string"`
	}
	enc, err := NewEncoder(x{})
	if err != nil {
		t.Fatal(err)
	}
	i := 84
	xx := &x{
		A1: -42,
		A2: nil,
		A3: &i,
		B:  42,
		C:  true,
		D:  math.Pi,
		E:  "Loreum",
	}
	var buf bytes.Buffer
	if err := enc.Encode(xx, &buf); err != nil {
		t.Fatal(err)
	}
	if !equalStdLib(t, xx, buf.Bytes()) {
		t.Error("expected outputs to be equal")
	}
}

// TestCompositeStructFieldTypes tests that struct
// fields of composite types, uch as struct, slice,
// array and map can be encoded.
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
		C2 []string               `json:"C2"`
		D  []int                  `json:"d"`
		E  []bool                 `json:"e"`
		F  []float32              `json:"f,omitempty"`
		G  []*uint                `json:"g"`
		H  [3]string              `json:"h"`
		I  [1]int                 `json:"i,omitempty"`
		J  [0]bool                `json:"j"`
		K1 []byte                 `json:"k1"`
		K2 []byte                 `json:"k2"`
		L  []*int                 `json:"l"`
		M1 []y                    `json:"m1"`
		M2 *[]y                   `json:"m2"`
		N1 []*y                   `json:"n1"`
		N2 []*y                   `json:"n2"`
		O1 [3]*int                `json:"o1"`
		O2 *[3]*bool              `json:"o2,omitempty"`
		P  [3]*y                  `json:"p"`
		Q  [][]int                `json:"q"`
		R  [2][2]string           `json:"r"`
		S1 map[int]string         `json:"s1,omitempty"`
		S2 map[int]string         `json:"s2"`
		S3 map[int]string         `json:"s3"`
		S4 map[string]interface{} `json:"s4"`
		T1 *map[string]int        `json:"t1,omitempty"`
		T2 *map[string]int        `json:"t2"`
		T3 *map[string]int        `json:"t3"`
		U1 interface{}            `json:"u1"`
		U2 interface{}            `json:"u2"`
		U3 interface{}            `json:"u3"`
		U4 interface{}            `json:"u4,omitempty"`
		U5 interface{}            `json:"u5"`
		U6 interface{}            `json:"u6"`
		u7 interface{}
	}
	enc, err := NewEncoder(&x{})
	if err != nil {
		t.Fatal(err)
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Error(err)
	}
	var (
		l1, l2 = 0, 42
		m1, m2 = y{X: "Loreum"}, y{}
	)
	i0 := 42
	i1 := &i0
	i2 := &i1
	i3 := &i2
	xx := x{
		A:  y{X: "Loreum"},
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
		U1: "Loreum",
		U2: &l2,
		U3: nil,
		U4: false,
		U5: (*int)(nil), // typed nil
		U6: i3,          // chain of pointers
		u7: nil,
	}
	var buf bytes.Buffer
	if err := enc.Encode(xx, &buf); err != nil {
		t.Fatal(err)
	}
	if !equalStdLib(t, xx, buf.Bytes()) {
		t.Error("expected outputs to be equal")
	}
}

// TestEmbeddedStructs tests that named and unnamed
// embedded structs fields can be encoded.
func TestEmbeddedStructs(t *testing.T) {
	type r struct {
		J string `json:"j"`
	}
	type v struct {
		H bool   `json:"h,omitempty"`
		I string `json:"i"`
	}
	type y struct {
		D int8  `json:"d"`
		E uint8 `json:"e,omitempty"`
		r
		v
	}
	type z struct {
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
	type x1 struct {
		A string `json:"a,omitempty"`
		y
		B string `json:"b"`
		v `json:"v"`
		C string `json:"c,omitempty"`
		z `json:",omitempty"`
		*x1
	}
	enc, err := NewEncoder(x1{})
	if err != nil {
		t.Fatal(err)
	}
	xx1 := &x1{
		A: "Loreum",
		y: y{
			D: math.MinInt8,
			r: r{J: "Sit Amet"},
			v: v{H: false},
		},
		z: z{
			G: math.MaxUint16,
			y: y{D: 21, r: r{J: "Ipsem"}},
			v: v{H: true},
		},
		x1: &x1{
			A: "Muerol",
		},
	}
	var buf bytes.Buffer
	if err := enc.Encode(xx1, &buf); err != nil {
		t.Fatal(err)
	}
	if !equalStdLib(t, xx1, buf.Bytes()) {
		t.Error("expected outputs to be equal")
	}
	// xx is a variant of the x type with the first
	// field not using the omitempty option.
	type x2 struct {
		A int16 `json:"a"`
		v `json:"v"`
	}
	enc, err = NewEncoder(x2{})
	if err != nil {
		t.Fatal(err)
	}
	xx2 := &x2{A: 42, v: v{I: "Loreum"}}
	buf.Reset()
	if err := enc.Encode(xx2, &buf); err != nil {
		t.Fatal(err)
	}
	if !equalStdLib(t, xx2, buf.Bytes()) {
		t.Error("expected outputs to be equal")
	}
}

// TestAnonymousFields tests advanced cases for anonymous
// struct fields.
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
				X: "Loreum",
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
			f := F{E: &E{D: D{C: &C{B: B{A: &A{X1: s, X2: &s}}}}}}
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
			f := F{Z1: "Loreum", E: &E{D: &D{C: &C{B: B{A: A{X2: "Loreum"}, Z3: new(bool)}}, Z2: 1}}}
			return []interface{}{f, &f}
		},
	}}
	for _, tt := range testdata {
		tt := tt
		t.Run(tt.label, func(t *testing.T) {
			inputs := tt.input()
			for i, input := range inputs {
				input := input
				var label string
				if i == 0 {
					label = "non-pointer"
				} else {
					label = "pointer"
				}
				t.Run(label, func(t *testing.T) {
					enc, err := NewEncoder(input)
					if err != nil {
						t.Error(err)
					}
					var buf bytes.Buffer
					if err := enc.Encode(input, &buf); err != nil {
						t.Error(err)
					}
					if !equalStdLib(t, input, buf.Bytes()) {
						t.Error("expected outputs to be equal")
					}
				})
			}
		})
	}
}

// TestEmbeddedTypes tests that embedded struct
// fields of composite and primitive types are
// encoded whether they are exported.
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
	enc, err := NewEncoder(x{})
	if err != nil {
		t.Fatal(err)
	}
	xx := &x{
		P1: P1(42),
		P2: P2("Loreum"),
		P3: P3(true),
		p4: p4(math.MaxUint32),
		C1: C1{"A": 1, "B": 2},
		C2: C2{"A", "B", "C"},
		C3: C3{1, 2, 3},
		c4: c4{true, false},
	}
	var buf bytes.Buffer
	if err := enc.Encode(xx, &buf); err != nil {
		t.Error(err)
	}
	if !equalStdLib(t, xx, buf.Bytes()) {
		t.Error("expected outputs to be equal")
	}
}

// TestRecursiveType tests that recursive types
// can be encoded without entering a recursion hole
// when the encoder's instructions are generated.
func TestRecursiveType(t *testing.T) {
	type x struct {
		A string `json:"a"`
		X *x     `json:"x"`
	}
	xx := &x{
		A: "Loreum",
		X: &x{A: "Ipsem"},
	}
	enc, err := NewEncoder(x{})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := enc.Encode(xx, &buf); err != nil {
		t.Error(err)
	}
	if !equalStdLib(t, xx, buf.Bytes()) {
		t.Error("expected outputs to be equal")
	}
}

// TestJSONMarshaler tests that a type that implements
// the json.Marshaler interface is encoded using the
// result of its MarshalJSON method call result.
func TestJSONMarshaler(t *testing.T) {
	// Because the type big.Int also implements
	// the encoding.TextMarshaler interface, the
	// test ensure that MarshalJSON has priority.
	type x struct {
		B1 big.Int  `json:"b1"`
		B2 big.Int  `json:"b2,omitempty"`
		B3 *big.Int `json:"b3"`
		B4 *big.Int `json:"b4,omitempty"`
	}
	enc, err := NewEncoder(x{})
	if err != nil {
		t.Fatal(err)
	}
	xx := &x{
		B1: *big.NewInt(math.MaxInt64),
		B3: big.NewInt(math.MaxInt64),
		B4: nil,
	}
	var buf bytes.Buffer
	if err := enc.Encode(xx, &buf); err != nil {
		t.Error(err)
	}
	if !equalStdLib(t, xx, buf.Bytes()) {
		t.Error("expected outputs to be equal")
	}
}

// TestTextMarshaler tests that a type that implements
// the encoding.TextMarshaler interface encodes to a
// quoted string of its MashalText method call result.
func TestTextMarshaler(t *testing.T) {
	type x struct {
		IP1 net.IP  `json:"ip1"`
		IP2 net.IP  `json:"ip2,omitempty"`
		IP3 *net.IP `json:"ip3"`
		IP4 *net.IP `json:"ip4"`
		IP5 *net.IP `json:"ip5,omitempty"`
	}
	enc, err := NewEncoder(x{})
	if err != nil {
		t.Fatal(err)
	}
	xx := &x{
		IP1: net.IP{192, 168, 0, 1},
		IP3: &net.IP{127, 0, 0, 1},
	}
	var buf bytes.Buffer
	if err := enc.Encode(xx, &buf); err != nil {
		t.Error(err)
	}
	if !equalStdLib(t, xx, buf.Bytes()) {
		t.Error("expected outputs to be equal")
	}
}

// TestMarshalerError tests that a MarshalerError
// is returned when a MarshalText or a MarshalJSON
// method returns an error.
func TestMarshalerError(t *testing.T) {
	type x struct {
		InvalidIP net.IP
	}
	enc, err := NewEncoder(x{})
	if err != nil {
		t.Fatal(err)
	}
	xx := &x{
		// InvalidIP is not compliant with
		// net.IPv4len or net.IPv6len.
		InvalidIP: []byte{0, 0, 0, 0, 0},
	}
	var buf bytes.Buffer
	err = enc.Encode(xx, &buf)
	if err != nil {
		me, ok := err.(*MarshalerError)
		if !ok {
			t.Fatalf("got %T, want MarshalerError", err)
		}
		iptyp := reflect.TypeOf(net.IP{})
		if me.Typ != iptyp {
			t.Errorf("got %s, want %s", me.Typ, iptyp)
		}
		if me.Err == nil {
			t.Errorf("expected non-nil error")
		}
		if me.Error() == "" {
			t.Error("expected non-empty error message")
		}
	} else {
		t.Error("got nil, want non-nil error")
	}
}

// TestTime tests that a time.Time type can be
// encoded as a string with various layouts and
// as an integer representing a Unix timestamp.
func TestTime(t *testing.T) {
	s := "2009-07-12T11:03:25Z"

	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := NewEncoder(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer

	testdata := []struct {
		Layout string
		Str    string
	}{
		{time.RFC3339, `"2009-07-12T11:03:25Z"`},
		{time.RFC1123Z, `"Sun, 12 Jul 2009 11:03:25 +0000"`},
		{time.RFC822Z, `"12 Jul 09 11:03 +0000"`},
	}
	for _, tt := range testdata {
		buf.Reset()
		if err := enc.Encode(&tm, &buf, TimeLayout(tt.Layout)); err != nil {
			t.Error(err)
		}
		if s := buf.String(); s != tt.Str {
			t.Errorf("for layout `%s`, got %s, want %s", tt.Layout, s, tt.Str)
		}
	}
	buf.Reset()
	if err := enc.Encode(&tm, &buf, UnixTimestamp); err != nil {
		t.Error(err)
	}
	if s, want := buf.String(), "1247396605"; s != want {
		t.Errorf("got %s, want %s", s, want)
	}
	// Special case to test error when the year
	// of the date is outside of range [0.9999].
	// see golang.org/issue/4556#c15.
	for _, tm := range []time.Time{
		time.Date(-1, time.January, 1, 0, 0, 0, 0, time.UTC),
		time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC),
	} {
		if err := enc.Encode(tm, &buf); err == nil {
			t.Error("got nil, expected non-nil error")
		}
	}
}

// TestDuration tests that a time.Duration type
// can be encoded in multiple representations.
func TestDuration(t *testing.T) {
	s := "1h3m40s"

	d, err := time.ParseDuration(s)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := NewEncoder(time.Duration(0))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer

	testdata := []struct {
		Fmt DurationFmt
		Str string
	}{
		{DurationString, strconv.Quote(s)},
		{DurationMinutes, "63.666666666666664"},
		{DurationSeconds, "3820"},
		{DurationMilliseconds, "3820000"},
		{DurationMicroseconds, "3820000000"},
		{DurationNanoseconds, "3820000000000"},
	}
	for _, tt := range testdata {
		buf.Reset()
		if err := enc.Encode(&d, &buf, DurationFormat(tt.Fmt)); err != nil {
			t.Error(err)
		}
		if s := buf.String(); s != tt.Str {
			t.Errorf("for %s, got %s, want %s", tt.Fmt, s, tt.Str)
		}
	}
}

// TestByteArray tests that that a byte array can
// be encoded either as a JSON array or as a JSON
// string with the ByteArrayAsString option.
func TestByteArray(t *testing.T) {
	var (
		a byte = 'a'
		b byte = 'b'
		c byte = 'c'
	)
	testdata := []struct {
		Val interface{}
		Str string
		Raw bool
	}{
		{[3]byte{'a', 'b', 'c'}, "[97,98,99]", false},
		{[3]byte{'d', 'e', 'f'}, `"def"`, true},
		{[3]*byte{&a, &b, &c}, "[97,98,99]", true},
		{[3]*byte{&a, &b, &c}, "[97,98,99]", false},
	}
	for _, td := range testdata {
		var opts []Option
		if td.Raw {
			opts = append(opts, ByteArrayAsString)
		}
		enc, err := NewEncoder(td.Val)
		if err != nil {
			t.Error(err)
		}
		var buf bytes.Buffer
		if err := enc.Encode(td.Val, &buf, opts...); err != nil {
			t.Error(err)
		}
		if s := buf.String(); s != td.Str {
			t.Errorf("got %s, want %s", s, td.Str)
		}
	}
}

// TestByteSliceVariousSizes tests that a byte slice
// of various size encodes as a base64 string by default.
func TestByteSliceVariousSizes(t *testing.T) {
	for _, s := range []int{
		0, 64, 128, 1024, 2048,
	} {
		size := s
		t.Run(fmt.Sprintf("size: %d", size), func(t *testing.T) {
			b := make([]byte, size)
			if _, err := rand.Read(b); err != nil {
				t.Fatal(err)
			}
			enc, err := NewEncoder([]byte{})
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if err := enc.Encode(&b, &buf); err != nil {
				t.Error(err)
			}
			if !equalStdLib(t, &b, buf.Bytes()) {
				t.Error("expected outputs to be equal")
			}
		})
	}
}

// TestRenamedByteSlice tests that a name type
// that represents a slice of bytes is encoded
// the same way as a regular byte slice.
func TestRenamedByteSlice(t *testing.T) {
	type (
		b  byte
		b1 []byte
		b2 []b
	)
	testdata := []interface{}{
		b1("Loreum"),
		b2("Loreum"),
	}
	for _, tt := range testdata {
		enc, err := NewEncoder(tt)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := enc.Encode(tt, &buf); err != nil {
			t.Error(err)
		}
		if !equalStdLib(t, tt, buf.Bytes()) {
			t.Error("expected outputs to be equal")
		}
	}
}

// TestByteSliceAsRawString tests that that a byte
// slice can be encoded as a raw JSON string when
// the DisableBase64Slice option is set.
func TestByteSliceAsRawString(t *testing.T) {
	b := []byte("Loreum")

	enc, err := NewEncoder(b)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := enc.Encode(b, &buf, RawByteSlices); err != nil {
		t.Error(err)
	}
	want := strconv.Quote(string(b))
	if s := buf.String(); s != want {
		t.Errorf("got %s, want %s", s, want)
	}
}

// TestInvalidFloatValues tests that encoding an
// invalid float value returns UnsupportedValueError.
func TestInvalidFloatValues(t *testing.T) {
	enc, err := NewEncoder(float64(0))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []float64{
		math.NaN(),
		math.Inf(-1),
		math.Inf(1),
	} {
		v := v
		err := enc.Encode(&v, nil)
		if err != nil {
			_, ok := err.(*UnsupportedValueError)
			if !ok {
				t.Errorf("got %T, want UnsupportedValueError", err)
			}
		} else {
			t.Error("got nil, want non-nil error")
		}
	}
}

// TestStringEscaping tests that control and reserved
// JSON characters are properly escaped when encoding
// a string.
func TestStringEscaping(t *testing.T) {
	b := []byte{'A', 1, 2, 3, '"', '\\', '/', 'B', 'C', '\b', '\f', '\n', '\r', '\t'}
	testdata := []struct {
		Bts  []byte
		Want string
		NSE  bool // NoStringEscaping
	}{
		{b, `"A\u0001\u0002\u0003\"\\\/BC\b\f\n\r\t"`, false},
		{b, `"` + string(b) + `"`, true},
	}
	for _, tt := range testdata {
		s := string(tt.Bts)
		enc, err := NewEncoder(s)
		if err != nil {
			t.Fatal(err)
		}
		var opts []Option
		if tt.NSE {
			opts = append(opts, NoStringEscaping)
		}
		var buf bytes.Buffer
		if err := enc.Encode(&s, &buf, opts...); err != nil {
			t.Error(err)
		}
		if s := buf.String(); s != tt.Want {
			t.Errorf("got %#q, want %#q", s, tt.Want)
		}
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
		{"\x08", `"\b"`},
		{"\x09", `"\t"`},
		{"\x0a", `"\n"`},
		{"\x0b", `"\u000b"`},
		{"\x0c", `"\f"`},
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
		enc, err := NewEncoder(tt.in)
		if err != nil {
			t.Error(err)
		}
		var buf bytes.Buffer
		if err := enc.Encode(tt.in, &buf); err != nil {
			t.Error(err)
			continue
		}
		if s := buf.String(); s != tt.out {
			t.Errorf("got %#q, want %#q", s, tt.out)
		}
	}
}

// TestTaggedFieldDominates tests that a field with
// a tag dominates untagged fields.
// Taken from encoding/json.
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
	y := Y{A{"Loreum"}, D{"Ipsum"}}

	enc, err := NewEncoder(Y{})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := enc.Encode(y, &buf); err != nil {
		t.Fatal(err)
	}
	if !equalStdLib(t, y, buf.Bytes()) {
		t.Error("expected outputs to be equal")
	}
}

// TestDuplicatedFieldDisappears tests that duplicate
// field at the same level of embedding are ignored.
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
		// There are no tags here,
		// so S should not appear.
		Z struct {
			A
			C
			// Y contains a tagged S field through B,
			// it should not dominate.
			Y
		}
	)
	z := Z{
		A{"Loreum"},
		C{"Ipsum"},
		Y{A{"Sit"}, D{"Amet"}},
	}
	enc, err := NewEncoder(Z{})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := enc.Encode(z, &buf); err != nil {
		t.Fatal(err)
	}
	if !equalStdLib(t, z, buf.Bytes()) {
		t.Error("expected outputs to be equal")
	}
}

// TestJSONNumber tests that a json.Number literal value
// can be encoded, and that an error is returned if it
// isn't a valid number according to the JSON grammar.
func TestJSONNumber(t *testing.T) {
	testdata := []struct {
		Number  json.Number
		Want    string
		IsValid bool
	}{
		{json.Number("42"), "42", true},
		{json.Number("-42"), "-42", true},
		{json.Number("24.42"), "24.42", true},
		{json.Number("-666.66"), "-666.66", true},
		{json.Number("3.14"), "3.14", true},
		{json.Number("-3.14"), "-3.14", true},
		{json.Number("1e3"), "1e3", true},
		{json.Number("1E-6"), "1E-6", true},
		{json.Number("1E+42"), "1E+42", true},
		{json.Number("1E+4.0"), "", false},
		{json.Number("084"), "", false},
		{json.Number("-03.14"), "", false},
		{json.Number("-"), "", false},
		{json.Number(""), "", false},
		{json.Number("invalid"), "", false},
	}
	for _, tt := range testdata {
		enc, err := NewEncoder(tt.Number)
		if err != nil {
			t.Error(err)
		}
		var buf bytes.Buffer
		err = enc.Encode(&tt.Number, &buf)
		if err != nil && tt.IsValid {
			t.Error(err)
			continue
		}
		if err == nil && !tt.IsValid {
			t.Errorf("for %s, expected non-nil error", tt.Number)
			continue
		}
		if s := buf.String(); s != tt.Want {
			t.Errorf("got %s, want %s", s, tt.Want)
		}
	}
}

// TestDurationFmtString tests that the String method of
// the DurationFmt type returns the appropriate description.
func TestDurationFmtString(t *testing.T) {
	testdata := []struct {
		Fmt DurationFmt
		Str string
	}{
		{DurationString, "str"},
		{DurationMinutes, "min"},
		{DurationSeconds, "s"},
		{DurationMilliseconds, "ms"},
		{DurationMicroseconds, "μs"},
		{DurationNanoseconds, "nanosecond"},
		{DurationFmt(-1), "unknown"},
		{DurationFmt(6), "unknown"},
	}
	for _, tt := range testdata {
		if s := tt.Fmt.String(); s != tt.Str {
			t.Errorf("got %s, want %s", s, tt.Str)
		}
	}
}

// equalStdLib marshals i to JSON using the encoding/json
// package and returns whether the output equals b.
func equalStdLib(t *testing.T, i interface{}, b []byte) bool {
	sb, err := json.Marshal(i)
	if err != nil {
		t.Error(err)
	}
	t.Logf("standard: %s", string(sb))
	t.Logf("jettison: %s", string(b))

	return bytes.Equal(sb, b)
}

type simplePayload struct {
	St   int    `json:"st"`
	Sid  int    `json:"sid"`
	Tt   string `json:"tt"`
	Gr   int    `json:"gr"`
	UUID string `json:"uuid"`
	IP   string `json:"ip"`
	Ua   string `json:"ua"`
	Tz   int    `json:"tz"`
	V    bool   `json:"v"`
}

func (*simplePayload) NKeys() int    { return 9 }
func (t *simplePayload) IsNil() bool { return t == nil }

func (t *simplePayload) MarshalJSONObject(enc *gojay.Encoder) {
	enc.AddIntKey("st", t.St)
	enc.AddIntKey("sid", t.Sid)
	enc.AddStringKey("tt", t.Tt)
	enc.AddIntKey("gr", t.Gr)
	enc.AddStringKey("uuid", t.UUID)
	enc.AddStringKey("ip", t.IP)
	enc.AddStringKey("ua", t.Ua)
	enc.AddIntKey("tz", t.Tz)
	enc.AddBoolKey("v", t.V)
}

func BenchmarkSimplePayload(b *testing.B) {
	enc, err := NewEncoder(simplePayload{})
	if err != nil {
		b.Fatal(err)
	}
	if err := enc.Compile(); err != nil {
		b.Fatal(err)
	}
	sp := &simplePayload{
		St:   1,
		Sid:  2,
		Tt:   "TestString",
		Gr:   4,
		UUID: "8f9a65eb-4807-4d57-b6e0-bda5d62f1429",
		IP:   "127.0.0.1",
		Ua:   "Mozilla",
		Tz:   8,
		V:    true,
	}
	b.Run("standard", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			bts, err := json.Marshal(sp)
			if err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(bts)))
		}
	})
	b.Run("jsoniter", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			bts, err := jsoniterStd.Marshal(sp)
			if err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(bts)))
		}
	})
	b.Run("gojay", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			bts, err := gojay.MarshalJSONObject(sp)
			if err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(bts)))
		}
	})
	b.Run("jettison", func(b *testing.B) {
		var buf bytes.Buffer
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if err := enc.Encode(sp, &buf); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(buf.Len()))
			buf.Reset()
		}
	})
}

func BenchmarkComplexPayload(b *testing.B) {
	type y struct {
		X string `json:"x"`
	}
	type x struct {
		A  y `json:"a"`
		B1 *y
		B2 *y
		C  []string     `json:"c"`
		D  []int        `json:"d"`
		E  []bool       `json:"e"`
		F  []float32    `json:"f,omitempty"`
		G  []*uint      `json:"g"`
		H  [3]string    `json:"h"`
		I  [1]int       `json:"i,omitempty"`
		J  [0]bool      `json:"j"`
		K  []byte       `json:"k"`
		L  []*int       `json:"l"`
		M1 []y          `json:"m1"`
		M2 *[]y         `json:"m2"`
		N  []*y         `json:"n"`
		O1 [3]*int      `json:"o1"`
		O2 *[3]*bool    `json:"o2,omitempty"`
		P  [3]*y        `json:"p"`
		Q  [][]int      `json:"q"`
		R  [2][2]string `json:"r"`
	}
	enc, err := NewEncoder(x{})
	if err != nil {
		b.Fatal(err)
	}
	if err := enc.Compile(); err != nil {
		b.Fatal(err)
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		b.Fatal(err)
	}
	var (
		l1, l2 = 0, 42
		m1, m2 = y{X: "Loreum"}, y{}
	)
	xx := &x{
		A:  y{X: "Loreum"},
		B1: nil,
		B2: &y{X: "Ipsum"},
		C:  []string{"one", "two", "three"},
		D:  []int{1, 2, 3},
		E:  []bool{},
		H:  [3]string{"alpha", "beta", "gamma"},
		I:  [1]int{42},
		K:  k,
		L:  []*int{&l1, &l2, nil},
		M1: []y{m1, m2},
		N:  []*y{&m1, &m2, nil},
		O1: [3]*int{&l1, &l2, nil},
		P:  [3]*y{&m1, &m2, nil},
		Q:  [][]int{{1, 2}, {3, 4}},
		R:  [2][2]string{{"a", "b"}, {"c", "d"}},
	}
	b.Run("standard", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bts, err := json.Marshal(xx)
			if err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(bts)))
		}
	})
	b.Run("jsoniter", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bts, err := jsoniterStd.Marshal(xx)
			if err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(bts)))
		}
	})
	b.Run("jettison", func(b *testing.B) {
		var buf bytes.Buffer
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := enc.Encode(xx, &buf); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(buf.Len()))
			buf.Reset()
		}
	})
}

func BenchmarkInterface(b *testing.B) {
	s := "Loreum"
	var iface interface{} = &s
	enc, err := NewEncoder(iface)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("standard", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bts, err := json.Marshal(iface)
			if err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(bts)))
		}
	})
	b.Run("jsoniter", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bts, err := jsoniterStd.Marshal(iface)
			if err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(bts)))
		}
	})
	b.Run("jettison", func(b *testing.B) {
		var buf bytes.Buffer
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := enc.Encode(iface, &buf); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(buf.Len()))
			buf.Reset()
		}
	})
}

func BenchmarkMap(b *testing.B) {
	m := map[string]int{
		"a": 1,
		"b": 2,
		"c": 3,
	}
	b.Run("standard", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bts, err := json.Marshal(m)
			if err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(bts)))
		}
	})
	b.Run("jsoniter", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bts, err := jsoniterStd.Marshal(m)
			if err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(bts)))
		}
	})
	b.Run("jettison", func(b *testing.B) {
		enc, err := NewEncoder(m)
		if err != nil {
			b.Fatal(err)
		}
		b.Run("sort", benchMap(enc, m))
		b.Run("nosort", benchMap(enc, m, UnsortedMap))
	})
}

func benchMap(enc *Encoder, m map[string]int, opts ...Option) func(b *testing.B) {
	var buf bytes.Buffer
	return func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := enc.Encode(&m, &buf, opts...); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(buf.Len()))
			buf.Reset()
		}
	}
}
