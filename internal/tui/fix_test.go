package tui

import (
	"reflect"
	"testing"
)

func TestParseEpisodeInput(t *testing.T) {
	cases := []struct {
		in     string
		cur    int
		season int
		eps    []int
		ok     bool
	}{
		{"S01E02", 5, 1, []int{2}, true},
		{"s2e10", -1, 2, []int{10}, true},
		{"1x05", -1, 1, []int{5}, true},
		{"S01E02-E04", -1, 1, []int{2, 3, 4}, true},
		{"7", 3, 3, []int{7}, true},
		{"e7", -1, 1, []int{7}, true},
		{"hello", 1, 0, nil, false},
	}
	for _, c := range cases {
		s, e, ok := parseEpisodeInput(c.in, c.cur)
		if ok != c.ok || s != c.season || !reflect.DeepEqual(e, c.eps) {
			t.Errorf("%q: got %d %v %v", c.in, s, e, ok)
		}
	}
}
