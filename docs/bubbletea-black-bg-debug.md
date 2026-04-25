# Adding a togglable black background to a Bubble Tea TUI — debug log

A walkthrough of the bugs we hit adding `ctrl+b` to flip a black background on/off,
why each fix attempt failed, and what finally worked. Useful next time we touch
lipgloss / bubbletea / cellbuf rendering.

## The setup

- bubbletea v1.3.10 in alt-screen mode (`tea.WithAltScreen()`)
- lipgloss v1.1.0 (uses `charmbracelet/x/cellbuf` under the hood)
- The toggle was supposed to wrap the View output in a `Background(black)` style.

## What we kept seeing

Two distinct artefacts, in this order:

1. **Bleed-on (bg = ON)** — slightly-lighter rectangles around every styled
   span (the `lk` mark, dim subtitle, finder path, result titles…). The
   "black" was inconsistent across the screen.
2. **Residue-off (bg = OFF)** — after toggling off, a black strip stayed at
   the top of the screen (and any cell that didn't change content).

## Root causes (in order of discovery)

### 1. Outer wrap doesn't reach inner cells

`styleBlackBg.Width(w).Height(h).Render(out)` only fills the *padding* it
adds around `out`. Inside `out`, every styled span emits its own SGR
sequence ending in `\x1b[0m`, which resets bg to terminal default. The
outer wrap can't repaint cells that already received a "reset bg" escape
inside the content.

### 2. `Faint(true)` muddies the bg shade

After we baked `Background(black)` into every base style, the dim text
*still* looked off. `Faint` (SGR 2) is rendered by many terminals as
"reduce fg/bg intensity" — so cells with `Faint+bg=#000` came out as a
slightly lighter black than cells with just `bg=#000`. Replacing
`Faint(true)` with an explicit `Foreground("#8a8a8a")` made every styled
cell render the same uniform black.

### 3. Plain ASCII spaces between styled spans

Code like `"  " + styleLogo.Render("lk") + " " + styleDim.Render("...")`
emits *raw* characters between styled segments. Those carry no SGR, so the
cellbuf assigns them whatever attrs were last set — which after a reset is
"default", showing terminal background. Fix: post-process the rendered
string with a regex that re-asserts the bg escape after every SGR:

```go
sgrRe.ReplaceAllStringFunc(s, func(m string) string { return m + blackBgEsc })
```

### 4. Basic vs truecolor black aren't the same colour

The first regex hack used `\x1b[40m` (basic ANSI black). On a truecolor
terminal that index can map to `#0c0c0c` (or whatever the user's palette
defines), which is *visibly different* from `\x1b[48;2;0;0;0m`
(`#000000`) emitted by lipgloss. Mixing them produced rectangles in
inverted shade. Switching the post-process to truecolor `\x1b[48;2;0;0;0m`
made everything match.

### 5. lipgloss `Height()` pads with `\n`, not space-cells

When toggling bg off, the new render had fewer "filled" cells than the
previous black frame. `lipgloss.NewStyle().Height(h).Render(s)` adds bare
newlines, which only move the cursor — they don't repaint cells. So
prior-frame black cells stayed black. Replacing the height pad with
literal `strings.Repeat(" ", w)` rows for each missing line gave us actual
characters to write into every cell.

### 6. Bubbletea's diff renderer skips unchanged-content cells

Even with literal spaces, cellbuf compares cell *content* and skips writes
when the char is unchanged — even if the SGR attrs differ. So switching
from "bg=black space" to "default-bg space" wasn't always re-emitted,
leaving the residue strip at the top of the screen. `tea.ClearScreen`
didn't help because it clears using the *current* SGR (still black bg).

The fix that finally worked: on toggle, exit and re-enter alt-screen.

```go
return m, tea.Sequence(tea.ExitAltScreen, tea.EnterAltScreen)
```

The terminal allocates a brand-new alt-screen buffer, blank by definition,
so the next render lays down on a clean canvas. There's a single-frame
flicker of the primary screen — acceptable for a manual toggle.

## What survived in the codebase

- `withBg(style)` in [styles.go](../styles.go) — bakes the bg into every base
  style at build time; rebuilt on toggle via `rebuildTheme()`.
- `styleInput()` for bubbles `textinput` — applies the same bg to prompt,
  text, placeholder, and cursor sub-styles.
- Replaced all `Faint(true)` usage with an explicit muted foreground.
- `forceBlackBg()` in [model.go](../model.go) — regex post-process that
  re-asserts truecolor `#000000` bg after every SGR escape, paving over
  raw spaces between styled spans.
- `padToScreen()` — pads to literal `w × h` cells with real spaces, so
  every cell gets explicitly written each frame.
- `tea.Sequence(tea.ExitAltScreen, tea.EnterAltScreen)` on toggle to defeat
  the diff renderer.

## Heuristics for next time

1. **Don't trust an outer Background to reach inner cells.** If you want
   uniform bg under styled content, bake `.Background(...)` into every
   base style at construction (the crush approach).
2. **`Faint(true)` is bg-leaky on many terminals.** Use an explicit muted
   fg colour instead when you need uniform bg.
3. **Match colour spaces.** If lipgloss emits truecolor escapes, your
   post-process patches must too — never mix basic `\x1b[40m` with
   `\x1b[48;2;0;0;0m`.
4. **Raw spaces between styled spans inherit prior SGR.** Either wrap the
   whole line in a styled span, or post-process to re-assert attrs after
   every reset.
5. **lipgloss `Height()` doesn't paint cells**, only adds newlines. If you
   need a full-screen repaint, fill rows with literal spaces yourself.
6. **Bubbletea's diff renderer can stick on unchanged-char cells.** When
   you need to invalidate everything, `tea.ClearScreen` may not be enough
   — `tea.Sequence(ExitAltScreen, EnterAltScreen)` is the bigger hammer.
