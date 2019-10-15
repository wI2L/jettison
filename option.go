package jettison

// Option overrides a particular behavior
// of an encoder
type Option func(*encodeOpts)

// DurationFmt represents the format used
// to encode a time.Duration value.
type DurationFmt int

// Duration formats.
const (
	DurationString = iota
	DurationMinutes
	DurationSeconds
	DurationMilliseconds
	DurationMicroseconds
	DurationNanoseconds
)

// String implements the fmt.Stringer interface.
func (df DurationFmt) String() string {
	if df < DurationString || df > DurationNanoseconds {
		return "unknown"
	}
	names := []string{
		"str", "min", "s", "ms", "μs", "nanosecond",
	}
	return names[df]
}

// TimeLayout sets the time layout used to
// encode time.Time values.
func TimeLayout(layout string) Option {
	return func(o *encodeOpts) {
		o.timeLayout = layout
	}
}

// DurationFormat sets the format used to
// encode time.Duration values.
func DurationFormat(df DurationFmt) Option {
	return func(o *encodeOpts) {
		o.durationFmt = df
	}
}

// UnixTimestamp configures the encoder to encode
// time.Time values as Unix timestamps. This option
// has precedence over any time layout.
func UnixTimestamp() Option {
	return func(o *encodeOpts) {
		o.useTimestamps = true
	}
}

// UnsortedMap disables map keys sort.
func UnsortedMap() Option {
	return func(o *encodeOpts) {
		o.unsortedMap = true
	}
}

// ByteArrayAsString encodes byte arrays as
// raw JSON strings.
func ByteArrayAsString() Option {
	return func(o *encodeOpts) {
		o.byteArrayAsString = true
	}
}

// RawByteSlice disables the default behavior that
// encodes byte slices as base64-encoded strings.
func RawByteSlice() Option {
	return func(o *encodeOpts) {
		o.noBase64Slice = true
	}
}

// NilMapEmpty encodes nil Go maps as
// empty JSON objects, rather than null.
func NilMapEmpty() Option {
	return func(o *encodeOpts) {
		o.nilMapEmpty = true
	}
}

// NilSliceEmpty encodes nil Go slices as
// empty JSON arrays, rather than null.
func NilSliceEmpty() Option {
	return func(o *encodeOpts) {
		o.nilSliceEmpty = true
	}
}

// NoStringEscaping disables strings escaping.
func NoStringEscaping() Option {
	return func(o *encodeOpts) {
		o.noStringEscape = true
	}
}

// NoHTMLEscaping disables the escaping of HTML
// characters when encoding JSON strings.
func NoHTMLEscaping() Option {
	return func(o *encodeOpts) {
		o.noHTMLEscape = true
	}
}

// NoUTF8Coercion disables UTF-8 coercion
// when encoding JSON strings.
func NoUTF8Coercion() Option {
	return func(o *encodeOpts) {
		o.noUTF8Coercion = true
	}
}
