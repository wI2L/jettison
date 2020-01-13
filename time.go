package jettison

import "time"

const epoch = 62135683200 // 1970-01-01T00:00:00

// DurationFmt represents the format used
// to encode a time.Duration value.
type DurationFmt int

// DurationFmt constants.
const (
	DurationString DurationFmt = iota
	DurationMinutes
	DurationSeconds
	DurationMilliseconds
	DurationMicroseconds
	DurationNanoseconds // default
)

// String implements the fmt.Stringer
// interface for DurationFmt.
func (f DurationFmt) String() string {
	if !f.valid() {
		return "unknown"
	}
	return durationFmtStr[f]
}

func (f DurationFmt) valid() bool {
	return f >= DurationString && f <= DurationNanoseconds
}

var (
	zeroDuration   = []byte("0s")
	durationFmtStr = []string{"str", "min", "s", "ms", "μs", "nanosecond"}
	dayOffset      = [13]uint16{0, 306, 337, 0, 31, 61, 92, 122, 153, 184, 214, 245, 275}
)

// appendDuration appends the textual representation
// of d to the tail of dst and returns the extended buffer.
// Adapted from https://golang.org/src/time/time.go.
func appendDuration(dst []byte, d time.Duration) []byte {
	var buf [32]byte

	l := len(buf)
	u := uint64(d)
	n := d < 0
	if n {
		u = -u
	}
	if u < uint64(time.Second) {
		// Special case: if duration is smaller than
		// a second, use smaller units, like 1.2ms
		var prec int
		l--
		buf[l] = 's'
		l--
		switch {
		case u == 0:
			return zeroDuration
		case u < uint64(time.Microsecond):
			prec = 0
			buf[l] = 'n'
		case u < uint64(time.Millisecond):
			prec = 3
			// U+00B5 'µ' micro sign is 0xC2 0xB5.
			// Need room for two bytes.
			l--
			copy(buf[l:], "µ")
		default: // Format as milliseconds.
			prec = 6
			buf[l] = 'm'
		}
		l, u = fmtFrac(buf[:l], u, prec)
		l = fmtInt(buf[:l], u)
	} else {
		l--
		buf[l] = 's'

		l, u = fmtFrac(buf[:l], u, 9)

		// Format as seconds.
		l = fmtInt(buf[:l], u%60)
		u /= 60

		// Format as minutes.
		if u > 0 {
			l--
			buf[l] = 'm'
			l = fmtInt(buf[:l], u%60)
			u /= 60

			// Format as hours. Stop there, because
			// days can be different lengths.
			if u > 0 {
				l--
				buf[l] = 'h'
				l = fmtInt(buf[:l], u)
			}
		}
	}
	if n {
		l--
		buf[l] = '-'
	}
	return append(dst, buf[l:]...)
}

// fmtInt formats v into the tail of buf.
// It returns the index where the output begins.
// Taken from https://golang.org/src/time/time.go.
func fmtInt(buf []byte, v uint64) int {
	w := len(buf)
	if v == 0 {
		w--
		buf[w] = '0'
	} else {
		for v > 0 {
			w--
			buf[w] = byte(v%10) + '0'
			v /= 10
		}
	}
	return w
}

// fmtFrac formats the fraction of v/10**prec (e.g., ".12345")
// into the tail of buf, omitting trailing zeros. It omits the
// decimal point too when the fraction is 0. It returns the
// index where the output bytes begin and the value v/10**prec.
// Taken from https://golang.org/src/time/time.go.
func fmtFrac(buf []byte, v uint64, prec int) (nw int, nv uint64) {
	// Omit trailing zeros up to and including decimal point.
	w := len(buf)
	print := false
	for i := 0; i < prec; i++ {
		digit := v % 10
		print = print || digit != 0
		if print {
			w--
			buf[w] = byte(digit) + '0'
		}
		v /= 10
	}
	if print {
		w--
		buf[w] = '.'
	}
	return w, v
}

func rdnToYmd(rdn uint32) (uint16, uint16, uint16) {
	// Rata Die algorithm by Peter Baum.
	var (
		Z = rdn + 306
		H = 100*Z - 25
		A = H / 3652425
		B = A - (A >> 2)
		y = (100*B + H) / 36525
		d = B + Z - (1461 * y >> 2)
		m = (535*d + 48950) >> 14
	)
	if m > 12 {
		y++
		m -= 12
	}
	return uint16(y), uint16(m), uint16(d) - dayOffset[m]
}

// appendRFC3339Time appends the RFC3339 textual representation
// of t to the tail of dst and returns the extended buffer.
// Adapted from https://github.com/chansen/c-timestamp.
func appendRFC3339Time(t time.Time, dst []byte, nano bool) []byte {
	var buf [37]byte

	// Base layout chars with opening quote.
	buf[0], buf[5], buf[8], buf[11], buf[14], buf[17] = '"', '-', '-', 'T', ':', ':'

	// Year.
	_, offset := t.Zone()
	sec := t.Unix() + int64(offset) + epoch
	y, m, d := rdnToYmd(uint32(sec / 86400))
	for i := 4; i >= 1; i-- {
		buf[i] = byte(y%10) + '0'
		y /= 10
	}
	buf[7], m = byte(m%10)+'0', m/10 // month
	buf[6] = byte(m%10) + '0'

	buf[10], d = byte(d%10)+'0', d/10 // day
	buf[9] = byte(d%10) + '0'

	// Hours/minutes/seconds.
	s := sec % 86400
	buf[19], s = byte(s%10)+'0', s/10
	buf[18], s = byte(s%06)+'0', s/6
	buf[16], s = byte(s%10)+'0', s/10
	buf[15], s = byte(s%06)+'0', s/6
	buf[13], s = byte(s%10)+'0', s/10
	buf[12], _ = byte(s%10)+'0', 0

	n := 20

	// Fractional second precision.
	nsec := t.Nanosecond()
	if nano && nsec != 0 {
		buf[n] = '.'
		u := nsec
		for i := 9; i >= 1; i-- {
			buf[n+i] = byte(u%10) + '0'
			u /= 10
		}
		// Remove trailing zeros.
		var rpad int
		for i := 9; i >= 1; i-- {
			if buf[n+i] == '0' {
				rpad++
			} else {
				break
			}
		}
		n += 10 - rpad
	}
	// Zone.
	if offset == 0 {
		buf[n] = 'Z'
		n++
	} else {
		var z int
		zone := offset / 60 // convert to minutes
		if zone < 0 {
			buf[n] = '-'
			z = -zone
		} else {
			buf[n] = '+'
			z = zone
		}
		buf[n+3] = ':'
		buf[n+5], z = byte(z%10)+'0', z/10
		buf[n+4], z = byte(z%06)+'0', z/6
		buf[n+2], z = byte(z%10)+'0', z/10
		buf[n+1], _ = byte(z%10)+'0', 0
		n += 6
	}
	// Finally, add the closing quote.
	// It's position depends on the presence
	// of the fractional seconds and/or the
	// timezone offset.
	buf[n] = '"'

	return append(dst, buf[:n+1]...)
}
