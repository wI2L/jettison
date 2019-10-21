package jettison

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unsafe"
)

const validFieldNameChars = "!#$%&()*+-./:<=>?@[]^_{|}~ "

type field struct {
	name       string
	keyNonEsc  []byte
	keyEscHTML []byte
	sf         reflect.StructField
	typ        reflect.Type
	index      []int
	tag        bool
	quoted     bool
	omitEmpty  bool

	// offsetSeq is the sequence of offsets
	// between top-level pointer to field.
	offsetSeq []uintptr

	// indirSeq is the sequence of pointer
	// indirections to follow to reach the
	// field through one or more anonymous
	// parent fields.
	indirSeq []bool

	// countSeq represents the number of
	// fields in each parent struct.
	countSeq []int

	// pfc represents the number of fields
	// of the field's parent struct.
	pfc int
}

// byIndex sorts a list of fields by index sequence.
type byIndex []field

func (x byIndex) Len() int      { return len(x) }
func (x byIndex) Swap(i, j int) { x[i], x[j] = x[j], x[i] }

func (x byIndex) Less(i, j int) bool {
	for k, xik := range x[i].index {
		if k >= len(x[j].index) {
			return false
		}
		if xik != x[j].index[k] {
			return xik < x[j].index[k]
		}
	}
	return len(x[i].index) < len(x[j].index)
}

// newStructInstr returns a new instruction to encode a
// Go struct. It returns an error if the given type is
// unexpected, or if one of the struct fields has an
// unsupported type.
func newStructInstr(t reflect.Type) (Instruction, error) {
	fields := structFields(t)
	instrs := make([]Instruction, 0, len(fields))

	// Iterate over the list of fields scanned
	// and add an instruction to encode each.
	for _, f := range fields {
		instr, err := newStructFieldInstr(f)
		if err != nil {
			return nil, &UnsupportedTypeError{f.typ,
				fmt.Sprintf("field %s of struct %s", f.name, t),
			}
		}
		instrs = append(instrs, instr)
	}
	return func(p unsafe.Pointer, w Writer, es *encodeState) error {
		if err := w.WriteByte('{'); err != nil {
			return err
		}
		// Reinitialize the current first-field mark
		// to encode the object in the next depth-level.
		ff := es.firstField
		es.firstField = false
		for i := 0; i < len(instrs); i++ {
			if err := instrs[i](p, w, es); err != nil {
				return err
			}
		}
		es.firstField = ff
		return w.WriteByte('}')
	}, nil
}

// newStructFieldInstr returns a new instruction
// to encode the field of a struct.
func newStructFieldInstr(f field) (Instruction, error) {
	ft := f.sf.Type
	isPtr := ft.Kind() == reflect.Ptr
	if isPtr {
		ft = ft.Elem()
	}
	// Find the adequate instruction to encode the
	// struct field according to its type. If the
	// field is a pointer, the instruction is then
	// wrapped with another one that writes null
	// if the pointer is nil.
	instr, err := cachedTypeInstr(ft)
	if err != nil {
		return nil, err
	}
	if f.quoted {
		if f.typ.Kind() == reflect.String {
			instr = wrapQuoteInstr(quotedStringInstr)
		} else {
			instr = wrapQuoteInstr(instr)
		}
	}
	// The length of sequences must be equal.
	if len(f.indirSeq) != len(f.offsetSeq) {
		return nil, fmt.Errorf("inconsistent indirection and offset sequences length: %d and %d",
			len(f.indirSeq), len(f.offsetSeq),
		)
	}
	// If f is an embedded pointer field and there
	// is no other fields in the parent, the received
	// pointer points to the field itself.
	if f.sf.Anonymous && f.pfc == 1 && isPtr {
		return wrapAnonymousFieldInstr(instr, f), nil
	}
	// Wrap resolved type instruction to handle the
	// omitempty option and writing the field's name.
	// The last offset of the sequence is used, which
	// correspond to that of the field.
	instr = wrapSetAddressable(wrapStructFieldInstr(instr, f, isPtr, ft), isPtr)

	if len(f.indirSeq) > 0 {
		return indirInstr(instr, f), nil
	}
	// Nothing to follow.
	return instr, nil
}

// wrapSetAddressable returns a wrapped instruction
// of instr that set whether the field is addressable.
func wrapSetAddressable(instr Instruction, canAddr bool) Instruction {
	if canAddr {
		return func(p unsafe.Pointer, w Writer, es *encodeState) error {
			es.addressable = true
			err := instr(p, w, es)
			es.addressable = false
			return err
		}
	}
	return instr
}

// wrapAnonymousFieldInstr returns a wrapped instruction
// of instr that encode a solitary anonymous field.
func wrapAnonymousFieldInstr(instr Instruction, f field) Instruction {
	var (
		key    = f.keyNonEsc
		keyEsc = f.keyEscHTML
		omit   = f.omitEmpty
	)
	return func(p unsafe.Pointer, w Writer, es *encodeState) error {
		// Input value may be a typed nil.
		if omit && p == nil {
			return nil
		}
		// Dereference if the input eface given to
		// Encode holds a pointer.
		if p != nil && es.inputPtr {
			p = *(*unsafe.Pointer)(p)
			if omit && p == nil {
				return nil
			}
		}
		k := keyEsc
		if es.opts.noHTMLEscape {
			k = key
		}
		if err := writeFieldKey(k, w, es); err != nil {
			return err
		}
		if p == nil {
			_, err := w.WriteString("null")
			return err
		}
		return instr(p, w, es)
	}
}

func wrapStructFieldInstr(instr Instruction, f field, isPtr bool, ft reflect.Type) Instruction {
	// Create a copy of the variables needed
	// in the instruction to avoid keeping a
	// reference to f.
	var (
		key    = f.keyNonEsc
		keyEsc = f.keyEscHTML
		omit   = f.omitEmpty
		offset = f.sf.Offset
	)
	if isPtr {
		return func(p unsafe.Pointer, w Writer, es *encodeState) error {
			if p != nil {
				p = unsafe.Pointer(uintptr(p) + offset)
				p = *(*unsafe.Pointer)(p)
			}
			if omit && p == nil {
				return nil
			}
			k := keyEsc
			if es.opts.noHTMLEscape {
				k = key
			}
			if err := writeFieldKey(k, w, es); err != nil {
				return err
			}
			if p == nil {
				_, err := w.WriteString("null")
				return err
			}
			return instr(p, w, es)
		}
	}
	return func(v unsafe.Pointer, w Writer, es *encodeState) error {
		p := unsafe.Pointer(uintptr(v) + offset)
		if omit && isEmpty(p, ft) {
			return nil
		}
		k := keyEsc
		if es.opts.noHTMLEscape {
			k = key
		}
		if err := writeFieldKey(k, w, es); err != nil {
			return err
		}
		return instr(p, w, es)
	}
}

func indirInstr(instr Instruction, f field) Instruction {
	var (
		indirSeq  = f.indirSeq
		offsetSeq = f.offsetSeq
		countSeq  = f.countSeq
	)
	return func(p unsafe.Pointer, w Writer, es *encodeState) error {
		if p == nil {
			return nil
		}
		for i, indir := range indirSeq {
			p = unsafe.Pointer(uintptr(p) + offsetSeq[i])
			if indir {
				if i == len(indirSeq)-1 && countSeq[0] == 1 && !es.inputPtr {
					break
				}
				if p != nil {
					p = *(*unsafe.Pointer)(p)
				}
				if p == nil {
					return nil
				}
			}
		}
		return instr(p, w, es)
	}
}

type typeCount map[reflect.Type]int

// structFields returns a list of fields that should be
// encoded for the given type. The algorithm is breadth-first
// search over the set of structs to include, the top struct
// and then any reachable anonymous structs.
func structFields(t reflect.Type) []field {
	var (
		fields []field
		curr   = []field{}
		next   = []field{{typ: t}}
		ccnt   typeCount
		ncnt   = make(typeCount)
		visit  = make(map[reflect.Type]bool)
	)
	for len(next) > 0 {
		curr, next = next, curr[:0]
		ccnt, ncnt = ncnt, make(map[reflect.Type]int)

		for _, f := range curr {
			if visit[f.typ] {
				continue
			}
			visit[f.typ] = true
			// Scan the type for fields to encoded.
			fields, next = scanFields(f, fields, next, ccnt, ncnt)
		}
	}
	// Sort fields by name, breaking ties with depth,
	// then whether the field name come from the JSON
	// tag, and finally with the index sequence.
	sortFields(fields)

	fields = filterByVisibility(fields)

	// Sort fields by their index sequence.
	sort.Sort(byIndex(fields))

	return fields
}

// filterByVisibility deletes all fields that are hidden
// by the Go rules for embedded fields, except that fields
// with JSON tags are promoted. The fields are sorted in
// primary order of name, secondary order of field index
// length.
func filterByVisibility(fields []field) []field {
	ret := fields[:0]

	for adv, i := 0, 0; i < len(fields); i += adv {
		// One iteration per name.
		// Find the sequence of fields with the name
		// of this first field.
		fi := fields[i]
		for adv = 1; i+adv < len(fields); adv++ {
			fj := fields[i+adv]
			if fj.name != fi.name {
				break
			}
		}
		if adv == 1 {
			// Only one field with this name.
			ret = append(ret, fi)
			continue
		}
		// More than one field with the same name are
		// present, delete hidden fields by choosing
		// the one dominant field that survives.
		if dominant, ok := dominantField(fields[i : i+adv]); ok {
			ret = append(ret, dominant)
		}
	}
	return ret
}

// dominantField looks through the fields, all of which
// are known to have the same name, to find the single
// field that dominates the others using Go's embedding
// rules, modified by the presence of JSON tags. If there
// are multiple top-level fields, it returns false. This
// condition is an error in Go, and we skip all the fields.
func dominantField(fields []field) (field, bool) {
	if len(fields) > 1 &&
		len(fields[0].index) == len(fields[1].index) &&
		fields[0].tag == fields[1].tag {
		return field{}, false
	}
	return fields[0], true
}

func scanFields(f field, fields, next []field, cnt, ncnt typeCount) ([]field, []field) {
	var escBuf bytes.Buffer

	for i := 0; i < f.typ.NumField(); i++ {
		sf := f.typ.Field(i)

		if !shouldEncodeField(sf) {
			continue
		}
		tag := sf.Tag.Get("json")
		if tag == "-" {
			continue
		}
		// Parse name and options from the content of the
		// json tag.
		name, opts := parseTag(tag)
		if !isValidFieldName(name) {
			name = ""
		}
		index := make([]int, len(f.index)+1)
		copy(index, f.index)
		index[len(f.index)] = i

		ft := sf.Type
		isPtr := ft.Kind() == reflect.Ptr
		if ft.Name() == "" && isPtr {
			ft = ft.Elem()
		}
		// If the field is a named embedded struct or a
		// simple field, record it and its index sequence.
		if name != "" || !sf.Anonymous || ft.Kind() != reflect.Struct {
			tagged := name != ""
			// If a name is not present in the tag,
			// use the struct field's name instead.
			if name == "" {
				name = sf.Name
			}
			// Build HTML escaped field key.
			escBuf.Reset()
			_, _ = escBuf.WriteString(`,"`)
			json.HTMLEscape(&escBuf, []byte(name))
			_, _ = escBuf.WriteString(`":`)

			fields = append(fields, field{
				typ:        ft,
				sf:         sf,
				name:       name,
				tag:        tagged,
				index:      index,
				omitEmpty:  opts.Contains("omitempty"),
				quoted:     opts.Contains("string") && isPrimitiveType(ft),
				keyNonEsc:  []byte(`,"` + name + `":`),
				keyEscHTML: append([]byte{}, escBuf.Bytes()...), // copy
				offsetSeq:  f.offsetSeq,
				indirSeq:   f.indirSeq,
				countSeq:   f.countSeq,
				pfc:        f.typ.NumField(),
			})
			if cnt[f.typ] > 1 {
				// If there were multiple instances, add a
				// second, so that the annihilation code will
				// see a duplicate. It only cares about the
				// distinction between 1 or 2, so don't bother
				// generating any more copies.
				fields = append(fields, fields[len(fields)-1])
			}
			continue
		}
		// Record unnamed embedded struct
		// to be scanned in the next round.
		ncnt[ft]++
		if ncnt[ft] == 1 {
			next = append(next, field{
				typ:       ft,
				name:      ft.Name(),
				index:     index,
				offsetSeq: append(f.offsetSeq, sf.Offset),
				indirSeq:  append(f.indirSeq, isPtr),
				countSeq:  append(f.countSeq, f.typ.NumField()),
			})
		}
	}
	return fields, next
}

func sortFields(fields []field) {
	sort.Slice(fields, func(i int, j int) bool {
		x := fields

		if x[i].name != x[j].name {
			return x[i].name < x[j].name
		}
		if len(x[i].index) != len(x[j].index) {
			return len(x[i].index) < len(x[j].index)
		}
		if x[i].tag != x[j].tag {
			return x[i].tag
		}
		return byIndex(x).Less(i, j)
	})
}

//nolint:interfacer
// Keep this function costless so that
// it can be inlined by the compiler.
func writeFieldKey(key []byte, w Writer, es *encodeState) error {
	if !es.firstField {
		// Omit leading comma for first field.
		// Key cannot be nil or empty, because
		// the scanFields methods always quote
		// the name and prepend a comma.
		key, es.firstField = key[1:], true
	}
	_, err := w.Write(key)
	return err
}

// wrapQuoteInstr wraps the given instruction and
// writes a JSON quote before and after its execution.
func wrapQuoteInstr(instr Instruction) Instruction {
	return func(p unsafe.Pointer, w Writer, es *encodeState) error {
		if err := w.WriteByte('"'); err != nil {
			return err
		}
		if err := instr(p, w, es); err != nil {
			return err
		}
		return w.WriteByte('"')
	}
}

// shouldEncodeField returns whether a struct
// field should be encoded.
func shouldEncodeField(sf reflect.StructField) bool {
	isUnexported := sf.PkgPath != ""
	if sf.Anonymous {
		t := sf.Type
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		// Ignore embedded fields of unexported non-struct
		// types, but in the contrary, don't ignore embedded
		// fields of unexported struct types since they may
		// have exported fields.
		if isUnexported && t.Kind() != reflect.Struct {
			return false
		}
	} else if isUnexported {
		// Ignore unexported non-embedded fields.
		return false
	}
	return true
}

// isValidFieldName returns whether s is a valid
// name and can be used as a JSON key to encode
// a struct field.
func isValidFieldName(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		switch {
		case strings.ContainsRune(validFieldNameChars, c):
			// Backslash and quote chars are reserved, but
			// otherwise any punctuation chars are allowed
			// in a tag name.
		case !unicode.IsLetter(c) && !unicode.IsDigit(c):
			return false
		}
	}
	return true
}

// isEmpty returns whether the value pointed by
// p is the zero-value of the given type.
func isEmpty(p unsafe.Pointer, t reflect.Type) bool {
	switch t.Kind() {
	case reflect.String:
		return (*reflect.StringHeader)(p).Len == 0
	case reflect.Slice:
		return (*reflect.SliceHeader)(p).Len == 0
	case reflect.Array:
		return false
	case reflect.Map:
		v := reflect.NewAt(t, p).Elem()
		return v.Len() == 0
	case reflect.Bool:
		return !*(*bool)(p)
	case reflect.Interface:
		return *(*interface{})(p) == nil
	case reflect.Int:
		return *(*int)(p) == 0
	case reflect.Int8:
		return *(*int8)(p) == 0
	case reflect.Int16:
		return *(*int16)(p) == 0
	case reflect.Int32:
		return *(*int32)(p) == 0
	case reflect.Int64:
		return *(*int64)(p) == 0
	case reflect.Uint:
		return *(*uint)(p) == 0
	case reflect.Uint8:
		return *(*uint8)(p) == 0
	case reflect.Uint16:
		return *(*uint16)(p) == 0
	case reflect.Uint32:
		return *(*uint32)(p) == 0
	case reflect.Uint64:
		return *(*uint64)(p) == 0
	case reflect.Uintptr:
		return *(*uintptr)(p) == 0
	case reflect.Float32:
		return *(*float32)(p) == 0
	case reflect.Float64:
		return *(*float64)(p) == 0
	}
	return false
}

// tagOptions represents the arguments following a
// comma in a struct field's tag.
type tagOptions []string

// parseTag parses the content of a struct field
// tag and return the name and the list of options.
func parseTag(tag string) (string, tagOptions) {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx], strings.Split(tag[idx+1:], ",")
	}
	return tag, nil
}

// Contains reports whether a list of options
// contains a particular substring flag.
func (o tagOptions) Contains(opt string) bool {
	if len(o) == 0 {
		return false
	}
	for _, v := range o {
		if v == opt {
			return true
		}
	}
	return false
}
