package main

import (
	"bytes"
	"fmt"
	"io"
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
	filter  string
	cursor  int
	preview string
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
	m.dir, m.entries, m.filter, m.cursor, m.preview, m.err = dir, entries, "", 0, "", nil
	for i, e := range entries {
		if e.Name() == selectName {
			m.cursor = i
		}
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
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.Type {
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
				m.preview = readPreview(filepath.Join(m.dir, e.Name()), e.IsDir())
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
	innerH := m.h - 4 // 2 border rows + header + footer
	leftW := m.w/2 - 2
	rightW := m.w - leftW - 4
	if innerH < 1 || leftW < 1 || rightW < 1 {
		return "terminal too small"
	}

	v := m.visible()
	start := 0
	if m.cursor >= innerH {
		start = m.cursor - innerH + 1
	}
	var rows []string
	for i := start; i < len(v) && i < start+innerH; i++ {
		name := v[i].Name()
		if v[i].IsDir() {
			name += "/"
		}
		name = clip(name, leftW, 1)
		switch {
		case i == m.cursor:
			name = selStyle.Render(name)
		case v[i].IsDir():
			name = dirStyle.Render(name)
		}
		rows = append(rows, name)
	}

	left := paneStyle.Width(leftW).Height(innerH).Render(strings.Join(rows, "\n"))
	right := paneStyle.Width(rightW).Height(innerH).Render(clip(m.preview, rightW, innerH))

	header := clip(m.dir, m.w, 1)
	footer := "> " + m.filter
	if m.err != nil {
		footer = m.err.Error()
	}
	help := dimStyle.Render("  ↑↓ move · ← back · → open · enter preview · type to filter · esc quit")
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
