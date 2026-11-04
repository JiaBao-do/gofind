package main

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/glamour"
)

const previewLimit = 64 * 1024

// readPreview returns the text shown in the preview pane for path; width is used to wrap Markdown.
func readPreview(path string, isDir bool, width int) string {
	if isDir {
		entries, err := os.ReadDir(path)
		if err != nil {
			return err.Error()
		}
		var b strings.Builder
		for _, e := range entries {
			b.WriteString(e.Name())
			if e.IsDir() {
				b.WriteByte('/')
			}
			b.WriteByte('\n')
		}
		return b.String()
	}

	var text string
	var err error
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv", ".tsv":
		text, err = previewCSV(path)
	case ".docx":
		text, err = previewDocx(path)
	case ".xlsx":
		text, err = previewXlsx(path)
	case ".pdf":
		text, err = previewPDF(path)
	case ".md", ".markdown":
		text, err = previewMarkdown(path, width)
	default:
		text, err = previewText(path)
	}
	if err != nil {
		return err.Error()
	}
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r", ""), "\t", "    ")
}

func previewText(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf, err := io.ReadAll(io.LimitReader(f, previewLimit))
	if err != nil {
		return "", err
	}
	if bytes.IndexByte(buf, 0) >= 0 {
		return "(binary file)", nil
	}
	return string(buf), nil
}

// table lays rows out in aligned columns.
func table(rows [][]string) string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		fmt.Fprintln(w, strings.Join(r, "\t"))
	}
	w.Flush()
	return b.String()
}

// shortcut: only the first 64 KB of a CSV is shown, enough for a preview.
func previewCSV(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	r := csv.NewReader(io.LimitReader(f, previewLimit))
	if strings.EqualFold(filepath.Ext(path), ".tsv") {
		r.Comma = '\t'
	}
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var rows [][]string
	for {
		rec, err := r.Read()
		if err != nil {
			break // EOF or the row cut by the 64 KB limit
		}
		for i := range rec {
			rec[i] = strings.ReplaceAll(rec[i], "\t", " ")
		}
		rows = append(rows, rec)
	}
	return table(rows), nil
}

// zipFile reads one member of an Office (zip) document; missing members return nil.
func zipFile(z *zip.ReadCloser, name string) ([]byte, error) {
	for _, f := range z.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(io.LimitReader(rc, 32<<20))
	}
	return nil, nil
}

// previewDocx extracts paragraph text from word/document.xml.
func previewDocx(path string) (string, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("not a valid .docx: %w", err)
	}
	defer z.Close()
	data, err := zipFile(z, "word/document.xml")
	if err != nil || data == nil {
		return "", fmt.Errorf("not a valid .docx: no document body")
	}
	var b strings.Builder
	d := xml.NewDecoder(bytes.NewReader(data))
	inText := false
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				inText = true
			case "tab":
				b.WriteByte(' ')
			case "br":
				b.WriteByte('\n')
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				b.WriteByte('\n')
			}
		case xml.CharData:
			if inText {
				b.Write(t)
			}
		}
	}
	return b.String(), nil
}

// previewXlsx shows the first worksheet as a table.
func previewXlsx(path string) (string, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("not a valid .xlsx: %w", err)
	}
	defer z.Close()

	var shared []string
	if data, err := zipFile(z, "xl/sharedStrings.xml"); err == nil && data != nil {
		var sst struct {
			SI []struct {
				T string `xml:"t"`
				R []struct {
					T string `xml:"t"`
				} `xml:"r"`
			} `xml:"si"`
		}
		if err := xml.Unmarshal(data, &sst); err != nil {
			return "", fmt.Errorf("reading shared strings: %w", err)
		}
		for _, si := range sst.SI {
			s := si.T
			for _, r := range si.R { // rich text is split into runs
				s += r.T
			}
			shared = append(shared, s)
		}
	}

	// shortcut: always the first sheet file, not the workbook's first tab by order; fine for most files.
	data, err := zipFile(z, "xl/worksheets/sheet1.xml")
	if err != nil || data == nil {
		return "", fmt.Errorf("not a valid .xlsx: no first worksheet")
	}
	var ws struct {
		Rows []struct {
			C []struct {
				R  string `xml:"r,attr"`
				T  string `xml:"t,attr"`
				V  string `xml:"v"`
				Is struct {
					T string `xml:"t"`
				} `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(data, &ws); err != nil {
		return "", fmt.Errorf("reading worksheet: %w", err)
	}
	var rows [][]string
	for i, row := range ws.Rows {
		if i >= 500 {
			break
		}
		var out []string
		for _, c := range row.C {
			col := colIndex(c.R)
			for len(out) < col { // empty cells are omitted from the XML
				out = append(out, "")
			}
			v := c.V
			switch c.T {
			case "s":
				if n, err := strconv.Atoi(c.V); err == nil && n >= 0 && n < len(shared) {
					v = shared[n]
				}
			case "inlineStr":
				v = c.Is.T
			}
			out = append(out, strings.ReplaceAll(v, "\t", " "))
		}
		rows = append(rows, out)
	}
	return table(rows), nil
}

// colIndex turns a cell ref like "C7" into a 0-based column (2); bad refs give the next column.
func colIndex(ref string) int {
	n := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		n = n*26 + int(r-'A'+1)
	}
	return n - 1
}

// previewMarkdown renders Markdown with terminal styling; falls back to plain text if rendering fails.
func previewMarkdown(path string, width int) (string, error) {
	src, err := previewText(path)
	if err != nil {
		return "", err
	}
	r, err := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(max(width-2, 20)))
	if err != nil {
		return src, nil
	}
	out, err := r.Render(src)
	if err != nil {
		return src, nil
	}
	return out, nil
}

// previewPDF uses pdftotext (poppler) when installed; Go has no PDF reader in the stdlib.
func previewPDF(path string) (string, error) {
	bin, err := exec.LookPath("pdftotext")
	if err != nil {
		return "(PDF preview needs pdftotext: install poppler, e.g. `winget install oschwartz10612.Poppler`, `brew install poppler` or `apt install poppler-utils`)", nil
	}
	out, err := exec.Command(bin, "-layout", "-l", "10", path, "-").Output()
	if err != nil {
		return "", fmt.Errorf("pdftotext: %w", err)
	}
	return string(out), nil
}
