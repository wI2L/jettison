package jettison

// An option represents a particular behavior
// of an encoder
type Option func(*encodeState)

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
	return func(es *encodeState) { es.timeLayout = layout }
}

// DurationFormat sets the format used to
// encode time.Duration values.
func DurationFormat(df DurationFmt) Option {
	return func(es *encodeState) { es.durationFmt = df }
}

// UnixTimestamp configures the encoder to encode
// time.Time values as Unix timestamps. This option
// has precedence over any time layout.
func UnixTimestamp(es *encodeState) {
	es.useTimestamps = true
}

// UnsortedMap disables map keys sort.
func UnsortedMap(es *encodeState) {
	es.unsortedMap = true
}

// ByteArrayAsString encodes byte arrays as
// raw JSON strings.
func ByteArrayAsString(es *encodeState) {
	es.byteArrayAsString = true
}

// RawByteSlices disables the default behavior that
// encodes byte slices as base64-encoded strings.
func RawByteSlices(es *encodeState) {
	es.noBase64Slice = true
}

// NilMapEmpty encodes nil Go maps as
// empty JSON objects, rather than null.
func NilMapEmpty(es *encodeState) {
	es.nilMapEmpty = true
}

// NilSliceEmpty encodes nil Go slices as
// empty JSON arrays, rather than null.
func NilSliceEmpty(es *encodeState) {
	es.nilSliceEmpty = true
}

// NoStringEscaping disables strings escaping.
func NoStringEscaping(es *encodeState) {
	es.noStringEscape = true
}

// NoHTMLEscaping disables the escaping of HTML
// characters when encoding JSON strings.
func NoHTMLEscaping(es *encodeState) {
	es.noHTMLEscape = true
}

// NoUTF8Coercion disables UTF-8 coercion
// when encoding JSON strings.
func NoUTF8Coercion(es *encodeState) {
	es.noUTF8Coercion = true
}
