package jettison

import (
	"context"
	"fmt"
	"time"
)

// defaultTimeLayout is the default layout used
// to format time.Time values. This is compliant
// with the ECMA specification and the JavaScript
// Date's toJSON method implementation.
const defaultTimeLayout = time.RFC3339Nano

// defaultDurationFmt is the default format used
// to encode time.Duration values.
const defaultDurationFmt = DurationNanoseconds

// An Option overrides the default encoding
// behavior of the MarshalOpts function.
type Option func(*encOpts)

type bitmask uint64

func (b *bitmask) set(f bitmask)      { *b |= f }
func (b *bitmask) has(f bitmask) bool { return *b&f != 0 }

const (
	unixTime bitmask = 1 << iota
	unsortedMap
	rawByteSlice
	byteArrayAsString
	nilMapEmpty
	nilSliceEmpty
	noStringEscaping
	noHTMLEscaping
	noUTF8Coercion
	noCompact
	noNumberValidation
)

type encOpts struct {
	ctx         context.Context
	timeLayout  string
	durationFmt DurationFmt
	flags       bitmask
	allowList   stringSet
	denyList    stringSet
}

func defaultEncOpts() encOpts {
	return encOpts{
		ctx:         context.TODO(),
		timeLayout:  defaultTimeLayout,
		durationFmt: defaultDurationFmt,
	}
}

func (eo *encOpts) apply(opts ...Option) {
	for _, opt := range opts {
		if opt != nil {
			opt(eo)
		}
	}
}

func (eo encOpts) validate() error {
	switch {
	case eo.ctx == nil:
		return fmt.Errorf("nil context")
	case eo.timeLayout == "":
		return fmt.Errorf("empty time layout")
	case !eo.durationFmt.valid():
		return fmt.Errorf("unknown duration format")
	default:
		return nil
	}
}

// isDeniedField returns whether a struct field
// identified by its name must be skipped during
// the encoding of a struct.
func (eo encOpts) isDeniedField(name string) bool {
	// The deny list has precedence and must
	// be checked first if it has entries.
	if eo.denyList != nil {
		if _, ok := eo.denyList[name]; ok {
			return true
		}
	}
	if eo.allowList != nil {
		if _, ok := eo.allowList[name]; !ok {
			return true
		}
	}
	return false
}

type stringSet map[string]struct{}

func fieldListToSet(list []string) stringSet {
	m := make(stringSet)
	for _, f := range list {
		m[f] = struct{}{}
	}
	return m
}

// UnixTime configures an encoder to encode
// time.Time values as Unix timestamps. This
// option, when used, has precedence over any
// time layout confiured.
func UnixTime() Option {
	return func(o *encOpts) { o.flags.set(unixTime) }
}

// UnsortedMap configures an encoder to skip
// the sort of map keys.
func UnsortedMap() Option {
	return func(o *encOpts) { o.flags.set(unsortedMap) }
}

// RawByteSlice configures an encoder to
// encode byte slices as raw JSON strings,
// rather than bas64-encoded strings.
func RawByteSlice() Option {
	return func(o *encOpts) { o.flags.set(rawByteSlice) }
}

// ByteArrayAsString configures an encoder
// to encode byte arrays as raw JSON strings.
func ByteArrayAsString() Option {
	return func(o *encOpts) { o.flags.set(byteArrayAsString) }
}

// NilMapEmpty configures an encoder to
// encode nil Go maps as empty JSON objects,
// rather than null.
func NilMapEmpty() Option {
	return func(o *encOpts) { o.flags.set(nilMapEmpty) }
}

// NilSliceEmpty configures an encoder to
// encode nil Go slices as empty JSON arrays,
// rather than null.
func NilSliceEmpty() Option {
	return func(o *encOpts) { o.flags.set(nilSliceEmpty) }
}

// NoStringEscaping configures an encoder to
// disable string escaping.
func NoStringEscaping() Option {
	return func(o *encOpts) { o.flags.set(noStringEscaping) }
}

// NoHTMLEscaping configures an encoder to
// disable the escaping of problematic HTML
// characters in JSON strings.
func NoHTMLEscaping() Option {
	return func(o *encOpts) { o.flags.set(noHTMLEscaping) }
}

// NoUTF8Coercion configures an encoder to
// disable UTF8 coercion that replace invalid
// bytes with the Unicode replacement rune.
func NoUTF8Coercion() Option {
	return func(o *encOpts) { o.flags.set(noUTF8Coercion) }
}

// NoNumberValidation configures an encoder to
// disable the validation of json.Number values.
func NoNumberValidation() Option {
	return func(o *encOpts) { o.flags.set(noNumberValidation) }
}

// NoCompact configures an encoder to disable
// the compaction of the JSON output produced
// by a call to MarshalJSON, or the content of
// a json.RawMessage.
// see https://golang.org/pkg/encoding/json/#Compact
func NoCompact() Option {
	return func(o *encOpts) { o.flags.set(noCompact) }
}

// TimeLayout sets the time layout used to encode
// time.Time values. The layout must be compatible
// with the Golang time package specification.
func TimeLayout(layout string) Option {
	return func(o *encOpts) {
		o.timeLayout = layout
	}
}

// DurationFormat sets the format used to encode
// time.Duration values.
func DurationFormat(format DurationFmt) Option {
	return func(o *encOpts) {
		o.durationFmt = format
	}
}

// WithContext sets the context to use during
// encoding. The context will be passed in to
// the AppendJSONContext method of types that
// implement the AppendMarshalerCtx interface.
func WithContext(ctx context.Context) Option {
	return func(o *encOpts) {
		o.ctx = ctx
	}
}

// AllowList sets the list of first-level fields
// which are to be considered when encoding a struct.
// The fields are identified by the name that is
// used in the final JSON payload.
// See DenyFields documentation for more information
// regarding joint use with this option.
func AllowList(fields []string) Option {
	m := fieldListToSet(fields)
	return func(o *encOpts) {
		o.allowList = m
	}
}

// DenyList is similar to AllowList, but conversely
// sets the list of fields to omit during encoding.
// When used in cunjunction with AllowList, denied
// fields have precedence over the allowed fields.
func DenyList(fields []string) Option {
	m := fieldListToSet(fields)
	return func(o *encOpts) {
		o.denyList = m
	}
}
