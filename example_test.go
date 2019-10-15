package jettison_test

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"reflect"
	"strconv"

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
