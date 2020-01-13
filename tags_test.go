package jettison

import "testing"

func TestParseTag(t *testing.T) {
	testdata := []struct {
		tag  string
		name string
		opts []string
	}{
		{"", "", nil},
		{"foobar", "foobar", nil},
		{"foo,bar", "foo", []string{"bar"}},
		{"bar,", "bar", nil},
		{"a,b,c,", "a", []string{"b", "c"}},
		{" foo , bar ,", " foo ", []string{" bar "}},
		{",bar", "", []string{"bar"}},
		{", ", "", []string{" "}},
		{",", "", nil},
		{"bar, ,foo", "bar", []string{" ", "foo"}},
	}
	for _, v := range testdata {
		name, opts := parseTag(v.tag)
		if name != v.name {
			t.Errorf("tag name: got %q, want %q", name, v.name)
		}
		for _, opt := range v.opts {
			if !opts.Contains(opt) {
				t.Errorf("missing tag option %q", opt)
			}
		}
	}
}
