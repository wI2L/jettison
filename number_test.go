package jettison

import (
	"regexp"
	"testing"
)

func TestIsValidNumber(t *testing.T) {
	// Taken from https://golang.org/src/encoding/json/number_test.go
	// Regexp from: https://stackoverflow.com/a/13340826
	var re = regexp.MustCompile(
		`^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?$`,
	)
	valid := []string{
		"0",
		"-0",
		"1",
		"-1",
		"0.1",
		"-0.1",
		"1234",
		"-1234",
		"12.34",
		"-12.34",
		"12E0",
		"12E1",
		"12e34",
		"12E-0",
		"12e+1",
		"12e-34",
		"-12E0",
		"-12E1",
		"-12e34",
		"-12E-0",
		"-12e+1",
		"-12e-34",
		"1.2E0",
		"1.2E1",
		"1.2e34",
		"1.2E-0",
		"1.2e+1",
		"1.2e-34",
		"-1.2E0",
		"-1.2E1",
		"-1.2e34",
		"-1.2E-0",
		"-1.2e+1",
		"-1.2e-34",
		"0E0",
		"0E1",
		"0e34",
		"0E-0",
		"0e+1",
		"0e-34",
		"-0E0",
		"-0E1",
		"-0e34",
		"-0E-0",
		"-0e+1",
		"-0e-34",
	}
	for _, tt := range valid {
		if !isValidNumber(tt) {
			t.Errorf("%s should be valid", tt)
		}
		if !re.MatchString(tt) {
			t.Errorf("%s should be valid but regexp does not match", tt)
		}
	}
	invalid := []string{
		"",
		"-",
		"invalid",
		"1.0.1",
		"1..1",
		"-1-2",
		"012a42",
		"01.2",
		"012",
		"12E12.12",
		"1e2e3",
		"1e+-2",
		"1e--23",
		"1e",
		"e1",
		"1e+",
		"1ea",
		"1a",
		"1.a",
		"1.",
		"01",
		"1.e1",
	}
	for _, tt := range invalid {
		if isValidNumber(tt) {
			t.Errorf("%s should be invalid", tt)
		}
		if re.MatchString(tt) {
			t.Errorf("%s should be invalid but matches regexp", tt)
		}
	}
}
