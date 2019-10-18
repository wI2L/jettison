package jettison

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/francoispqt/gojay"
	jsoniter "github.com/json-iterator/go"
)

var jsoniterStd = jsoniter.ConfigCompatibleWithStandardLibrary

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
	enc, err := NewEncoder(reflect.TypeOf(simplePayload{}))
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
			// NoUTF8Coercion and NoHTMLEscaping are used to
			// have a fair comparison with Gojay, which does
			// not coerce strings to valid UTF-8 and doesn't
			// escape HTML characters either.
			// None of the string fields of the SimplePayload
			// type contains HTML characters nor contains invalid
			// UTF-8 byte sequences, so this is fine.
			if err := enc.Encode(sp, &buf, NoUTF8Coercion(), NoHTMLEscaping()); err != nil {
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
	enc, err := NewEncoder(reflect.TypeOf(x{}))
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
	var iface interface{} = s
	enc, err := NewEncoder(reflect.TypeOf(iface))
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
		enc, err := NewEncoder(reflect.TypeOf(m))
		if err != nil {
			b.Fatal(err)
		}
		b.Run("sort", benchMap(enc, m))
		b.Run("nosort", benchMap(enc, m, UnsortedMap()))
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

func BenchmarkStringEscaping(b *testing.B) {
	s := "<ŁØŘ€M ƗƤŞỮM ĐØŁØŘ ŞƗŦ ΔM€Ŧ>"

	enc, err := NewEncoder(reflect.TypeOf(s))
	if err != nil {
		b.Fatal(err)
	}
	b.Run("Full",
		benchEscaping(enc, s))
	b.Run("NoUTF8Coercion",
		benchEscaping(enc, s, NoUTF8Coercion()))
	b.Run("NoHTMLEscaping",
		benchEscaping(enc, s, NoHTMLEscaping()))
	b.Run("NoUTF8Coercion/NoHTMLEscaping",
		benchEscaping(enc, s, NoUTF8Coercion(), NoHTMLEscaping()))
	b.Run("NoStringEscaping",
		benchEscaping(enc, s, NoStringEscaping()))
}

//nolint:unparam
func benchEscaping(enc *Encoder, s string, opts ...Option) func(b *testing.B) {
	var buf bytes.Buffer
	return func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if err := enc.Encode(s, &buf, opts...); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(buf.Len()))
			buf.Reset()
		}
	}
}

func BenchmarkMarshaler(b *testing.B) {
	for _, bb := range []struct {
		Name string
		Impl interface{}
	}{
		{"JSON", jsonBM{}},
		{"Text", textBM{}},
		{"Jettison", jetiBM{}},
	} {
		bb := bb
		b.Run(bb.Name, func(b *testing.B) {
			enc, err := NewEncoder(reflect.TypeOf(bb.Impl))
			if err != nil {
				b.Fatal(err)
			}
			var buf bytes.Buffer
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := enc.Encode(bb.Impl, &buf); err != nil {
					b.Fatal(err)
				}
				b.SetBytes(int64(buf.Len()))
				buf.Reset()
			}
		})
	}
}

type (
	jsonBM struct{}
	textBM struct{}
	jetiBM struct{}
)

func (jsonBM) MarshalJSON() ([]byte, error) {
	return []byte(`"Lorem ipsum dolor sit amet"`), nil
}
func (textBM) MarshalText() ([]byte, error) {
	return []byte("Lorem ipsum dolor sit amet"), nil
}
func (jetiBM) WriteJSON(w Writer) error {
	_, err := w.WriteString("Lorem ipsum dolor sit amet")
	return err
}
