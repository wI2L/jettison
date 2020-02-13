package jettison

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	jsoniter "github.com/json-iterator/go"
	segmentj "github.com/segmentio/encoding/json"
)

var jsoniterCfg = jsoniter.ConfigCompatibleWithStandardLibrary

type marshalFunc func(interface{}) ([]byte, error)

type codeResponse struct {
	Tree     *codeNode `json:"tree"`
	Username string    `json:"username"`
}

type codeNode struct {
	Name     string      `json:"name"`
	Kids     []*codeNode `json:"kids"`
	CLWeight float64     `json:"cl_weight"`
	Touches  int         `json:"touches"`
	MinT     int64       `json:"min_t"`
	MaxT     int64       `json:"max_t"`
	MeanT    int64       `json:"mean_t"`
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

func BenchmarkSimple(b *testing.B) {
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
	benchMarshal(b, sp)
}

func BenchmarkComplex(b *testing.B) {
	benchMarshal(b, xx)
}

func BenchmarkCodeMarshal(b *testing.B) {
	// Taken from the encoding/json package.
	x := codeInit(b)
	benchMarshal(b, x)
}

func BenchmarkMap(b *testing.B) {
	m := map[string]int{
		"Cassianus": 1,
		"Ludovicus": 42,
		"Flavius":   8990,
		"Baldwin":   345,
		"Agapios":   -43,
		"Liberia":   0,
	}
	benchMarshal(b, m)
	benchMarshalOpts(b, "jettison-nosort", m, UnsortedMap())
}

func BenchmarkSyncMap(b *testing.B) {
	if testing.Short() {
		b.SkipNow()
	}
	var sm sync.Map

	sm.Store("a", "foobar")
	sm.Store("b", 42)
	sm.Store("c", false)
	sm.Store("d", float64(3.14159))

	benchMarshalOpts(b, "sorted", m)
	benchMarshalOpts(b, "unsorted", m, UnsortedMap())
}

func BenchmarkDuration(b *testing.B) {
	if testing.Short() {
		b.SkipNow()
	}
	d := 1337 * time.Second
	benchMarshal(b, d)
}

func BenchmarkDurationFormat(b *testing.B) {
	if testing.Short() {
		b.SkipNow()
	}
	d := 32*time.Hour + 56*time.Minute + 25*time.Second
	for _, f := range []DurationFmt{
		DurationString,
		DurationMinutes,
		DurationSeconds,
		DurationMicroseconds,
		DurationMilliseconds,
		DurationNanoseconds,
	} {
		benchMarshalOpts(b, f.String(), d, DurationFormat(f))
	}
}

func BenchmarkTime(b *testing.B) {
	if testing.Short() {
		b.SkipNow()
	}
	t := time.Now()
	benchMarshal(b, t)
}

func BenchmarkStringEscaping(b *testing.B) {
	if testing.Short() {
		b.SkipNow()
	}
	s := "<ŁØŘ€M ƗƤŞỮM ĐØŁØŘ ŞƗŦ ΔM€Ŧ>"

	benchMarshalOpts(b, "Full", s)
	benchMarshalOpts(b, "NoUTF8Coercion", s, NoUTF8Coercion())
	benchMarshalOpts(b, "NoHTMLEscaping", s, NoHTMLEscaping())
	benchMarshalOpts(b, "NoUTF8Coercion/NoHTMLEscaping", s, NoUTF8Coercion(), NoHTMLEscaping())
	benchMarshalOpts(b, "NoStringEscaping", s, NoStringEscaping())
}

type (
	jsonbm    struct{}
	textbm    struct{}
	jetibm    struct{}
	jetictxbm struct{}
)

var (
	loreumipsum  = "Lorem ipsum dolor sit amet"
	loreumipsumQ = strconv.Quote(loreumipsum)
)

func (jsonbm) MarshalJSON() ([]byte, error)          { return []byte(loreumipsumQ), nil }
func (textbm) MarshalText() ([]byte, error)          { return []byte(loreumipsum), nil }
func (jetibm) AppendJSON(dst []byte) ([]byte, error) { return append(dst, loreumipsum...), nil }

func (jetictxbm) AppendJSONContext(_ context.Context, dst []byte) ([]byte, error) {
	return append(dst, loreumipsum...), nil
}

func BenchmarkMarshaler(b *testing.B) {
	if testing.Short() {
		b.SkipNow()
	}
	for _, bb := range []struct {
		name string
		impl interface{}
		opts []Option
	}{
		{"json", jsonbm{}, nil},
		{"text", textbm{}, nil},
		{"append", jetibm{}, nil},
		{"appendctx", jetictxbm{}, []Option{WithContext(context.Background())}},
	} {
		benchMarshalOpts(b, bb.name, bb.impl, bb.opts...)
	}
}

func codeInit(b *testing.B) *codeResponse {
	f, err := os.Open("testdata/code.json.gz")
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		b.Fatal(err)
	}
	data, err := ioutil.ReadAll(gz)
	if err != nil {
		b.Fatal(err)
	}
	codeJSON := data

	var resp codeResponse
	if err := json.Unmarshal(codeJSON, &resp); err != nil {
		b.Fatalf("unmarshal code.json: %s", err)
	}
	if data, err = Marshal(&resp); err != nil {
		b.Fatalf("marshal code.json: %s", err)
	}
	if !bytes.Equal(data, codeJSON) {
		b.Logf("different lengths: %d - %d", len(data), len(codeJSON))

		for i := 0; i < len(data) && i < len(codeJSON); i++ {
			if data[i] != codeJSON[i] {
				b.Logf("re-marshal: changed at byte %d", i)
				b.Logf("old: %s", string(codeJSON[i-10:i+10]))
				b.Logf("new: %s", string(data[i-10:i+10]))
				break
			}
		}
		b.Fatal("re-marshal code.json: different result")
	}
	return &resp
}

func benchMarshalOpts(b *testing.B, name string, x interface{}, opts ...Option) {
	b.Run(name, func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			bts, err := MarshalOpts(x, opts...)
			if err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(bts)))
		}
	})
}

func benchMarshal(b *testing.B, x interface{}) {
	for _, bb := range []struct {
		name string
		fn   marshalFunc
	}{
		{"standard", json.Marshal},
		{"jsoniter", jsoniterCfg.Marshal},
		{"segmentj", segmentj.Marshal},
		{"jettison", Marshal},
	} {
		bb := bb
		b.Run(bb.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				bts, err := bb.fn(x)
				if err != nil {
					b.Error(err)
				}
				b.SetBytes(int64(len(bts)))
			}
		})
	}
}
