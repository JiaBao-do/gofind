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

func TestHumanSize(t *testing.T) {
	for n, want := range map[int64]string{0: "0 B", 1023: "1023 B", 1024: "1.0 KB", 1536: "1.5 KB", 5 << 20: "5.0 MB", 3 << 30: "3.0 GB"} {
		if got := humanSize(n); got != want {
			t.Errorf("humanSize(%d)=%q want %q", n, got, want)
		}
	}
}

func TestRowWidth(t *testing.T) {
	for _, w := range []int{10, 50, 70, 100} {
		if got := len([]rune(row(w, "a-very-long-file-name-that-overflows.txt", "2026-11-04 17:21", "TXT file", "1.0 KB"))); got != w {
			t.Errorf("row width %d: got %d", w, got)
		}
	}
}
