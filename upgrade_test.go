package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestUpgradeNeeded(t *testing.T) {
	tests := []struct {
		current, latest string
		want            bool
	}{
		{"v1.0.0", "v1.1.0", true},
		{"v1.1.0", "v1.1.0", false},
		{"v1.2.0", "v1.1.9", false},
		{"v1.9.9", "v1.10.0", true},
		{"dev", "v1.1.0", true},
		{"v1.1.0-3-gabc123-dirty", "v1.1.0", true},
	}
	for _, tt := range tests {
		if got, why := upgradeNeeded(tt.current, tt.latest); got != tt.want {
			t.Errorf("upgradeNeeded(%q, %q) = %v (%s), want %v", tt.current, tt.latest, got, why, tt.want)
		}
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("binary")
	sum := sha256.Sum256(data)
	sums := []byte("deadbeef  gofind-linux-amd64\n" + hex.EncodeToString(sum[:]) + " *gofind-windows-amd64.exe\n")
	if err := verifyChecksum(sums, "gofind-windows-amd64.exe", data); err != nil {
		t.Errorf("valid checksum rejected: %v", err)
	}
	if err := verifyChecksum(sums, "gofind-linux-amd64", data); err == nil {
		t.Error("wrong checksum accepted")
	}
	if err := verifyChecksum(sums, "gofind-darwin-arm64", data); err == nil {
		t.Error("missing checksum accepted")
	}
}

func TestReplaceExecutable(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "gofind.exe")
	writeFile(t, exe, "old")
	if err := replaceExecutable(exe, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, exe); got != "new" {
		t.Errorf("exe = %q, want new", got)
	}
	if _, err := os.Stat(exe + ".new"); err == nil {
		t.Error("temporary .new file left behind")
	}
}

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
