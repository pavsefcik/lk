# lk

A TUI bookmark manager — save folders, files, and URLs, then find and open them instantly with fuzzy search.

**Requirements:** macOS, [Go](https://go.dev) 1.24+

## Install

1. Clone this repo anywhere you like:
   ```zsh
   git clone https://github.com/pavsefcik/lk
   ```
2. Build the binary:
   ```zsh
   cd lk && go build -o lk .
   ```
3. Put the binary on your `$PATH`, or just call it by its full path.

## Usage

| Key       | Action                                  |
| --------- | --------------------------------------- |
| `↑ / ↓`   | Move selection                          |
| `enter`   | Open the highlighted bookmark           |
| `^e`      | Edit the highlighted bookmark           |
| `^d`      | Enter delete mode (multi-select)        |
| `^o`      | Enter multi-open mode                   |
| `space`   | Toggle mark (in delete / multi modes)   |
| `enter`   | Confirm delete / open all marked        |
| `esc`     | Back out of mode, or back to main menu  |
| `^b`      | Toggle solid black background           |

- Files are revealed in Finder.
- Folders open in Finder.
- URLs open in your default browser.

## Data

Bookmarks are stored at `~/.local/share/lk/lk_data.json`. See [lk_data_example.json](lk_data_example.json) for the format — a JSON array of `{path, title, description}` objects.
