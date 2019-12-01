package jettison_test

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"reflect"
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
		C: []string{"blue", "green", "purple"},
	}
	b, err := jettison.Marshal(&x)
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(b)
	// Output:
	// {"a":"Loreum","b":-42,"colors":["blue","green","purple"]}
}

func ExampleMarshalTo() {
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
	var buf bytes.Buffer
	err := jettison.MarshalTo(&x, &buf)
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(buf.Bytes())
	// Output:
	// {"a":true,"b":42,"users":{"bob":"admin","jerry":"user"}}
}

func ExampleEncoder_Encode() {
	type X struct {
		M map[string]int
		S []int
	}
	x := X{}
	enc, err := jettison.NewEncoder(reflect.TypeOf(x))
	if err != nil {
		log.Fatal(err)
	}
	var buf bytes.Buffer
	if err := enc.Encode(&x, &buf); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", buf.String())

	buf.Reset()
	if err := enc.Encode(&x, &buf,
		jettison.NilMapEmpty(),
		jettison.NilSliceEmpty(),
	); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", buf.String())
	// Output:
	// {"M":null,"S":null}
	// {"M":{},"S":[]}
}

type Animal int

const (
	Unknown Animal = iota
	Gopher
	Zebra
)

func (a Animal) WriteJSON(w jettison.Writer) error {
	var s string
	switch a {
	default:
		s = "unknown"
	case Gopher:
		s = "gopher"
	case Zebra:
		s = "zebra"
	}
	_, err := w.WriteString(strconv.Quote(s))
	return err
}

func Example_customMarshaler() {
	zoo := []Animal{
		Unknown,
		Zebra,
		Gopher,
		Zebra,
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
	// ["unknown","zebra","gopher","zebra","unknown","zebra","gopher"]
}

func ExampleTimeLayout() {
	t := time.Date(2019, time.December, 25, 16, 44, 28, 0, time.UTC)

	enc, err := jettison.NewEncoder(reflect.TypeOf(t))
	if err != nil {
		log.Fatal(err)
	}
	for _, layout := range []string{
		time.RFC3339,
		time.RFC822,
		time.RFC1123Z,
		time.Kitchen,
	} {
		var buf bytes.Buffer
		if err := enc.Encode(t, &buf, jettison.TimeLayout(layout)); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\n", buf.String())
	}
	// Output:
	// "2019-12-25T16:44:28Z"
	// "25 Dec 19 16:44 UTC"
	// "Wed, 25 Dec 2019 16:44:28 +0000"
	// "4:44PM"
}

func ExampleWithFields() {
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
	enc, err := jettison.NewEncoder(reflect.TypeOf(x))
	if err != nil {
		log.Fatal(err)
	}
	var buf bytes.Buffer
	if err := enc.Encode(&x, &buf); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", buf.String())

	buf.Reset()
	if err := enc.Encode(&x, &buf,
		jettison.WithFields([]string{"Z", "β", "Gamma", "π"}),
	); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", buf.String())
	// Output:
	// {"Z":{"ω":42},"α":"1","β":"2","Gamma":"3","π":"4"}
	// {"Z":{"ω":42},"β":"2","Gamma":"3","π":"4"}
}

func ExampleIntegerBase() {
	type X struct {
		A int32
		B int64
		C uint8
		D uint16
		E uintptr
	}
	x := X{
		A: -4242,
		B: -424242,
		C: 42,
		D: 4242,
		E: 0x42,
	}
	enc, err := jettison.NewEncoder(reflect.TypeOf(x))
	if err != nil {
		log.Fatal(err)
	}
	for _, base := range []int{2, 8, 10, 16, 36} {
		var buf bytes.Buffer
		if err := enc.Encode(&x, &buf, jettison.IntegerBase(base)); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\n", buf.String())
	}
	// Output:
	// {"A":-1000010010010,"B":-1100111100100110010,"C":101010,"D":1000010010010,"E":1000010}
	// {"A":-10222,"B":-1474462,"C":52,"D":10222,"E":102}
	// {"A":-4242,"B":-424242,"C":42,"D":4242,"E":66}
	// {"A":"-1092","B":"-67932","C":"2a","D":"1092","E":"42"}
	// {"A":"-39u","B":"-93ci","C":"16","D":"39u","E":"1u"}
}
