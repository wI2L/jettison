package jettison

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"sync"
	"unicode"
)

const validChars = "!#$%&()*+-./:<=>?@[]^_{|}~ "

var fieldsCache sync.Map // map[reflect.Type][]field

type seq struct {
	offset uintptr
	indir  bool
}

type field struct {
	typ        reflect.Type
	name       string
	keyNonEsc  []byte
	keyEscHTML []byte
	index      []int
	tag        bool
	quoted     bool
	omitEmpty  bool
	omitNil    bool
	instr      instruction
	empty      emptyFunc

	// embedSeq represents the sequence of offsets
	// and indirections to follow to reach the field
	// through one or more anonymous fields.
	embedSeq []seq
}

type typeCount map[reflect.Type]int

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

// cachedFields is similar to structFields, but uses a
// cache to avoid repeated work.
func cachedFields(t reflect.Type) []field {
	if f, ok := fieldsCache.Load(t); ok {
		return f.([]field)
	}
	f, _ := fieldsCache.LoadOrStore(t, structFields(t))
	return f.([]field)
}

// structFields returns a list of fields that should be
// encoded for the given struct type. The algorithm is
// breadth-first search over the set of structs to include,
// the top one and then any reachable anonymous structs.
func structFields(t reflect.Type) []field {
	var (
		flds []field
		ccnt typeCount
		curr = []field{}
		next = []field{{typ: t}}
		ncnt = make(typeCount)
		seen = make(map[reflect.Type]bool)
	)
	for len(next) > 0 {
		curr, next = next, curr[:0]
		ccnt, ncnt = ncnt, make(map[reflect.Type]int)

		for _, f := range curr {
			if seen[f.typ] {
				continue
			}
			seen[f.typ] = true
			// Scan the type for fields to encode.
			flds, next = scanFields(f, flds, next, ccnt, ncnt)
		}
	}
	sortFields(flds)

	flds = filterByVisibility(flds)

	// Sort fields by their index sequence.
	sort.Sort(byIndex(flds))

	return flds
}

// sortFields sorts the fields by name, breaking ties
// with depth, then whether the field name come from
// the JSON tag, and finally with the index sequence.
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
		case strings.ContainsRune(validChars, c):
			// Backslash and quote chars are reserved, but
			// otherwise any punctuation chars are allowed
			// in a tag name.
		case !unicode.IsLetter(c) && !unicode.IsDigit(c):
			return false
		}
	}
	return true
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
		// the dominant field that survives.
		if dominant, ok := dominantField(fields[i : i+adv]); ok {
			ret = append(ret, dominant)
		}
	}
	return ret
}

func typeByIndex(t reflect.Type, index []int) reflect.Type {
	for _, i := range index {
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		t = t.Field(i).Type
	}
	return t
}

// dominantField looks through the fields, all of which
// are known to have the same name, to find the single
// field that dominates the others using Go's embedding
// rules, modified by the presence of JSON tags. If there
// are multiple top-level fields, it returns false. This
// condition is an error in Go, and all fields are skipped.
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
		// Parse name and options from the content
		// of the JSON tag.
		name, opts := parseTag(tag)
		if !isValidFieldName(name) {
			name = ""
		}
		index := make([]int, len(f.index)+1)
		copy(index, f.index)
		index[len(f.index)] = i

		typ := sf.Type
		isPtr := typ.Kind() == reflect.Ptr
		if typ.Name() == "" && isPtr {
			typ = typ.Elem()
		}
		// If the field is a named embedded struct or a
		// simple field, record it and its index sequence.
		if name != "" || !sf.Anonymous || typ.Kind() != reflect.Struct {
			tagged := name != ""
			// If a name is not present in the tag,
			// use the struct field's name instead.
			if name == "" {
				name = sf.Name
			}
			// Build HTML escaped field key.
			escBuf.Reset()
			_, _ = escBuf.WriteString(`"`)
			json.HTMLEscape(&escBuf, []byte(name))
			_, _ = escBuf.WriteString(`":`)

			nf := field{
				typ:        typ,
				name:       name,
				tag:        tagged,
				index:      index,
				omitEmpty:  opts.Contains("omitempty"),
				omitNil:    opts.Contains("omitnil"),
				quoted:     opts.Contains("string") && isBasicType(typ),
				keyNonEsc:  []byte(`"` + name + `":`),
				keyEscHTML: append([]byte(nil), escBuf.Bytes()...),  // copy
				embedSeq:   append(f.embedSeq[:0:0], f.embedSeq...), // clone
			}
			// Add final offset to sequences.
			nf.embedSeq = append(nf.embedSeq, seq{sf.Offset, false})
			fields = append(fields, nf)

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
		ncnt[typ]++
		if ncnt[typ] == 1 {
			next = append(next, field{
				typ:      typ,
				name:     typ.Name(),
				index:    index,
				embedSeq: append(f.embedSeq, seq{sf.Offset, isPtr}),
			})
		}
	}
	return fields, next
}
