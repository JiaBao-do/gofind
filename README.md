# gofind

A small terminal file explorer, fzf-flavoured: a file list on the left, a preview
pane on the right, and type-to-filter fuzzy matching.

## Install

No Go toolchain needed.

**Windows (x64 / ARM64)**, in PowerShell:

```powershell
irm https://raw.githubusercontent.com/JiaBao-do/gofind/main/install.ps1 | iex
```

**Linux / macOS (amd64 / arm64)**:

```sh
curl -fsSL https://raw.githubusercontent.com/JiaBao-do/gofind/main/install.sh | sh
```

Open a new terminal and run `gofind` (or `gofind <dir>`).

Options: `GOFIND_VERSION=v0.1.0` (specific release), `GOFIND_INSTALL_DIR=...`,
`GOFIND_UNINSTALL=1`, `GOFIND_NO_MODIFY_PATH=1`.

With Go: `go install github.com/JiaBao-do/gofind@latest`

## Commands

```
gofind [dir]      browse dir (default: current folder)
gofind version    print the version
gofind upgrade    install the latest release
gofind help       show help
```

## Preview

Text and code as-is, Markdown styled, CSV/TSV and Excel (`.xlsx`, first sheet)
as aligned tables, Word (`.docx`) as text, PDF via `pdftotext` if installed
(poppler). Folders show their recursive size.

Zooming the terminal (Ctrl +/-) re-lays out the panes and re-wraps the preview.

## Keys

```
↑ ↓      move
←        go to parent folder
→        open folder
enter    preview file / folder in the right pane
tab      focus the preview; up/down/pgup/pgdn/home/end scroll it
type     fuzzy filter the list (backspace edits, esc clears)
esc      quit (when the filter is empty), ctrl+c always quits
```

## Release

Push a tag (`git tag v0.2.0 && git push --tags`); GitHub Actions builds every
platform and publishes the release the installers download from.
