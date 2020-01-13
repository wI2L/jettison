package jettison

import "strings"

// tagOptions represents the arguments following
// a comma in a struct field's tag.
type tagOptions []string

// parseTag parses the content of a struct field
// tag and return the name and list of options.
func parseTag(tag string) (string, tagOptions) {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx], strings.Split(tag[idx+1:], ",")
	}
	return tag, nil
}

// Contains returns whether a list of options
// contains a particular substring flag.
func (opts tagOptions) Contains(name string) bool {
	if len(opts) == 0 {
		return false
	}
	for _, o := range opts {
		if o == name {
			return true
		}
	}
	return false
}
