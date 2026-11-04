package main

import "testing"

func TestFuzzy(t *testing.T) {
	cases := []struct {
		p, s string
		want bool
	}{
		{"", "anything", true},
		{"mgo", "main.go", true},
		{"MAIN", "main.go", true},
		{"gm", "main.go", false},
		{"é", "café.txt", true},
		{"xyz", "main.go", false},
	}
	for _, c := range cases {
		if got := fuzzy(c.p, c.s); got != c.want {
			t.Errorf("fuzzy(%q,%q)=%v want %v", c.p, c.s, got, c.want)
		}
	}
}
