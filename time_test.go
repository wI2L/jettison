package jettison

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"
)

func TestDurationFmtString(t *testing.T) {
	testdata := []struct {
		fmt DurationFmt
		str string
	}{
		{DurationString, "str"},
		{DurationMinutes, "min"},
		{DurationSeconds, "s"},
		{DurationMilliseconds, "ms"},
		{DurationMicroseconds, "μs"},
		{DurationNanoseconds, "nanosecond"},
		{DurationFmt(-1), "unknown"},
		{DurationFmt(6), "unknown"},
	}
	for _, tt := range testdata {
		if s := tt.fmt.String(); s != tt.str {
			t.Errorf("got %q, want %q", s, tt.str)
		}
	}
}

func TestAppendDuration(t *testing.T) {
	// Taken from https://golang.org/src/time/time_test.go
	var testdata = []struct {
		str string
		dur time.Duration
	}{
		{"0s", 0},
		{"1ns", 1 * time.Nanosecond},
		{"1.1µs", 1100 * time.Nanosecond},
		{"2.2ms", 2200 * time.Microsecond},
		{"3.3s", 3300 * time.Millisecond},
		{"4m5s", 4*time.Minute + 5*time.Second},
		{"4m5.001s", 4*time.Minute + 5001*time.Millisecond},
		{"5h6m7.001s", 5*time.Hour + 6*time.Minute + 7001*time.Millisecond},
		{"8m0.000000001s", 8*time.Minute + 1*time.Nanosecond},
		{"2562047h47m16.854775807s", 1<<63 - 1},
		{"-2562047h47m16.854775808s", -1 << 63},
	}
	for _, tt := range testdata {
		buf := appendDuration(make([]byte, 0, 32), tt.dur)

		if s := string(buf); s != tt.str {
			t.Errorf("got %q, want %q", s, tt.str)
		}
		if tt.dur > 0 {
			buf = make([]byte, 0, 32)
			buf = appendDuration(buf, -tt.dur)
			if s := string(buf); s != "-"+tt.str {
				t.Errorf("got %q, want %q", s, "-"+tt.str)
			}
		}
	}
}

func TestAppendRFC3339Time(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	var (
		bat int
		buf []byte
	)
	for _, nano := range []bool{true, false} {
		for i := 0; i < 1e3; i++ {
			if testing.Short() && i > 1e2 {
				break
			}
			// Generate a location with a random offset
			// between 0 and 12 hours.
			off := rand.Intn(12*60 + 1)
			if rand.Intn(2) == 0 { // coin flip
				off = -off
			}
			loc := time.FixedZone("", off)

			// Generate a random time between now and the
			// Unix epoch, with random fractional seconds.
			ts := rand.Int63n(time.Now().Unix() + 1)
			tm := time.Unix(ts, rand.Int63n(999999999+1)).In(loc)

			layout := time.RFC3339
			if nano {
				layout = time.RFC3339Nano
			}
			bat = len(buf)
			buf = appendRFC3339Time(tm, buf, nano)

			// The time encodes with double-quotes.
			want := strconv.Quote(tm.Format(layout))

			if s := string(buf[bat:]); s != want {
				t.Errorf("got %s, want %s", s, want)
			}
		}
	}
}

//nolint:scopelint
func BenchmarkRFC3339Time(b *testing.B) {
	if testing.Short() {
		b.SkipNow()
	}
	tm := time.Now()

	for _, tt := range []struct {
		name   string
		layout string
	}{
		{"", time.RFC3339},
		{"-nano", time.RFC3339Nano},
	} {
		b.Run(fmt.Sprintf("%s%s", "jettison", tt.name), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				buf := make([]byte, 32)
				appendRFC3339Time(tm, buf, tt.layout == time.RFC3339Nano)
			}
		})
		b.Run(fmt.Sprintf("%s%s", "standard", tt.name), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				buf := make([]byte, 32)
				tm.AppendFormat(buf, tt.layout)
			}
		})
	}
}
