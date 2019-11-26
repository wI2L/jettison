package jettison

import (
	"bytes"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"testing"
	"unsafe"
)

// TestSpecialTypeInstr tests that specialTypeInstr
// returns an instruction only for special types
// defined in isSpecialType.
func TestSpecialTypeInstr(t *testing.T) {
	for _, tt := range []struct {
		typ reflect.Type
		ack bool
	}{
		{timeTimeType, true},
		{timeDurationType, true},
		{jsonNumberType, true},
		{reflect.TypeOf(string("")), false},
		{reflect.TypeOf(struct{ A string }{}), false},
	} {
		instr := specialTypeInstr(tt.typ)
		if instr == nil && tt.ack {
			t.Errorf("specialTypeInstr(%s): expected non-nil instruction", tt.typ)
		}
	}
}

// TestBasicTypeInstr tests that the basicTypeInstr
// returns an instruction only for basic Go types
// defined in isBasicType.
func TestBasicTypeInstr(t *testing.T) {
	valid := []reflect.Kind{
		reflect.String,
		reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64,
	}
	for _, k := range kinds() {
		instr := basicTypeInstr(k)
		if instr == nil && kindIn(valid, k) {
			t.Errorf("basicTypeInstr(%s): expected non-nil instruction", k)
		}
	}
}

// TestIntegerInstr tests that integerInstr returns
// an error for non integer kinds.
func TestIntegerInstr(t *testing.T) {
	valid := []reflect.Kind{
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
	}
	for _, k := range kinds() {
		var i int64 = math.MaxInt8
		var buf bytes.Buffer
		want := strconv.Itoa(math.MaxInt8)

		err := integerInstr(unsafe.Pointer(&i), &buf, newState(), k)
		if err != nil {
			if kindIn(valid, k) {
				t.Errorf("integerInstr(%s): %s", k, err)
			}
		} else {
			if s := buf.String(); s != want {
				t.Errorf("got %s, want %s", s, want)
			}
		}
	}
}

// TestUnsignedIntegerInstr tests that unsignedIntegerInstr
// returns an error for non unsigned integer kinds.
func TestUnsignedIntegerInstr(t *testing.T) {
	valid := []reflect.Kind{
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
	}
	for _, k := range kinds() {
		var i uint64 = math.MaxUint8
		var buf bytes.Buffer
		want := strconv.Itoa(math.MaxUint8)

		err := unsignedIntegerInstr(unsafe.Pointer(&i), &buf, newState(), k)
		if err != nil {
			if kindIn(valid, k) {
				t.Errorf("unsignedIntegerInstr(%s): %s", k, err)
			}
		} else {
			if s := buf.String(); s != want {
				t.Errorf("got %s, want %s", s, want)
			}
		}
	}
}

// TestFloatInstr tests that floatInstr returns an
// error when an invalid bit size is used.
func TestFloatInstr(t *testing.T) {
	f := math.MaxFloat32
	for bs := 0; bs <= 256; bs++ {
		var buf bytes.Buffer
		err := floatInstr(unsafe.Pointer(&f), &buf, newState(), bs)
		if err != nil && (bs == 32 || bs == 64) {
			t.Errorf("floatInstr(%d): %s", bs, err)
		}
	}
}

func kinds() []reflect.Kind {
	var kinds []reflect.Kind
	for k := reflect.Invalid; k <= reflect.UnsafePointer; k++ {
		kinds = append(kinds, k)
	}
	return kinds
}

func kindIn(l []reflect.Kind, k reflect.Kind) bool {
	for _, e := range l {
		if e == k {
			return true
		}
	}
	return false
}

func TestIsValidNumber(t *testing.T) {
	// Taken from https://golang.org/src/encoding/json/number_test.go
	// Regexp from: https://stackoverflow.com/a/13340826
	var re = regexp.MustCompile(
		`^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?$`,
	)
	valid := []string{
		"0",
		"-0",
		"1",
		"-1",
		"0.1",
		"-0.1",
		"1234",
		"-1234",
		"12.34",
		"-12.34",
		"12E0",
		"12E1",
		"12e34",
		"12E-0",
		"12e+1",
		"12e-34",
		"-12E0",
		"-12E1",
		"-12e34",
		"-12E-0",
		"-12e+1",
		"-12e-34",
		"1.2E0",
		"1.2E1",
		"1.2e34",
		"1.2E-0",
		"1.2e+1",
		"1.2e-34",
		"-1.2E0",
		"-1.2E1",
		"-1.2e34",
		"-1.2E-0",
		"-1.2e+1",
		"-1.2e-34",
		"0E0",
		"0E1",
		"0e34",
		"0E-0",
		"0e+1",
		"0e-34",
		"-0E0",
		"-0E1",
		"-0e34",
		"-0E-0",
		"-0e+1",
		"-0e-34",
	}
	for _, tt := range valid {
		if !isValidNumber(tt) {
			t.Errorf("%s should be valid", tt)
		}
		if !re.MatchString(tt) {
			t.Errorf("%s should be valid but regexp does not match", tt)
		}
	}
	invalid := []string{
		"",
		"invalid",
		"1.0.1",
		"1..1",
		"-1-2",
		"012a42",
		"01.2",
		"012",
		"12E12.12",
		"1e2e3",
		"1e+-2",
		"1e--23",
		"1e",
		"e1",
		"1e+",
		"1ea",
		"1a",
		"1.a",
		"1.",
		"01",
		"1.e1",
	}
	for _, tt := range invalid {
		if isValidNumber(tt) {
			t.Errorf("%s should be invalid", tt)
		}
		if re.MatchString(tt) {
			t.Errorf("%s should be invalid but matches regexp", tt)
		}
	}
}
