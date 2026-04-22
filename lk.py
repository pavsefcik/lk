#!/usr/bin/env python3
"""
LK // bookmark folders, files, and URLs

Usage:
  lk                     Save current Finder path or search bookmarks
  lk /some/path          Save a folder or file
  lk https://example.com Save a URL
  lk something           Search bookmarks and open selected result

Behavior:
  - No args: grabs the Finder selection/window path, offers to save it
  - Files are revealed in Finder
  - Folders are opened in Finder
  - URLs are opened in the default browser
"""

import json
import os
import subprocess
import sys
from difflib import SequenceMatcher
from pathlib import Path
from urllib.parse import unquote, urlparse

from textual.app import App, ComposeResult
from textual.binding import Binding
from textual.containers import Vertical, VerticalScroll
from textual.message import Message
from textual.reactive import reactive
from textual.screen import Screen
from textual.widgets import Input, Static

DATA_FILE = Path.home() / ".local" / "share" / "lk" / "lk_data.json"


# --- Data helpers ---

def load():
    if not DATA_FILE.exists():
        return []
    try:
        with open(DATA_FILE) as f:
            return json.load(f)
    except json.JSONDecodeError as e:
        print(
            f"Error: {DATA_FILE} is not valid JSON ({e.msg} at line {e.lineno}, col {e.colno}).\n"
            f"Fix it with: lk -f",
            file=sys.stderr,
        )
        sys.exit(1)


def persist(entries):
    DATA_FILE.parent.mkdir(parents=True, exist_ok=True)
    with open(DATA_FILE, "w") as f:
        json.dump(entries, f, indent=2, ensure_ascii=False)


def normalize_path(path_str):
    if path_str.startswith("smb://"):
        parsed = urlparse(path_str)
        return "/Volumes" + unquote(parsed.path)
    if path_str.startswith("file://"):
        return unquote(urlparse(path_str).path)
    return os.path.expanduser(path_str)


def resolve_stored(path_str):
    if path_str.startswith(("http://", "https://")):
        return path_str
    normalized = normalize_path(path_str)
    # .absolute() preserves symlinks; .resolve() would canonicalize them away
    return str(Path(normalized).absolute())


def word_matches(word, haystack):
    if word in haystack:
        return True
    return any(
        SequenceMatcher(None, word, hw).ratio() >= 0.8 for hw in haystack.split()
    )


def filter_entries(query, entries, search_texts=None):
    if not query.strip():
        return entries
    words = query.lower().split()
    return [
        e
        for i, e in enumerate(entries)
        if all(
            word_matches(w, search_texts[i] if search_texts else f"{e['title']} {e.get('description', '')} {e['path']}".lower())
            for w in words
        )
    ]


def build_search_texts(entries):
    return [
        f"{e['title']} {e.get('description', '')} {e['path']}".lower()
        for e in entries
    ]


def open_path(path):
    if path.startswith(("http://", "https://")):
        subprocess.run(["/usr/bin/open", path])
    elif os.path.isfile(path):
        subprocess.run(["osascript", "-e", """
            on run argv
                set thePath to POSIX file (item 1 of argv) as alias
                tell application "Finder"
                    activate
                    set theFolder to container of thePath
                    if (count of Finder windows) > 0 then
                        tell application "System Events" to keystroke "t" using command down
                        delay 0.3
                        set target of front Finder window to theFolder
                        delay 0.1
                        select thePath
                    else
                        reveal thePath
                    end if
                end tell
            end run
        """, path])
    elif os.path.isdir(path):
        subprocess.run(["osascript", "-e", """
            on run argv
                set thePath to POSIX file (item 1 of argv)
                tell application "Finder"
                    activate
                    if (count of Finder windows) > 0 then
                        tell application "System Events" to keystroke "t" using command down
                        delay 0.3
                        set target of front Finder window to thePath
                    else
                        open thePath
                    end if
                end tell
            end run
        """, path])
    else:
        subprocess.run(["/usr/bin/open", path])


def get_finder_path():
    try:
        result = subprocess.run(
            ["osascript", "-e", """
                tell app "Finder"
                    set sel to selection
                    if sel is {} then
                        POSIX path of (target of front window as alias)
                    else
                        POSIX path of (item 1 of sel as alias)
                    end if
                end tell
            """],
            capture_output=True, text=True, timeout=3,
        )
        path = result.stdout.strip()
        return path if path else None
    except Exception:
        return None


# --- TUI widgets ---

class SearchInput(Input):
    """Input that forwards arrow keys and enter to the containing screen."""

    class Navigate(Message):
        def __init__(self, direction: int):
            super().__init__()
            self.direction = direction

    class Submit(Message):
        pass

    class EditBookmark(Message):
        pass

    class DeleteBookmark(Message):
        pass

    class MultiPick(Message):
        pass

    def _on_key(self, event):
        if event.key == "down":
            event.prevent_default(); event.stop()
            self.post_message(self.Navigate(1))
        elif event.key == "up":
            event.prevent_default(); event.stop()
            self.post_message(self.Navigate(-1))
        elif event.key == "enter":
            event.prevent_default(); event.stop()
            self.post_message(self.Submit())
        elif event.key == "ctrl+e":
            event.prevent_default(); event.stop()
            self.post_message(self.EditBookmark())
        elif event.key == "ctrl+d":
            event.prevent_default(); event.stop()
            self.post_message(self.DeleteBookmark())
        elif event.key == "ctrl+o":
            event.prevent_default(); event.stop()
            self.post_message(self.MultiPick())
        else:
            super()._on_key(event)


class BookmarkItem(Static):
    DEFAULT_CSS = """
    BookmarkItem {
        padding: 0 2;
        height: auto;
        margin-bottom: 1;
    }
    BookmarkItem:hover {
        background: $surface-lighten-1;
    }
    BookmarkItem.selected {
        background: $accent;
        color: $text;
    }
    """

    def __init__(self, entry, index, mark=None, mode=None):
        self.entry = entry
        self.index = index
        self.mark = mark
        self.mode = mode
        super().__init__(self._render_text(), markup=True)

    def _render_text(self):
        title = self.entry["title"]
        desc = self.entry.get("description", "")
        path = self.entry["path"]
        if self.mark is not None:
            if self.mark:
                indicator = "[bold red]✕[/bold red]" if self.mode == "delete" else "[bold green]✓[/bold green]"
            else:
                indicator = "[dim]·[/dim]"
            lines = f" {indicator}  [bold]{title}[/bold]"
        else:
            lines = f"[bold]{self.index + 1}[/bold]  [bold]{title}[/bold]"
        if desc:
            lines += f"\n    [dim]{desc}[/dim]"
        lines += f"\n    [dim italic]{path}[/dim italic]"
        return lines

    def set_mark(self, mark):
        self.mark = mark
        self.update(self._render_text())


class MenuItem(Static):
    DEFAULT_CSS = """
    MenuItem {
        padding: 0 2;
        height: 1;
    }
    MenuItem:hover {
        background: $surface-lighten-1;
    }
    MenuItem.selected {
        background: $accent;
        color: $text;
    }
    """

    def __init__(self, label, value):
        super().__init__(label, markup=True)
        self.value = value


# --- Screens ---

class SaveScreen(Screen):
    DEFAULT_CSS = """
    SaveScreen { background: $background; }
    SaveScreen #header { margin: 1 2; height: auto; }
    SaveScreen #form { margin: 1 2; height: auto; }
    SaveScreen #form Input { margin-bottom: 1; }
    SaveScreen #status { dock: bottom; height: 1; padding: 0 2; color: $text-muted; }
    """

    BINDINGS = [Binding("escape", "cancel", show=False)]

    def __init__(self, stored_path="", existing=None):
        super().__init__()
        self.stored_path = stored_path
        self.existing = existing
        self._show_path = bool(existing) or not stored_path

    def compose(self) -> ComposeResult:
        if self.existing:
            yield Static(f"[bold yellow]Editing bookmark[/bold yellow]", id="header", markup=True)
        elif self.stored_path:
            yield Static(f"[bold]Saving:[/bold] [dim]{self.stored_path}[/dim]", id="header", markup=True)
        else:
            yield Static(f"[bold]New bookmark[/bold]", id="header", markup=True)
        with Vertical(id="form"):
            if self._show_path:
                yield Input(
                    placeholder="Path / URL (required)",
                    value=self.stored_path,
                    id="path",
                )
            yield Input(
                placeholder="Title (required)",
                value=self.existing["title"] if self.existing else "",
                id="title",
            )
            yield Input(
                placeholder="Description (optional)",
                value=self.existing.get("description", "") if self.existing else "",
                id="description",
            )
        yield Static("  enter confirm   esc cancel", id="status")

    def on_mount(self):
        if self._show_path:
            self.query_one("#path", Input).focus()
        else:
            self.query_one("#title", Input).focus()

    def on_input_submitted(self, event: Input.Submitted):
        if event.input.id == "path":
            if event.value.strip():
                self.query_one("#title", Input).focus()
        elif event.input.id == "title":
            if event.value.strip():
                self.query_one("#description", Input).focus()
        elif event.input.id == "description":
            title = self.query_one("#title", Input).value.strip()
            if not title:
                self.query_one("#title", Input).focus()
                return
            if self._show_path:
                path = self.query_one("#path", Input).value.strip()
                if not path:
                    self.query_one("#path", Input).focus()
                    return
            else:
                path = self.stored_path
            self._commit(path=path, title=title, description=event.value.strip())

    def _commit(self, path, title, description):
        stored = resolve_stored(path) if path else ""
        entries = load()
        # If editing, locate the original entry by its prior path; else dedupe by new path
        old_path = self.existing["path"] if self.existing else stored
        target = next((e for e in entries if e["path"] == old_path), None)
        if target is None and self.existing is None:
            target = next((e for e in entries if e["path"] == stored), None)
        if target is None:
            entries.append({"path": stored, "title": title, "description": description})
            action = "saved"
        else:
            target["path"] = stored
            target["title"] = title
            target["description"] = description
            action = "updated"
        persist(entries)
        self.dismiss((action, title))

    def action_cancel(self):
        self.dismiss(None)


class HelpScreen(Screen):
    DEFAULT_CSS = """
    HelpScreen { background: $background; }
    HelpScreen #help { margin: 1 2; height: auto; }
    HelpScreen #status { dock: bottom; height: 1; padding: 0 2; color: $text-muted; }
    """

    BINDINGS = [
        Binding("escape", "close", show=False),
        Binding("enter", "close", show=False),
    ]

    def compose(self) -> ComposeResult:
        yield Static(HELP_TEXT, id="help")
        yield Static("  enter/esc close", id="status")

    def action_close(self):
        self.dismiss()


HELP_TEXT = __doc__.strip()


class SearchScreen(Screen):
    DEFAULT_CSS = """
    SearchScreen { background: $background; }
    SearchScreen #search { dock: top; margin: 1 2 0 2; }
    SearchScreen #results { margin: 1 0; }
    SearchScreen #status { dock: bottom; height: 1; padding: 0 2; color: $text-muted; }
    """

    BINDINGS = [Binding("escape", "escape", show=False)]

    MAX_VISIBLE = 50

    selected_index = reactive(0)

    def __init__(self, initial_query=""):
        super().__init__()
        self.all_entries = load()
        self.initial_query = initial_query
        self._search_texts = build_search_texts(self.all_entries)
        self.matches = filter_entries(initial_query, self.all_entries, self._search_texts)
        self._last_query = initial_query.strip()
        self._filter_timer = None
        self._pending_query = self._last_query
        # modal mode state: "delete", "multi", or None
        self._mode = None
        self._marked = set()
        self._confirming = False

    def compose(self) -> ComposeResult:
        yield SearchInput(placeholder="Search bookmarks...", value=self.initial_query, id="search")
        yield VerticalScroll(id="results")
        yield Static("", id="status")

    def on_mount(self):
        if not self.all_entries:
            self.app.notify("No bookmarks yet. Add one from the main menu.", severity="warning")
            self.call_after_refresh(self.dismiss)
            return
        self.query_one("#search", SearchInput).focus()
        self._refresh_results()

    def on_key(self, event):
        if not self._mode:
            return
        key = event.key
        event.stop()
        event.prevent_default()
        n = min(len(self.matches), self.MAX_VISIBLE)
        if key == "down" and n:
            self.selected_index = (self.selected_index + 1) % n
        elif key == "up" and n:
            self.selected_index = (self.selected_index - 1) % n
        elif key == "space" and not self._confirming and n:
            idx = self.selected_index
            if idx in self._marked:
                self._marked.discard(idx)
            else:
                self._marked.add(idx)
            items = self.query("BookmarkItem")
            if 0 <= idx < len(items):
                items[idx].set_mark(idx in self._marked)
            self._update_status()
        elif key == "enter" and self._marked:
            if self._mode == "delete":
                if not self._confirming:
                    self._confirming = True
                    self._update_status()
                else:
                    self._do_delete()
            elif self._mode == "multi":
                self._do_multi_open()
        elif key == "escape":
            if self._confirming:
                self._confirming = False
                self._update_status()
            else:
                self._exit_mode()

    def on_search_input_navigate(self, event: SearchInput.Navigate):
        if self.matches:
            self.selected_index = (self.selected_index + event.direction) % len(self.matches)

    def on_search_input_submit(self, event: SearchInput.Submit):
        if self.matches and 0 <= self.selected_index < len(self.matches):
            open_path(self.matches[self.selected_index]["path"])

    def on_search_input_edit_bookmark(self, event: SearchInput.EditBookmark):
        if self.matches and 0 <= self.selected_index < len(self.matches):
            entry = self.matches[self.selected_index]
            self.app.push_screen(SaveScreen(entry["path"], existing=entry), self._after_edit)

    def on_search_input_delete_bookmark(self, event: SearchInput.DeleteBookmark):
        if self.matches:
            self._enter_mode("delete")

    def on_search_input_multi_pick(self, event: SearchInput.MultiPick):
        if self.matches:
            self._enter_mode("multi")

    def _after_edit(self, result):
        self.all_entries = load()
        self._search_texts = build_search_texts(self.all_entries)
        q = self.query_one("#search", SearchInput).value.strip()
        self.matches = filter_entries(q, self.all_entries, self._search_texts)
        self._last_query = q
        self.selected_index = 0
        self._refresh_results()

    def on_input_changed(self, event: Input.Changed):
        if self._filter_timer is not None:
            self._filter_timer.stop()
        self._pending_query = event.value.strip()
        self._filter_timer = self.set_timer(0.15, self._do_filter)

    def _do_filter(self):
        q = self._pending_query
        if q and self._last_query and q.startswith(self._last_query):
            source = self.matches
            texts = build_search_texts(source)
        else:
            source = self.all_entries
            texts = self._search_texts
        self.matches = filter_entries(q, source, texts)
        self._last_query = q
        self.selected_index = 0
        self._refresh_results()

    def _enter_mode(self, mode):
        self._mode = mode
        self._marked = set()
        self._confirming = False
        self.query_one("#search", SearchInput).disabled = True
        self._refresh_results()

    def _exit_mode(self):
        self._mode = None
        self._marked = set()
        self._confirming = False
        search = self.query_one("#search", SearchInput)
        search.disabled = False
        search.focus()
        self._refresh_results()

    def _do_delete(self):
        paths_to_delete = {self.matches[i]["path"] for i in self._marked}
        self.all_entries = [e for e in self.all_entries if e["path"] not in paths_to_delete]
        persist(self.all_entries)
        self._search_texts = build_search_texts(self.all_entries)
        q = self.query_one("#search", SearchInput).value.strip()
        self.matches = filter_entries(q, self.all_entries, self._search_texts)
        self._last_query = q
        self.selected_index = 0
        self._exit_mode()

    def _do_multi_open(self):
        for i in sorted(self._marked):
            open_path(self.matches[i]["path"])
        self._exit_mode()

    def watch_selected_index(self, value):
        self._highlight()

    def _refresh_results(self):
        container = self.query_one("#results", VerticalScroll)
        container.remove_children()
        visible = self.matches[:self.MAX_VISIBLE]
        for i, entry in enumerate(visible):
            mark = (i in self._marked) if self._mode else None
            item = BookmarkItem(entry, i, mark=mark, mode=self._mode)
            if i == self.selected_index:
                item.add_class("selected")
            container.mount(item)
        self.call_after_refresh(self._highlight)
        self._update_status()

    def _update_status(self):
        status = self.query_one("#status", Static)
        matched = len(self.matches)
        total = len(self.all_entries)
        if self._mode == "delete":
            n = len(self._marked)
            if self._confirming:
                status.update(f" [bold red]Delete {n} bookmark{'s' if n != 1 else ''}?[/bold red]   enter confirm   esc cancel")
            else:
                sel = f"  [bold]{n} selected[/bold]" if n else ""
                status.update(f" ↑↓ navigate   space toggle   enter delete{sel}   esc back")
        elif self._mode == "multi":
            n = len(self._marked)
            sel = f"  [bold]{n} selected[/bold]" if n else ""
            status.update(f" ↑↓ navigate   space toggle   enter open all{sel}   esc back")
        else:
            if matched > self.MAX_VISIBLE:
                status.update(f" showing {self.MAX_VISIBLE} of {matched} matches ({total} total)   ↑↓ enter open   ^e edit   ^d delete   ^o multi   esc back")
            else:
                status.update(f" {matched}/{total} bookmarks   ↑↓ enter open   ^e edit   ^d delete   ^o multi   esc back")

    def _highlight(self):
        items = self.query("BookmarkItem")
        for i, item in enumerate(items):
            if i == self.selected_index:
                item.add_class("selected")
                item.scroll_visible()
            else:
                item.remove_class("selected")

    def action_escape(self):
        if self._mode:
            self._exit_mode()
        else:
            self.dismiss()


class ChooserScreen(Screen):
    DEFAULT_CSS = """
    ChooserScreen { background: $background; }
    ChooserScreen #header { margin: 1 2; height: auto; }
    ChooserScreen #menu { margin: 1 2; height: auto; }
    ChooserScreen #status { dock: bottom; height: 1; padding: 0 2; color: $text-muted; }
    """

    BINDINGS = [
        Binding("escape", "quit_app", show=False),
        Binding("down", "move_down", show=False),
        Binding("up", "move_up", show=False),
        Binding("enter", "select", show=False),
    ]

    DEFAULT_STATUS = "  ↑↓ navigate   enter select   esc quit"

    selected_index = reactive(0)

    def __init__(self, auto_open=None):
        super().__init__()
        self.finder_path = get_finder_path()
        self.options = []
        self._build_options()
        self._status_timer = None
        self._auto_open = auto_open  # ("save", path) | ("search", query) | None

    def _build_options(self):
        save_label = "  [bold]Save this path[/bold]" if self.finder_path else "  [bold]Add bookmark[/bold]"
        self.options = [
            (save_label, "save"),
            ("  [bold]Search bookmarks[/bold]", "search"),
            ("  [dim]Reload Finder path[/dim]", "reload"),
            ("  [dim]Open data folder[/dim]", "data_folder"),
            ("  [dim]Help[/dim]", "help"),
            ("  [dim]Exit[/dim]", "exit"),
        ]

    def _header_markup(self):
        if self.finder_path:
            return f"\n  [bold]Finder path:[/bold] [dim]{self.finder_path}[/dim]\n"
        return f"\n  [dim]No Finder path selected[/dim]\n"

    def compose(self) -> ComposeResult:
        yield Static(self._header_markup(), id="header", markup=True)
        with Vertical(id="menu"):
            for label, value in self.options:
                yield MenuItem(label, value)
        yield Static(self.DEFAULT_STATUS, id="status")

    def on_mount(self):
        self._highlight()
        if self._auto_open:
            kind, arg = self._auto_open
            self._auto_open = None
            if kind == "save":
                self._push_save(arg)
            elif kind == "search":
                self._push_search(arg)

    def watch_selected_index(self, value):
        self._highlight()

    def _highlight(self):
        items = self.query("MenuItem")
        for i, item in enumerate(items):
            if i == self.selected_index:
                item.add_class("selected")
            else:
                item.remove_class("selected")

    def action_move_down(self):
        self.selected_index = (self.selected_index + 1) % len(self.options)

    def action_move_up(self):
        self.selected_index = (self.selected_index - 1) % len(self.options)

    def action_select(self):
        value = self.options[self.selected_index][1]
        if value == "reload":
            self._reload_finder()
        elif value == "data_folder":
            self._open_data_folder()
        elif value == "save":
            self._push_save(self.finder_path or "")
        elif value == "search":
            self._push_search()
        elif value == "help":
            self.app.push_screen(HelpScreen())
        elif value == "exit":
            self.app.exit()

    def action_quit_app(self):
        self.app.exit()

    def _push_save(self, path_or_url):
        stored = resolve_stored(path_or_url) if path_or_url else ""
        entries = load()
        existing = next((e for e in entries if e["path"] == stored), None) if stored else None
        self.app.push_screen(SaveScreen(stored, existing=existing), self._after_save)

    def _push_search(self, query=""):
        self.app.push_screen(SearchScreen(initial_query=query))

    def _after_save(self, result):
        if result:
            action, title = result
            self._flash_status(f"[bold green]{action.capitalize()}:[/bold green] {title}")

    def _reload_finder(self):
        old = self.finder_path
        self.finder_path = get_finder_path()
        self.query_one("#header", Static).update(self._header_markup())
        self._build_options()
        menu = self.query_one("#menu", Vertical)
        menu.remove_children()
        for label, value in self.options:
            menu.mount(MenuItem(label, value))
        self._highlight()
        if self.finder_path and self.finder_path != old:
            self._flash_status(f"[bold green]Reloaded[/bold green] → {self.finder_path}")
        elif self.finder_path and self.finder_path == old:
            self._flash_status("[bold green]Reloaded[/bold green] (no change)")
        elif old and not self.finder_path:
            self._flash_status("[bold yellow]Reloaded[/bold yellow] (no Finder path)")
        else:
            self._flash_status("[bold yellow]Reloaded[/bold yellow] (no Finder path)")

    def _flash_status(self, markup, duration=1.8):
        status = self.query_one("#status", Static)
        status.update(f"  {markup}")
        if self._status_timer is not None:
            self._status_timer.stop()
        self._status_timer = self.set_timer(duration, self._restore_status)

    def _restore_status(self):
        self.query_one("#status", Static).update(self.DEFAULT_STATUS)
        self._status_timer = None

    def _open_data_folder(self):
        DATA_FILE.parent.mkdir(parents=True, exist_ok=True)
        subprocess.run(["/usr/bin/open", str(DATA_FILE.parent)])
        self._flash_status(f"[bold green]Opened[/bold green] {DATA_FILE.parent}")


# --- App ---

class LkApp(App):
    TITLE = "lk"

    def __init__(self, initial_screens):
        super().__init__()
        self._initial_screens = list(initial_screens)

    def on_mount(self):
        for screen in self._initial_screens:
            self.push_screen(screen)


# --- Non-TUI commands ---

def cmd_data():
    if DATA_FILE.exists():
        subprocess.run(["/usr/bin/open", "-R", str(DATA_FILE)])
    else:
        print("No data file found.", file=sys.stderr)


def cmd_edit():
    subprocess.run(["/usr/bin/open", "-a", "TextEdit", str(DATA_FILE)])


def cmd_form():
    editor = os.environ.get("EDITOR", "nano")
    subprocess.run([editor, str(DATA_FILE)])


# --- Main ---

def main():
    args = sys.argv[1:]

    if not args:
        LkApp([ChooserScreen()]).run()
        return

    input_str = " ".join(args)

    if input_str in ("-h", "--help"):
        LkApp([HelpScreen()]).run()
    elif input_str in ("-d", "--data"):
        cmd_data()
    elif input_str in ("-e", "--edit"):
        cmd_edit()
    elif input_str in ("-f", "--form"):
        cmd_form()
    elif input_str.startswith(("http://", "https://", "smb://", "/", "./", "../", "~")):
        LkApp([ChooserScreen(auto_open=("save", input_str))]).run()
    else:
        LkApp([ChooserScreen(auto_open=("search", input_str))]).run()


if __name__ == "__main__":
    main()
