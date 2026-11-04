package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	paneStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))
	selStyle  = lipgloss.NewStyle().Reverse(true)
	dirStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	dimStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

type model struct {
	dir     string
	entries []os.DirEntry
	infos   map[string]fs.FileInfo // stat of each entry in the current dir, read once at load
	filter  string
	cursor  int
	preview string
	scroll  int    // first preview line shown
	focusR  bool   // Tab moved focus to the preview pane
	info    string // one-line summary shown above the preview
	err     error
	w, h    int
}

// fuzzy reports whether every rune of pattern appears in s in order (fzf-style), case-insensitive.
func fuzzy(pattern, s string) bool {
	s = strings.ToLower(s)
	for _, r := range strings.ToLower(pattern) {
		i := strings.IndexRune(s, r)
		if i < 0 {
			return false
		}
		s = s[i+len(string(r)):]
	}
	return true
}

func (m *model) load(dir string, selectName string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		m.err = err
		return
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	m.dir, m.entries, m.filter, m.cursor, m.preview, m.info, m.err = dir, entries, "", 0, "", "", nil
	m.scroll, m.focusR = 0, false
	m.infos = map[string]fs.FileInfo{}
	for i, e := range entries {
		if e.Name() == selectName {
			m.cursor = i
		}
		if fi, err := e.Info(); err == nil {
			m.infos[e.Name()] = fi
		}
	}
}

// humanSize formats bytes as B/KB/MB/GB/TB (1024-based).
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < 3; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

// size returns the cached size of an entry, 0 if its stat failed.
func (m model) size(name string) int64 {
	fi := m.infos[name]
	if fi == nil {
		return 0
	}
	return fi.Size()
}

// modified returns the cached modification time formatted like Explorer, "" if unknown.
func (m model) modified(name string) string {
	fi := m.infos[name]
	if fi == nil {
		return ""
	}
	return fi.ModTime().Format("2006-01-02 15:04")
}

// fileType mimics Explorer's Type column: "File folder", "GO file", "File".
func fileType(e os.DirEntry) string {
	if e.IsDir() {
		return "File folder"
	}
	ext := strings.TrimPrefix(filepath.Ext(e.Name()), ".")
	if ext == "" {
		return "File"
	}
	return strings.ToUpper(ext) + " file"
}

// fit pads or cuts s to exactly w display columns.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = clip(s, w, 1)
	if gap := w - lipgloss.Width(s); gap > 0 {
		s += strings.Repeat(" ", gap)
	}
	return s
}

// row lays out Name | Date modified | Type | Size; narrow panes drop Type, then Date.
func row(w int, name, mod, typ, size string) string {
	const modW, typW, sizeW = 17, 13, 10
	switch {
	case w >= 20+modW+typW+sizeW:
		return fit(name, w-modW-typW-sizeW) + fit(mod, modW) + fit(typ, typW) + fmt.Sprintf("%*s", sizeW, size)
	case w >= 20+modW+sizeW:
		return fit(name, w-modW-sizeW) + fit(mod, modW) + fmt.Sprintf("%*s", sizeW, size)
	default:
		return fit(name, w)
	}
}

// summary counts folders and files directly in the current dir and sums the file sizes.
func (m model) summary() string {
	dirs, files := 0, 0
	var total int64
	for _, e := range m.entries {
		if e.IsDir() {
			dirs++
		} else {
			files++
			total += m.size(e.Name())
		}
	}
	return fmt.Sprintf("%d folders · %d files · %s", dirs, files, humanSize(total))
}

type sizeMsg struct {
	path        string
	dirs, files int
	bytes       int64
}

// dirSize walks path recursively in the background; unreadable entries are skipped.
func dirSize(path string) tea.Cmd {
	return func() tea.Msg {
		msg := sizeMsg{path: path}
		filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if err != nil || p == path {
				return nil
			}
			if d.IsDir() {
				msg.dirs++
				return nil
			}
			msg.files++
			if fi, err := d.Info(); err == nil {
				msg.bytes += fi.Size()
			}
			return nil
		})
		return msg
	}
}

func (m model) visible() []os.DirEntry {
	if m.filter == "" {
		return m.entries
	}
	var out []os.DirEntry
	for _, e := range m.entries {
		if fuzzy(m.filter, e.Name()) {
			out = append(out, e)
		}
	}
	return out
}

func (m model) selected() os.DirEntry {
	v := m.visible()
	if m.cursor < 0 || m.cursor >= len(v) {
		return nil
	}
	return v[m.cursor]
}

func readPreview(path string, isDir bool) string {
	if isDir {
		entries, err := os.ReadDir(path)
		if err != nil {
			return err.Error()
		}
		var b strings.Builder
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() {
				name += "/"
			}
			b.WriteString(name)
			b.WriteByte('\n')
		}
		return b.String()
	}
	f, err := os.Open(path)
	if err != nil {
		return err.Error()
	}
	defer f.Close()
	buf, _ := io.ReadAll(io.LimitReader(f, 64*1024))
	if bytes.IndexByte(buf, 0) >= 0 {
		return "(binary file)"
	}
	return strings.ReplaceAll(strings.ReplaceAll(string(buf), "\r", ""), "\t", "    ")
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sizeMsg:
		if e := m.selected(); e != nil && filepath.Join(m.dir, e.Name()) == msg.path {
			m.info = fmt.Sprintf("%d folders · %d files · %s total", msg.dirs, msg.files, humanSize(msg.bytes))
		}
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyMsg:
		if m.focusR {
			page := max(m.previewH(), 1)
			switch msg.Type {
			case tea.KeyUp:
				m.scroll--
			case tea.KeyDown:
				m.scroll++
			case tea.KeyPgUp:
				m.scroll -= page
			case tea.KeyPgDown:
				m.scroll += page
			case tea.KeyHome:
				m.scroll = 0
			case tea.KeyEnd:
				m.scroll = m.maxScroll()
			case tea.KeyTab, tea.KeyShiftTab, tea.KeyEsc:
				m.focusR = false
			case tea.KeyCtrlC:
				return m, tea.Quit
			}
			m.scroll = min(max(m.scroll, 0), m.maxScroll())
			return m, nil
		}
		switch msg.Type {
		case tea.KeyTab:
			m.focusR = true
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc:
			if m.filter == "" {
				return m, tea.Quit
			}
			m.filter, m.cursor = "", 0
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(m.visible())-1 {
				m.cursor++
			}
		case tea.KeyLeft:
			parent := filepath.Dir(m.dir)
			if parent != m.dir {
				m.load(parent, filepath.Base(m.dir))
			}
		case tea.KeyRight:
			if e := m.selected(); e != nil && e.IsDir() {
				m.load(filepath.Join(m.dir, e.Name()), "")
			}
		case tea.KeyEnter:
			if e := m.selected(); e != nil {
				path := filepath.Join(m.dir, e.Name())
				m.preview, m.scroll = readPreview(path, e.IsDir()), 0
				if e.IsDir() {
					m.info = "calculating size…"
					return m, dirSize(path)
				}
				m.info = fmt.Sprintf("%s · %s · modified %s", fileType(e), humanSize(m.size(e.Name())), m.modified(e.Name()))
			}
		case tea.KeyBackspace:
			if m.filter != "" {
				r := []rune(m.filter)
				m.filter, m.cursor = string(r[:len(r)-1]), 0
			}
		case tea.KeyRunes, tea.KeySpace:
			m.filter += string(msg.Runes)
			m.cursor = 0
		}
	}
	return m, nil
}

// previewH is how many preview lines fit: screen minus header, footer, borders and the info line.
func (m model) previewH() int { return m.h - 5 }

func (m model) maxScroll() int {
	return max(strings.Count(m.preview, "\n")+1-m.previewH(), 0)
}

// clip cuts text to at most h lines of at most w runes each.
func clip(text string, w, h int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		if r := []rune(l); len(r) > w {
			lines[i] = string(r[:w])
		}
	}
	return strings.Join(lines, "\n")
}

func (m model) View() string {
	if m.w == 0 {
		return ""
	}
	innerH := m.h - 4   // 2 border rows + header + footer
	listH := innerH - 1 // column header row
	leftW := m.w*3/5 - 2
	rightW := m.w - leftW - 4
	if listH < 1 || leftW < 1 || rightW < 1 {
		return "terminal too small"
	}

	v := m.visible()
	start := 0
	if m.cursor >= listH {
		start = m.cursor - listH + 1
	}
	rows := []string{dimStyle.Render(row(leftW, "Name", "Date modified", "Type", "Size"))}
	for i := start; i < len(v) && i < start+listH; i++ {
		name := v[i].Name()
		size := ""
		if v[i].IsDir() {
			name += "/"
		} else {
			size = humanSize(m.size(v[i].Name()))
		}
		name = row(leftW, name, m.modified(v[i].Name()), fileType(v[i]), size)
		switch {
		case i == m.cursor:
			name = selStyle.Render(name)
		case v[i].IsDir():
			name = dirStyle.Render(name)
		}
		rows = append(rows, name)
	}

	leftStyle, rightStyle := paneStyle, paneStyle
	if m.focusR {
		rightStyle = paneStyle.BorderForeground(lipgloss.Color("12"))
	} else {
		leftStyle = paneStyle.BorderForeground(lipgloss.Color("12"))
	}
	body := m.preview
	if lines := strings.Split(m.preview, "\n"); m.scroll < len(lines) {
		body = strings.Join(lines[m.scroll:], "\n")
	}
	left := leftStyle.Width(leftW).Height(innerH).Render(strings.Join(rows, "\n"))
	previewText := clip(body, rightW, innerH)
	if m.info != "" {
		previewText = dimStyle.Render(clip(m.info, rightW, 1)) + "\n" + clip(body, rightW, innerH-1)
	}
	right := rightStyle.Width(rightW).Height(innerH).Render(previewText)

	header := clip(m.dir, max(m.w-lipgloss.Width(m.summary())-2, 1), 1) + "  " + dimStyle.Render(m.summary())
	footer := "> " + m.filter
	if m.err != nil {
		footer = m.err.Error()
	}
	help := dimStyle.Render("  ↑↓ move · ← back · → open · enter preview · tab scroll preview · type to filter · esc quit")
	return header + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, right) + "\n" + clip(footer+help, m.w, 1)
}

var version = "dev" // set by -ldflags at release build

func main() {
	start := "."
	if len(os.Args) > 1 {
		start = os.Args[1]
	}
	if start == "version" || start == "--version" {
		fmt.Println(version)
		return
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	m := model{}
	m.load(abs, "")
	if m.err != nil {
		fmt.Fprintln(os.Stderr, m.err)
		os.Exit(1)
	}
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
