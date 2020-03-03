// +build !go1.14

package jettison

import (
	"encoding"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"
)

type (
	bvtm int
	brtm int
	bvjm string
	brjm string
	cvtm struct{ L, R string }
	crtm struct{ L, R string }
)

func (m bvtm) MarshalText() ([]byte, error)  { return []byte(strconv.Itoa(int(m))), nil }
func (m *brtm) MarshalText() ([]byte, error) { return []byte(strconv.Itoa(int(*m))), nil }
func (m bvjm) MarshalJSON() ([]byte, error)  { return []byte(strconv.Quote(string(m))), nil }
func (m *brjm) MarshalJSON() ([]byte, error) { return []byte(strconv.Quote(string(*m))), nil }
func (m cvtm) MarshalText() ([]byte, error)  { return []byte(fmt.Sprintf("%s:%s", m.L, m.R)), nil }
func (m *crtm) MarshalText() ([]byte, error) { return []byte(fmt.Sprintf("%s:%s", m.L, m.R)), nil }

// TestTextMarshalerMapKey tests the marshaling
// of maps with key types that implements the
// encoding.TextMarshaler interface
func TestTextMarshalerMapKey(t *testing.T) {
	var (
		bval = bvtm(42)
		bref = brtm(84)
		cval = cvtm{L: "A", R: "B"}
		cref = crtm{L: "A", R: "B"}
		ip   = &net.IP{127, 0, 0, 1}
	)
	// The nil key cases, although supported by this library,
	// isn't tested with verions of Go prior to 1.14, because
	// the standard library panics on it, and thus, the results
	// cannot be compared.
	valid := []interface{}{
		map[time.Time]string{
			time.Now(): "now",
			{}:         "",
		},
		map[*net.IP]string{
			ip: "localhost",
		},
		map[cvtm]string{cval: "ab"},
		map[*cvtm]string{
			&cval: "ab",
		},
		map[*crtm]string{
			&cref: "ab",
		},
		map[bvtm]string{bval: "42"},
		map[*bvtm]string{
			&bval: "42",
		},
		map[brtm]string{bref: "42"},
		map[*brtm]string{
			&bref: "42",
		},
	}
	for _, v := range valid {
		marshalCompare(t, v, "valid")
	}
	invalid := []interface{}{
		// Non-pointer value of a pointer-receiver
		// type isn't a valid map key type.
		map[crtm]string{
			{L: "A", R: "B"}: "ab",
		},
	}
	for _, v := range invalid {
		marshalCompareError(t, v, "invalid")
	}
}

//nolint:godox
func TestNilMarshaler(t *testing.T) {
	testdata := []struct {
		v interface{}
	}{
		// json.Marshaler
		{struct{ M json.Marshaler }{M: nil}},
		{struct{ M json.Marshaler }{(*niljsonm)(nil)}},
		{struct{ M interface{} }{(*niljsonm)(nil)}},
		{struct{ M *niljsonm }{M: nil}},
		{json.Marshaler((*niljsonm)(nil))},
		{(*niljsonm)(nil)},

		// encoding.TextMarshaler
		{struct{ M encoding.TextMarshaler }{(*niltextm)(nil)}},
		{struct{ M interface{} }{(*niltextm)(nil)}},
		{struct{ M *niltextm }{M: nil}},
		{encoding.TextMarshaler((*niltextm)(nil))},
		{(*niltextm)(nil)},

		// The following case panics with versions of Go
		// prior to 1.14. See this issue for reference:
		// https://github.com/golang/go/issues/34235
		// {struct{ M encoding.TextMarshaler }{M: nil}},

		// jettison.Marshaler
		{struct{ M comboMarshaler }{M: nil}},
		{struct{ M comboMarshaler }{(*niljetim)(nil)}},
		{struct{ M interface{} }{(*niljetim)(nil)}},
		{struct{ M *niljetim }{M: nil}},
		{comboMarshaler((*niljetim)(nil))},
		{(*niljetim)(nil)},

		// jettison.MarshalerCtx
		{struct{ M comboMarshalerCtx }{M: nil}},
		{struct{ M comboMarshalerCtx }{(*nilmjctx)(nil)}},
		{struct{ M interface{} }{(*nilmjctx)(nil)}},
		{struct{ M *nilmjctx }{M: nil}},
		{comboMarshalerCtx((*nilmjctx)(nil))},
		{(*nilmjctx)(nil)},
	}
	for _, e := range testdata {
		marshalCompare(t, e.v, "nil-marshaler")
	}
}
