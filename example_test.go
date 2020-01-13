package jettison_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/wI2L/jettison"
)

func ExampleMarshal() {
	type X struct {
		A string   `json:"a"`
		B int64    `json:"b"`
		C []string `json:"colors"`
	}
	x := X{
		A: "Loreum",
		B: -42,
		C: []string{"blue", "white", "red"},
	}
	b, err := jettison.Marshal(x)
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(b)
	// Output:
	// {"a":"Loreum","b":-42,"colors":["blue","white","red"]}
}

func ExampleAppend() {
	type X struct {
		A bool              `json:"a"`
		B uint32            `json:"b"`
		C map[string]string `json:"users"`
	}
	x := X{
		A: true,
		B: 42,
		C: map[string]string{
			"bob":   "admin",
			"jerry": "user",
		},
	}
	buf, err := jettison.Append([]byte(nil), x)
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(buf)
	// Output:
	// {"a":true,"b":42,"users":{"bob":"admin","jerry":"user"}}
}

func ExampleAppendOpts() {
	for _, v := range []interface{}{
		nil, 2 * time.Second,
	} {
		buf, err := jettison.AppendOpts([]byte(nil), v,
			jettison.DurationFormat(jettison.DurationString),
		)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\n", string(buf))
	}
	// Output:
	// null
	// "2s"
}

type Animal int

const (
	Unknown Animal = iota
	Gopher
	Zebra
)

// AppendJSON implements the jettison.AppendMarshaler interface.
func (a Animal) AppendJSON(dst []byte) ([]byte, error) {
	var s string
	switch a {
	default:
		s = "unknown"
	case Gopher:
		s = "gopher"
	case Zebra:
		s = "zebra"
	}
	dst = append(dst, strconv.Quote(s)...)
	return dst, nil
}

func Example_customMarshaler() {
	zoo := []Animal{
		Unknown,
		Zebra,
		Gopher,
	}
	b, err := jettison.Marshal(zoo)
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(b)
	// Output:
	// ["unknown","zebra","gopher"]
}

func ExampleRawByteSlice() {
	bs := []byte("Loreum Ipsum")

	for _, opt := range []jettison.Option{
		nil, jettison.RawByteSlice(),
	} {
		b, err := jettison.MarshalOpts(bs, opt)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\n", string(b))
	}
	// Output:
	// "TG9yZXVtIElwc3Vt"
	// "Loreum Ipsum"
}

func ExampleByteArrayAsString() {
	b1 := [6]byte{'L', 'o', 'r', 'e', 'u', 'm'}
	b2 := [6]*byte{&b1[0], &b1[1], &b1[2], &b1[3], &b1[4], &b1[5]}

	for _, opt := range []jettison.Option{
		nil, jettison.ByteArrayAsString(),
	} {
		for _, v := range []interface{}{b1, b2} {
			b, err := jettison.MarshalOpts(v, opt)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("%s\n", string(b))
		}
	}
	// Output:
	// [76,111,114,101,117,109]
	// [76,111,114,101,117,109]
	// "Loreum"
	// [76,111,114,101,117,109]
}

func ExampleNilMapEmpty() {
	type X struct {
		M1 map[string]int
		M2 map[int]string
	}
	x := X{
		M1: map[string]int{},
		M2: nil,
	}
	for _, opt := range []jettison.Option{
		nil, jettison.NilMapEmpty(),
	} {
		b, err := jettison.MarshalOpts(x, opt)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\n", string(b))
	}
	// Output:
	// {"M1":{},"M2":null}
	// {"M1":{},"M2":{}}
}

func ExampleNilSliceEmpty() {
	type X struct {
		S1 []int
		S2 []string
	}
	x := X{
		S1: []int{},
		S2: nil,
	}
	for _, opt := range []jettison.Option{
		nil, jettison.NilSliceEmpty(),
	} {
		b, err := jettison.MarshalOpts(x, opt)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\n", string(b))
	}
	// Output:
	// {"S1":[],"S2":null}
	// {"S1":[],"S2":[]}
}

func ExampleUnixTime() {
	t := time.Date(2024, time.December, 24, 12, 24, 42, 0, time.UTC)

	b, err := jettison.MarshalOpts(t, jettison.UnixTime())
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(b)
	// Output:
	// 1735043082
}

func ExampleTimeLayout() {
	t := time.Date(2042, time.July, 25, 16, 42, 24, 67850, time.UTC)

	locs := []*time.Location{
		time.UTC, time.FixedZone("WTF", 666), time.FixedZone("LOL", -4242),
	}
	for _, layout := range []string{
		time.RFC3339,
		time.RFC822,
		time.RFC1123Z,
		time.RFC3339Nano, // default
	} {
		for _, loc := range locs {
			b, err := jettison.MarshalOpts(t.In(loc), jettison.TimeLayout(layout))
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("%s\n", string(b))
		}
	}
	// Output:
	// "2042-07-25T16:42:24Z"
	// "2042-07-25T16:53:30+00:11"
	// "2042-07-25T15:31:42-01:10"
	// "25 Jul 42 16:42 UTC"
	// "25 Jul 42 16:53 WTF"
	// "25 Jul 42 15:31 LOL"
	// "Fri, 25 Jul 2042 16:42:24 +0000"
	// "Fri, 25 Jul 2042 16:53:30 +0011"
	// "Fri, 25 Jul 2042 15:31:42 -0110"
	// "2042-07-25T16:42:24.00006785Z"
	// "2042-07-25T16:53:30.00006785+00:11"
	// "2042-07-25T15:31:42.00006785-01:10"
}

func ExampleDurationFormat() {
	d := 1*time.Hour + 3*time.Minute + 2*time.Second + 66*time.Millisecond

	for _, format := range []jettison.DurationFmt{
		jettison.DurationString,
		jettison.DurationMinutes,
		jettison.DurationSeconds,
		jettison.DurationMilliseconds,
		jettison.DurationMicroseconds,
		jettison.DurationNanoseconds,
	} {
		b, err := jettison.MarshalOpts(d, jettison.DurationFormat(format))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\n", string(b))
	}
	// Output:
	// "1h3m2.066s"
	// 63.03443333333333
	// 3782.066
	// 3782066
	// 3782066000
	// 3782066000000
}

func ExampleUnsortedMap() {
	m := map[int]string{
		3: "three",
		1: "one",
		2: "two",
	}
	b, err := jettison.MarshalOpts(m, jettison.UnsortedMap())
	if err != nil {
		log.Fatal(err)
	}
	var sorted map[int]string
	if err := json.Unmarshal(b, &sorted); err != nil {
		log.Fatal(err)
	}
	b, err = jettison.Marshal(sorted)
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(b)
	// Output:
	// {"1":"one","2":"two","3":"three"}
}

func ExampleNoCompact() {
	rm := json.RawMessage(`{ "a":"b" }`)
	for _, opt := range []jettison.Option{
		nil, jettison.NoCompact(),
	} {
		b, err := jettison.MarshalOpts(rm, opt)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\n", string(b))
	}
	// Output:
	// {"a":"b"}
	// { "a":"b" }
}

func ExampleAllowList() {
	type Z struct {
		Omega int `json:"ω"`
	}
	type Y struct {
		Pi string `json:"π"`
	}
	type X struct {
		Z     Z      `json:"Z"`
		Alpha string `json:"α"`
		Beta  string `json:"β"`
		Gamma string
		Y
	}
	x := X{
		Z:     Z{Omega: 42},
		Alpha: "1",
		Beta:  "2",
		Gamma: "3",
		Y:     Y{Pi: "4"},
	}
	for _, opt := range []jettison.Option{
		nil, jettison.AllowList([]string{"Z", "β", "Gamma", "π"}),
	} {
		b, err := jettison.MarshalOpts(x, opt)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\n", string(b))
	}
	// Output:
	// {"Z":{"ω":42},"α":"1","β":"2","Gamma":"3","π":"4"}
	// {"Z":{},"β":"2","Gamma":"3","π":"4"}
}

func ExampleDenyList() {
	type X struct {
		A int  `json:"aaAh"`
		B bool `json:"buzz"`
		C string
		D uint
	}
	x := X{
		A: -42,
		B: true,
		C: "Loreum",
		D: 42,
	}
	for _, opt := range []jettison.Option{
		nil, jettison.DenyList([]string{"buzz", "D"}),
	} {
		b, err := jettison.MarshalOpts(x, opt)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\n", string(b))
	}
	// Output:
	// {"aaAh":-42,"buzz":true,"C":"Loreum","D":42}
	// {"aaAh":-42,"C":"Loreum"}
}

type (
	secret string
	ctxKey string
)

const obfuscateKey = ctxKey("_obfuscate_")

// AppendJSONContext implements the jettison.AppendMarshalerCtx interface.
func (s secret) AppendJSONContext(ctx context.Context, dst []byte) ([]byte, error) {
	out := string(s)
	if v := ctx.Value(obfuscateKey); v != nil {
		if hide, ok := v.(bool); ok && hide {
			out = "**__SECRET__**"
		}
	}
	dst = append(dst, strconv.Quote(out)...)
	return dst, nil
}

func ExampleWithContext() {
	sec := secret("v3ryS3nSitiv3P4ssWord")

	b, err := jettison.Marshal(sec)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", string(b))

	ctx := context.WithValue(context.Background(),
		obfuscateKey, true,
	)
	b, err = jettison.MarshalOpts(sec, jettison.WithContext(ctx))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", string(b))
	// Output:
	// "v3ryS3nSitiv3P4ssWord"
	// "**__SECRET__**"
}
