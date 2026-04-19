# lk

A TUI bookmark manager — save folders, files, and URLs, then find and open them instantly with fuzzy search.

**Requirements:** macOS, Python 3.11+, [uv](https://github.com/astral-sh/uv)

## Install

1. Clone this repo anywhere you like:
   ```zsh
   git clone https://github.com/pavsefcik/lk
   ```
2. Double-click `install.command` inside the cloned folder (or run `./install.command` from it).

The installer adds an `lk` alias to your `~/.zshrc` pointing at wherever you cloned it, and primes the virtualenv.

## Usage

```
lk                     Save current Finder path, or open the chooser
lk /some/path          Save a folder or file
lk https://example.com Save a URL
lk something           Search bookmarks and open the result
```

### Inside the search TUI

| Key       | Action                                  |
| --------- | --------------------------------------- |
| `↑ / ↓`   | Move selection                          |
| `enter`   | Open the highlighted bookmark           |
| `^e`      | Edit the highlighted bookmark           |
| `^d`      | Enter delete mode (multi-select)        |
| `^o`      | Enter multi-open mode                   |
| `space`   | Toggle mark (in delete / multi modes)   |
| `enter`   | Confirm delete / open all marked        |
| `esc`     | Back out of modal, or back to main menu |

- Files are revealed in Finder.
- Folders open in Finder.
- URLs open in your default browser.

## Data

Bookmarks are stored at `~/.local/share/lk/lk_data.json`. See [lk_data_example.json](lk_data_example.json) for the format — a JSON array of `{path, title, description}` objects.
