package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeZip builds a minimal Office-style zip with the given members.
func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, body := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestPreviewFormats(t *testing.T) {
	dir := t.TempDir()

	csvPath := filepath.Join(dir, "a.csv")
	writeFile(t, csvPath, "name,qty\napple,3\n\"b,anana\",12\n")

	docx := filepath.Join(dir, "a.docx")
	writeZip(t, docx, map[string]string{"word/document.xml": `<w:document xmlns:w="x"><w:body>` +
		`<w:p><w:r><w:t>Hello</w:t></w:r><w:r><w:t xml:space="preserve"> world</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>Second</w:t></w:r></w:p></w:body></w:document>`})

	xlsx := filepath.Join(dir, "a.xlsx")
	writeZip(t, xlsx, map[string]string{
		"xl/sharedStrings.xml": `<sst><si><t>Item</t></si><si><r><t>Pe</t></r><r><t>ar</t></r></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData>` +
			`<row><c r="A1" t="s"><v>0</v></c><c r="C1"><v>42</v></c></row>` +
			`<row><c r="A2" t="s"><v>1</v></c><c r="B2" t="inlineStr"><is><t>inline</t></is></c></row>` +
			`</sheetData></worksheet>`,
	})

	md := filepath.Join(dir, "a.md")
	writeFile(t, md, "# Title\n\nsome **bold** text\n")

	cases := []struct {
		path string
		want []string
	}{
		{csvPath, []string{"name     qty\napple    3\nb,anana  12"}},
		{docx, []string{"Hello world\nSecond"}},
		{xlsx, []string{"Item", "42", "Pear", "inline"}},
		{md, []string{"Title", "bold"}},
	}
	for _, c := range cases {
		got := readPreview(c.path, false, 60)
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: preview %q missing %q", filepath.Base(c.path), got, w)
			}
		}
	}

	if got := readPreview(filepath.Join(dir, "a.docx"), false, 60); strings.Contains(got, "<w:") {
		t.Errorf("docx preview leaked XML: %q", got)
	}
}

func TestColIndex(t *testing.T) {
	for ref, want := range map[string]int{"A1": 0, "C7": 2, "Z3": 25, "AA10": 26} {
		if got := colIndex(ref); got != want {
			t.Errorf("colIndex(%q)=%d want %d", ref, got, want)
		}
	}
}
