package jettison

import (
	"bytes"
	"crypto/rand"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestNewEncoderNilInterface tests that creating a new
// encoder for a nil interface returns an error.
func TestNewEncoderNilInterface(t *testing.T) {
	_, err := NewEncoder(nil)
	if err == nil {
		t.Error("expected non-nil error")
	}
}

// TestInvalidWriter tests that invoking the Encode
// method of an encoder with an invalid writer does
// return an error.
func TestInvalidWriter(t *testing.T) {
	enc, err := NewEncoder(reflect.TypeOf(""))
	if err != nil {
		t.Fatal(err)
	}
	err = enc.Encode("", nil)
	if err != nil {
		if err != ErrInvalidWriter {
			t.Errorf("got %T, want ErrInvalidWriter", err)
		}
	} else {
		t.Error("expected non-nil error")
	}
}

// TestEncodeWithIncompatibleType tests that invoking the
// Encode method of an encoder with a type that differs from
// the one for which is was created returns an error.
func TestEncodeWithIncompatibleType(t *testing.T) {
	type (
		x struct{}
		y struct{}
	)
	enc, err := NewEncoder(reflect.TypeOf(x{}))
	if err != nil {
		t.Fatal(err)
	}
	err = enc.Encode(y{}, &bytes.Buffer{})
	if err != nil {
		tme, ok := err.(*TypeMismatchError)
		if !ok {
			t.Fatalf("got %T, want TypeMismatchError", err)
		}
		if s := tme.Error(); s == "" {
			t.Errorf("want non empty error message")
		}
		if tox := reflect.TypeOf(x{}); tme.EncType != tox {
			t.Errorf("got %s, want %s", tme.EncType, tox)
		}
		if toy := reflect.TypeOf(y{}); tme.SrcType != toy {
			t.Errorf("got %s, want %s", tme.SrcType, toy)
		}
	} else {
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
	enc, err := NewEncoder(reflect.TypeOf(int(0)))
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
		Val interface{}
		Str string
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
		enc, err := NewEncoder(reflect.TypeOf(tt.Val))
		if err != nil {
			t.Error(err)
		}
		var buf bytes.Buffer
		if err := enc.Encode(tt.Val, &buf); err != nil {
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
		Val interface{}
		Str string
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
		enc, err := NewEncoder(reflect.TypeOf(tt.Val))
		if err != nil {
			t.Error(err)
		}
		var buf bytes.Buffer
		if err := enc.Encode(tt.Val, &buf); err != nil {
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
		enc, _ := NewEncoder(reflect.TypeOf(tt))
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
		enc, _ := NewEncoder(reflect.TypeOf(tt))
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
		enc, err := NewEncoder(reflect.TypeOf(tt.Val))
		if err != nil {
			t.Error(err)
		}
		var buf bytes.Buffer
		var opts []Option
		if tt.NoSort {
			opts = append(opts, UnsortedMap())
		}
		if tt.NME {
			opts = append(opts, NilMapEmpty())
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

type (
	mapKeyString           string
	mapKeyInteger          int64
	mapKeyMarshaler        struct{}
	mapKeyStringMarshaler  string
	mapKeyIntegerMarshaler uint64
)

func (mapKeyMarshaler) MarshalText() ([]byte, error) {
	return []byte("loreum"), nil
}
func (mapKeyStringMarshaler) MarshalText() ([]byte, error) {
	return []byte("ipsum"), nil
}
func (mapKeyIntegerMarshaler) MarshalText() ([]byte, error) {
	return []byte("dolor"), nil
}

// TestMapKeyPrecedence tests that the precedence order
// of map key types is respected during marshaling. It is
// defined by the json.Marshal documentation as:
// - any string type
// - encoding.TextMarshaler
// - any integer type
func TestMapKeyPrecedence(t *testing.T) {
	testdata := []interface{}{
		map[mapKeyString]string{"loreum": "ipsum"},
		map[mapKeyInteger]string{1: "loreum"},
		map[mapKeyMarshaler]string{{}: "ipsum"},
		map[mapKeyStringMarshaler]string{mapKeyStringMarshaler("xxx"): "loreum"},
		map[mapKeyIntegerMarshaler]string{mapKeyIntegerMarshaler(42): "ipsum"},
	}
	for _, tt := range testdata {
		enc, err := NewEncoder(reflect.TypeOf(tt))
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
		enc, err := NewEncoder(reflect.TypeOf(tt.Val))
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		var opts []Option
		if tt.NME {
			opts = append(opts, NilSliceEmpty())
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
		enc, err := NewEncoder(reflect.TypeOf(tt))
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

type (
	intTextMarshaler    int
	intPtrTextMarshaler int
	strJSONMarshaler    string
	strPtrJSONMarshaler string
)

func (im intTextMarshaler) MarshalText() ([]byte, error) {
	return []byte(strconv.Itoa(int(im))), nil
}
func (im *intPtrTextMarshaler) MarshalText() ([]byte, error) {
	return []byte(strconv.Itoa(int(*im))), nil
}
func (sm strJSONMarshaler) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(string(sm))), nil
}
func (sm *strPtrJSONMarshaler) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(string(*sm))), nil
}

type (
	structTextMarshaler    struct{ L, R string }
	structPtrTextMarshaler struct{ L, R string }
)

func (sm structTextMarshaler) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("%s:%s", sm.L, sm.R)), nil
}
func (spm *structPtrTextMarshaler) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("%s:%s", spm.L, spm.R)), nil
}

// TestTextMarshalerMapKey tests that a map with
// key types implemeting the text.Marshaler interface
// can be encoded.
func TestTextMarshalerMapKey(t *testing.T) {
	var (
		im  intTextMarshaler    = 42
		ipm intPtrTextMarshaler = 84
		sm                      = structTextMarshaler{L: "A", R: "B"}
		spm                     = structPtrTextMarshaler{L: "A", R: "B"}
		ip                      = &net.IP{127, 0, 0, 1}
	)
	valid := []interface{}{
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
		map[structTextMarshaler]string{sm: "ab"},
		map[*structTextMarshaler]string{
			&sm: "ab",
			// nil: "",
		},
		map[*structPtrTextMarshaler]string{
			&spm: "ab",
			// nil: "",
		},
		map[intTextMarshaler]string{im: "42"},
		map[*intTextMarshaler]string{
			&im: "42",
			// nil: "",
		},
		map[intPtrTextMarshaler]string{ipm: "42"},
		map[*intPtrTextMarshaler]string{
			&ipm: "42",
			// nil: "",
		},
	}
	for _, tt := range valid {
		enc, err := NewEncoder(reflect.TypeOf(tt))
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

func TestInvalidTextMarshalerMapKey(t *testing.T) {
	for _, tt := range []interface{}{
		// Non-pointer value of a pointer-receiver
		// type isn't a valid map key type.
		map[structPtrTextMarshaler]string{
			{L: "A", R: "B"}: "ab",
		},
	} {
		enc, err := NewEncoder(reflect.TypeOf(tt))
		if err != nil {
			t.Error(err)
		}
		var buf bytes.Buffer
		err = enc.Encode(tt, &buf)
		_, jsonErr := json.Marshal(tt)

		// Trim the prefix of the JSON error string,
		// and compare with the error returned by
		// Jettison.
		s := strings.TrimPrefix(jsonErr.Error(), "json: ")
		if s != err.Error() {
			t.Errorf("got %s, want %s", s, err.Error())
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
	enc, err := NewEncoder(reflect.TypeOf((*x)(nil)).Elem())
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
	enc, err := NewEncoder(reflect.TypeOf((*x)(nil)).Elem())
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
	enc, err := NewEncoder(reflect.TypeOf(x{}))
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
	enc, err := NewEncoder(reflect.TypeOf((*x)(nil)).Elem())
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

// TestStructFieldNameHTMLEscaping tests that HTML
// characters inside struct field names are escaped.
func TestStructFieldNameHTMLEscaping(t *testing.T) {
	type Y struct {
		S string
	}
	type x struct {
		A int  `json:"ben&jerry"`
		B *int `json:"a>2"`
		C struct {
			*Y `json:"6<b"`
		}
		D bool `json:""`
	}
	enc, err := NewEncoder(reflect.TypeOf(x{}))
	if err != nil {
		t.Fatal(err)
	}
	xx := &x{}

	for _, opt := range []Option{nil, NoHTMLEscaping()} {
		var buf1, buf2 bytes.Buffer
		if err := enc.Encode(xx, &buf1, opt); err != nil {
			t.Fatal(err)
		}
		jenc := json.NewEncoder(&buf2)
		if opt != nil {
			jenc.SetEscapeHTML(false)
		}
		if err := jenc.Encode(xx); err != nil {
			t.Fatal(err)
		}
		jettison := buf1.String()
		// json.Encoder.Encode returns the JSON
		// encoding of the given value followed
		// by a newline character.
		standard := strings.TrimSuffix(buf2.String(), "\n")

		t.Logf("standard: %s", standard)
		t.Logf("jettison: %s", jettison)

		if jettison != standard {
			t.Error("expected outputs to be equal")
		}
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
	enc, err := NewEncoder(reflect.TypeOf(x{}))
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
	enc, err := NewEncoder(reflect.TypeOf((*x)(nil)).Elem())
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
	enc, err := NewEncoder(reflect.TypeOf(x{}))
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
	enc, err := NewEncoder(reflect.TypeOf(x1{}))
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
	// x2 is a variant of the x1 type with the first
	// field not using the omitempty option.
	type x2 struct {
		A int16 `json:"a"`
		v `json:"v"`
	}
	enc, err = NewEncoder(reflect.TypeOf(x2{}))
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
					enc, err := NewEncoder(reflect.TypeOf(input))
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
	enc, err := NewEncoder(reflect.TypeOf(x{}))
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
	enc, err := NewEncoder(reflect.TypeOf(x{}))
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

// TestJSONMarshaler tests that a type implementing
// the json.Marshaler interface is encoded using the
// result of its MarshalJSON method call result.
// Because the types big.Int and time.Time also
// implements the encoding.TextMarshaler interface,
// the test ensures that MarshalJSON has priority.
func TestJSONMarshaler(t *testing.T) {
	// T = Non-pointer receiver of composite type.
	// S = Non-pointer receiver of primitive type.
	// I = Pointer receiver of composite type.
	// P = Pointer receiver of primitive type.
	type x struct {
		T1 time.Time            `json:"t1"`
		T2 time.Time            `json:"t2,omitempty"`
		T3 *time.Time           `json:"t3"`
		T4 *time.Time           `json:"t4"`           // nil
		T5 *time.Time           `json:"t5,omitempty"` // nil
		S1 strJSONMarshaler     `json:"s1,omitempty"`
		S2 strJSONMarshaler     `json:"s2,omitempty"`
		S3 strJSONMarshaler     `json:"s3"`
		S4 *strJSONMarshaler    `json:"s4"`
		S5 *strJSONMarshaler    `json:"s5"`           // nil
		S6 *strJSONMarshaler    `json:"s6,omitempty"` // nil
		I1 big.Int              `json:"i1"`
		I2 big.Int              `json:"i2,omitempty"`
		I3 *big.Int             `json:"i3"`
		I4 *big.Int             `json:"i4"`           // nil
		I5 *big.Int             `json:"i5,omitempty"` // nil
		P1 strPtrJSONMarshaler  `json:"p1,omitempty"`
		P2 strPtrJSONMarshaler  `json:"p2,omitempty"`
		P3 strPtrJSONMarshaler  `json:"p3"`
		P4 *strPtrJSONMarshaler `json:"p4"`
		P5 *strPtrJSONMarshaler `json:"p5"`           // nil
		P6 *strPtrJSONMarshaler `json:"p6,omitempty"` // nil
	}
	enc, err := NewEncoder(reflect.TypeOf(x{}))
	if err != nil {
		t.Fatal(err)
	}
	var (
		now = time.Now()
		sm  = strJSONMarshaler("Loreum")
		spm = strPtrJSONMarshaler("Loreum")
	)
	xx := x{
		T1: now,
		T3: &now,
		S1: "Loreum",
		S4: &sm,
		I1: *big.NewInt(math.MaxInt64),
		I3: big.NewInt(math.MaxInt64),
		P1: "Loreum",
		P4: &spm,
	}
	testdata := []struct {
		name string
		val  interface{}
	}{
		{"non-pointer", xx},
		{"pointer", &xx},
	}
	for _, tt := range testdata {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := enc.Encode(tt.val, &buf); err != nil {
				t.Error(err)
			}
			if !equalStdLib(t, tt.val, buf.Bytes()) {
				t.Error("expected outputs to be equal")
			}
		})
	}
}

// TestTextMarshaler tests that a type that implements
// the encoding.TextMarshaler interface encodes to a
// quoted string of its MashalText method call result.
func TestTextMarshaler(t *testing.T) {
	// S = Non-pointer receiver of composite type.
	// I = Non-pointer receiver of primitive type.
	// F = Pointer receiver of composite kind.
	// P = Pointer receiver of primitive type.
	type x struct {
		S1 net.IP               `json:"s1"`
		S2 net.IP               `json:"s2,omitempty"`
		S3 *net.IP              `json:"s3"`
		S4 *net.IP              `json:"s4"`           // nil
		S5 *net.IP              `json:"s5,omitempty"` // nil
		I1 intTextMarshaler     `json:"i1,omitempty"`
		I2 intTextMarshaler     `json:"i2,omitempty"`
		I3 intTextMarshaler     `json:"i3"`
		I4 *intTextMarshaler    `json:"i4"`
		I5 *intTextMarshaler    `json:"i5"`           // nil
		I6 *intTextMarshaler    `json:"i6,omitempty"` // nil
		F1 big.Float            `json:"f1"`
		F2 big.Float            `json:"f2,omitempty"`
		F3 *big.Float           `json:"f3"`
		F4 *big.Float           `json:"f4"`           // nil
		F5 *big.Float           `json:"f5,omitempty"` // nil
		P1 intPtrTextMarshaler  `json:"p1,omitempty"`
		P2 intPtrTextMarshaler  `json:"p2,omitempty"`
		P3 intPtrTextMarshaler  `json:"p3"`
		P4 *intPtrTextMarshaler `json:"p4"`
		P5 *intPtrTextMarshaler `json:"p5"`           // nil
		P6 *intPtrTextMarshaler `json:"p6,omitempty"` // nil
	}
	enc, err := NewEncoder(reflect.TypeOf((*x)(nil)).Elem())
	if err != nil {
		t.Fatal(err)
	}
	var (
		im  = intTextMarshaler(42)
		ipm = intPtrTextMarshaler(42)
	)
	xx := x{
		S1: net.IP{192, 168, 0, 1},
		S3: &net.IP{127, 0, 0, 1},
		I1: 42,
		I4: &im,
		F1: *big.NewFloat(math.MaxFloat64),
		F3: big.NewFloat(math.MaxFloat64),
		P1: 42,
		P4: &ipm,
	}
	testdata := []struct {
		name string
		val  interface{}
	}{
		{"non-pointer", xx},
		{"pointer", &xx},
	}
	for _, tt := range testdata {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := enc.Encode(tt.val, &buf); err != nil {
				t.Error(err)
			}
			if !equalStdLib(t, tt.val, buf.Bytes()) {
				t.Error("expected outputs to be equal")
			}
		})
	}
}

type (
	nilJSONMarshaler string
	nilTextMarshaler string
	nilMarshaler     string
)

func (nm *nilJSONMarshaler) MarshalJSON() ([]byte, error) {
	if nm == nil {
		return []byte(strconv.Quote("Loreum")), nil
	}
	return nil, nil
}

func (nm *nilTextMarshaler) MarshalText() ([]byte, error) {
	if nm == nil {
		return []byte("Loreum"), nil
	}
	return nil, nil
}

func (nm *nilMarshaler) WriteJSON(w Writer) error {
	if nm == nil {
		_, err := w.WriteString(`"Loreum"`)
		return err
	}
	return nil
}

func (nm *nilMarshaler) MarshalJSON() ([]byte, error) {
	if nm == nil {
		return []byte(strconv.Quote("Loreum")), nil
	}
	return nil, nil
}

// bothMarshaler combines the json.Marshaler and
// the Marshaler interfaces so that the tests output
// can be compared against encoding/json.
type bothMarshaler interface {
	Marshaler
	json.Marshaler
}

// TestNilMarshaler tests that even if a nil interface
// value is passed in, as long as it implements MarshalJSON,
// it should be marshaled.
func TestNilMarshaler(t *testing.T) {
	testdata := []struct {
		v interface{}
	}{
		{v: struct{ M json.Marshaler }{M: nil}},
		{v: struct{ M json.Marshaler }{(*nilJSONMarshaler)(nil)}},
		{v: struct{ M interface{} }{(*nilJSONMarshaler)(nil)}},
		{v: struct{ M *nilJSONMarshaler }{M: nil}},
		{v: json.Marshaler((*nilJSONMarshaler)(nil))},
		{v: (*nilJSONMarshaler)(nil)},

		// FIXME: Panic with encoding/json.
		// {v: struct{ M encoding.TextMarshaler }{M: nil}},

		{v: struct{ M encoding.TextMarshaler }{(*nilTextMarshaler)(nil)}},
		{v: struct{ M interface{} }{(*nilTextMarshaler)(nil)}},
		{v: struct{ M *nilTextMarshaler }{M: nil}},
		{v: encoding.TextMarshaler((*nilTextMarshaler)(nil))},
		{v: (*nilTextMarshaler)(nil)},

		{v: struct{ M bothMarshaler }{M: nil}},
		{v: struct{ M bothMarshaler }{(*nilMarshaler)(nil)}},
		{v: struct{ M interface{} }{(*nilMarshaler)(nil)}},
		{v: struct{ M *nilMarshaler }{M: nil}},
		{v: bothMarshaler((*nilMarshaler)(nil))},
		{v: (*nilMarshaler)(nil)},
	}
	for _, tt := range testdata {
		enc, err := NewEncoder(reflect.TypeOf(tt.v))
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := enc.Encode(tt.v, &buf); err != nil {
			t.Fatal(err)
		}
		if !equalStdLib(t, tt.v, buf.Bytes()) {
			t.Error("expected outputs to be equal")
		}
	}
}

var errMarshaler = errors.New("")

type (
	errorJSONMarshaler    struct{}
	errorPtrJSONMarshaler struct{}
	errorTextMarshaler    struct{}
	errorPtrTextMarshaler struct{}
	errorMarshaler        struct{}
	errorPtrMarshaler     struct{}
)

func (errorJSONMarshaler) MarshalJSON() ([]byte, error)     { return nil, errMarshaler }
func (*errorPtrJSONMarshaler) MarshalJSON() ([]byte, error) { return nil, errMarshaler }

func (errorTextMarshaler) MarshalText() ([]byte, error)     { return nil, errMarshaler }
func (*errorPtrTextMarshaler) MarshalText() ([]byte, error) { return nil, errMarshaler }

func (errorMarshaler) WriteJSON(_ Writer) error     { return errMarshaler }
func (*errorPtrMarshaler) WriteJSON(_ Writer) error { return errMarshaler }

// TestMarshalerError tests that a MarshalerError
// is returned when a MarshalText or a MarshalJSON
// method returns an error.
func TestMarshalerError(t *testing.T) {
	for _, tt := range []interface{}{
		errorJSONMarshaler{},
		&errorPtrJSONMarshaler{},
		errorTextMarshaler{},
		&errorPtrTextMarshaler{},
		errorMarshaler{},
		&errorPtrMarshaler{},
	} {
		enc, err := NewEncoder(reflect.TypeOf(tt))
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		err = enc.Encode(tt, &buf)
		if err != nil {
			me, ok := err.(*MarshalerError)
			if !ok {
				t.Fatalf("got %T, want MarshalerError", err)
			}
			typ := reflect.TypeOf(tt)
			if me.Typ != typ {
				t.Errorf("got %s, want %s", me.Typ, typ)
			}
			if err := me.Unwrap(); err == nil {
				t.Errorf("expected non-nil error")
			}
			if me.Error() == "" {
				t.Error("expected non-empty error message")
			}
		} else {
			t.Error("got nil, want non-nil error")
		}
	}
}

type (
	stringMarshaler    string
	stringPtrMarshaler string
	structMarshaler    struct{}
	structPtrMarshaler struct{}
)

func (wm stringMarshaler) WriteJSON(w Writer) error {
	_, err := w.WriteString(strconv.Quote(string(wm)))
	return err
}
func (wm stringMarshaler) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(string(wm))), nil
}
func (wm *stringPtrMarshaler) WriteJSON(w Writer) error {
	_, err := w.WriteString(strconv.Quote(string(*wm)))
	return err
}
func (wm *stringPtrMarshaler) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(string(*wm))), nil
}
func (structMarshaler) WriteJSON(w Writer) error {
	_, err := w.WriteString(`"Loreum"`)
	return err
}
func (structMarshaler) MarshalJSON() ([]byte, error) {
	return []byte(`"Loreum"`), nil
}
func (*structPtrMarshaler) WriteJSON(w Writer) error {
	_, err := w.WriteString(`"Loreum"`)
	return err
}
func (*structPtrMarshaler) MarshalJSON() ([]byte, error) {
	return []byte(`"Loreum"`), nil
}

func TestMarshaler(t *testing.T) {
	// S = Non-pointer receiver of composite type.
	// I = Non-pointer receiver of primitive type.
	// F = Pointer receiver of composite kind.
	// P = Pointer receiver of primitive type.
	type x struct {
		S1 structMarshaler     `json:"s1"`
		S2 structMarshaler     `json:"s2,omitempty"`
		S3 *structMarshaler    `json:"s3"`
		S4 *structMarshaler    `json:"s4"`           // nil
		S5 *structMarshaler    `json:"s5,omitempty"` // nil
		I1 stringMarshaler     `json:"i1,omitempty"`
		I2 stringMarshaler     `json:"i2,omitempty"`
		I3 stringMarshaler     `json:"i3"`
		I4 *stringMarshaler    `json:"i4"`
		I5 *stringMarshaler    `json:"i5"`           // nil
		I6 *stringMarshaler    `json:"i6,omitempty"` // nil
		F1 structPtrMarshaler  `json:"f1"`
		F2 structPtrMarshaler  `json:"f2,omitempty"`
		F3 *structPtrMarshaler `json:"f3"`
		F4 *structPtrMarshaler `json:"f4"`           // nil
		F5 *structPtrMarshaler `json:"f5,omitempty"` // nil
		P1 stringPtrMarshaler  `json:"p1,omitempty"`
		P2 stringPtrMarshaler  `json:"p2,omitempty"`
		P3 stringPtrMarshaler  `json:"p3"`
		P4 *stringPtrMarshaler `json:"p4"`
		P5 *stringPtrMarshaler `json:"p5"`           // nil
		P6 *stringPtrMarshaler `json:"p6,omitempty"` // nil
	}
	enc, err := NewEncoder(reflect.TypeOf((*x)(nil)).Elem())
	if err != nil {
		t.Fatal(err)
	}
	var (
		swm  = stringMarshaler("Loreun")
		spwm = stringPtrMarshaler("Ipsum")
	)
	xx := x{
		S1: structMarshaler{},
		S3: &structMarshaler{},
		I1: "Loreun",
		I4: &swm,
		F1: structPtrMarshaler{},
		F3: &structPtrMarshaler{},
		P1: "Ipsum",
		P4: &spwm,
	}
	testdata := []struct {
		name string
		val  interface{}
	}{
		{"non-pointer", xx},
		{"pointer", &xx},
	}
	for _, tt := range testdata {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := enc.Encode(tt.val, &buf); err != nil {
				t.Error(err)
			}
			if !equalStdLib(t, tt.val, buf.Bytes()) {
				t.Error("expected outputs to be equal")
			}
		})
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
	enc, err := NewEncoder(timeType)
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
	if err := enc.Encode(&tm, &buf, UnixTimestamp()); err != nil {
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
	enc, err := NewEncoder(durationType)
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
	for _, tt := range testdata {
		var opts []Option
		if tt.Raw {
			opts = append(opts, ByteArrayAsString())
		}
		enc, err := NewEncoder(reflect.TypeOf(tt.Val))
		if err != nil {
			t.Error(err)
		}
		var buf bytes.Buffer
		if err := enc.Encode(tt.Val, &buf, opts...); err != nil {
			t.Error(err)
		}
		if s := buf.String(); s != tt.Str {
			t.Errorf("got %s, want %s", s, tt.Str)
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
			enc, err := NewEncoder(reflect.TypeOf([]byte{}))
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
		enc, err := NewEncoder(reflect.TypeOf(tt))
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

	enc, err := NewEncoder(reflect.TypeOf(b))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := enc.Encode(b, &buf, RawByteSlice()); err != nil {
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
	enc, err := NewEncoder(reflect.TypeOf(float64(0)))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []float64{
		math.NaN(),
		math.Inf(-1),
		math.Inf(1),
	} {
		v := v
		var buf bytes.Buffer
		err := enc.Encode(&v, &buf)
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
	b := []byte{'A', 1, 2, 3, '"', '\\', '/', 'B', 'C', '\b', '\f', '\n', '\r', '\t', 0xC7, 0xA3, 0xE2, 0x80, 0xA8, 0xE2, 0x80, 0xA9}
	testdata := []struct {
		Bts  []byte
		Want string
		NSE  bool // NoStringEscaping
	}{
		{b, `"A\u0001\u0002\u0003\"\\/BC\u0008\u000c\n\r\tǣ\u2028\u2029"`, false},
		{b, `"` + string(b) + `"`, true},
	}
	for _, tt := range testdata {
		s := string(tt.Bts)
		enc, err := NewEncoder(reflect.TypeOf(s))
		if err != nil {
			t.Fatal(err)
		}
		var opts []Option
		if tt.NSE {
			opts = append(opts, NoStringEscaping())
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

func TestStringHTMLEscaping(t *testing.T) {
	b := []byte{'<', '>', '&'}
	testdata := []struct {
		Bts  []byte
		Want string
		NSE  bool // NoStringEscaping
		NHE  bool // NoHTMLEscaping
	}{
		{b, `"\u003c\u003e\u0026"`, false, false},
		{b, `"<>&"`, false, true},

		// NoHTMLEscaping is ignored when NoStringEscaping
		// is set, because it's part of the escaping options.
		{b, `"<>&"`, true, false},
		{b, `"<>&"`, true, true},
	}
	for _, tt := range testdata {
		s := string(tt.Bts)
		enc, err := NewEncoder(reflect.TypeOf(s))
		if err != nil {
			t.Fatal(err)
		}
		var opts []Option
		if tt.NSE {
			opts = append(opts, NoStringEscaping())
		}
		if tt.NHE {
			opts = append(opts, NoHTMLEscaping())
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

// TestStringUTF8Coercion tests thats invalid bytes
// are replaced by the Unicode replacement rune when
// encoding a JSON string.
func TestStringUTF8Coercion(t *testing.T) {
	utf8Seq := string([]byte{'H', 'e', 'l', 'l', 'o', ',', ' ', 0xff, 0xfe, 0xff})
	testdata := []struct {
		Bts  string
		Want string
		NUC  bool // NoUTF8Coercion
	}{
		{utf8Seq, `"Hello, \ufffd\ufffd\ufffd"`, false},
		{utf8Seq, `"` + utf8Seq + `"`, true},
	}
	for _, tt := range testdata {
		enc, err := NewEncoder(reflect.TypeOf(tt.Bts))
		if err != nil {
			t.Fatal(err)
		}
		var opts []Option
		if tt.NUC {
			opts = append(opts, NoUTF8Coercion())
		}
		var buf bytes.Buffer
		if err := enc.Encode(tt.Bts, &buf, opts...); err != nil {
			t.Fatal(err)
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
		enc, err := NewEncoder(reflect.TypeOf(tt.in))
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

	enc, err := NewEncoder(reflect.TypeOf(Y{}))
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
	enc, err := NewEncoder(reflect.TypeOf(Z{}))
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
		enc, err := NewEncoder(numberType)
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

func TestInstrCache(t *testing.T) {
	type x struct {
		A string
	}
	i1, err := cachedTypeInstr(reflect.TypeOf(x{}))
	if err != nil {
		t.Fatal(err)
	}
	i2, err := cachedTypeInstr(reflect.TypeOf(x{}))
	if err != nil {
		t.Fatal(err)
	}
	p1 := reflect.ValueOf(i1).Pointer()
	p2 := reflect.ValueOf(i2).Pointer()
	if p1 != p2 {
		t.Errorf("expected instructions to be the same: %v != %v", p1, p2)
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
