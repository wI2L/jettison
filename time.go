package jettison

import "time"

var zeroDuration = []byte("0s")

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

// appendDurations appends the textual representation
// of d to dst and returns the extended buffer.
// Adapted from https://golang.org/src/time/time.go.
func appendDuration(dst []byte, d time.Duration) []byte {
	l := len(dst)
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
		dst[l] = 's'
		l--
		switch {
		case u == 0:
			return zeroDuration
		case u < uint64(time.Microsecond):
			prec = 0
			dst[l] = 'n'
		case u < uint64(time.Millisecond):
			prec = 3
			// U+00B5 'µ' micro sign is 0xC2 0xB5.
			// Need room for two bytes.
			l--
			copy(dst[l:], "µ")
		default: // Format as milliseconds.
			prec = 6
			dst[l] = 'm'
		}
		l, u = fmtFrac(dst[:l], u, prec)
		l = fmtInt(dst[:l], u)
	} else {
		l--
		dst[l] = 's'

		l, u = fmtFrac(dst[:l], u, 9)

		// Format as seconds.
		l = fmtInt(dst[:l], u%60)
		u /= 60

		// Format as minutes.
		if u > 0 {
			l--
			dst[l] = 'm'
			l = fmtInt(dst[:l], u%60)
			u /= 60

			// Format as hours. Stop there, because
			// days can be different lengths.
			if u > 0 {
				l--
				dst[l] = 'h'
				l = fmtInt(dst[:l], u)
			}
		}
	}
	if n {
		l--
		dst[l] = '-'
	}
	return dst[l:]
}
