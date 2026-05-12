package main

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------- screen enum ----------

type screenID int

const (
	screenChooser screenID = iota
	screenSearch
	screenSave
	screenHelp
)

// ---------- messages ----------

type finderPathMsg struct {
	path    string
	manual  bool
}
type clearFlashMsg struct{ id int }
type clearReloadedMsg struct{ id int }
type reloadedFadeMsg struct {
	id      int
	visible int
}
type startSelectHintFadeMsg struct{ id int }
type selectHintFadeMsg struct {
	id      int
	visible int
}
type openDoneMsg struct{}

// ---------- root model ----------

type model struct {
	screen        screenID
	width, height int
	chooser       chooserModel
	search        searchModel
	save          saveModel
	// save needs to know where to return after editing
	saveReturnTo screenID
	blackBg      bool
}

func newModel(initial screenID, arg string) model {
	m := model{
		screen:  initial,
		chooser: newChooserModel(),
		search:  newSearchModel(""),
		save:    newSaveModel("", nil),
	}
	switch initial {
	case screenSearch:
		m.search = newSearchModel(arg)
	case screenSave:
		m.save = newSaveModel(arg, nil)
		m.saveReturnTo = screenChooser
	}
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		fetchFinderPathCmd,
		textinput.Blink,
	)
}

func fetchFinderPathCmd() tea.Msg {
	return finderPathMsg{path: getFinderPath()}
}

// scheduleFade schedules n ticks across `total` time using an ease-in-quad
// curve over the cumulative count — gentle initial pause, accelerating through
// to the last letter (no end hesitation). `makeMsg` is called with the visible
// count after each step (n-1, n-2, … 0).
func scheduleFade(n int, total time.Duration, makeMsg func(visible int) tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, 0, n)
	for i := 1; i <= n; i++ {
		y := float64(i) / float64(n)
		t := math.Sqrt(y) // inverse of easeInQuad
		delay := time.Duration(t * float64(total))
		visible := n - i
		cmds = append(cmds, tea.Tick(delay, func(time.Time) tea.Msg {
			return makeMsg(visible)
		}))
	}
	return tea.Batch(cmds...)
}

func fadeReloadedCmd(id, n int, total time.Duration) tea.Cmd {
	return scheduleFade(n, total, func(v int) tea.Msg {
		return reloadedFadeMsg{id: id, visible: v}
	})
}

func fadeSelectHintCmd(id, n int, total time.Duration) tea.Cmd {
	return scheduleFade(n, total, func(v int) tea.Msg {
		return selectHintFadeMsg{id: id, visible: v}
	})
}

// reloadFinderPathCmd adds a small artificial delay so the spinner is visible
// — pure UX, since getFinderPath() is fast.
func reloadFinderPathCmd() tea.Msg {
	path := getFinderPath()
	time.Sleep(700 * time.Millisecond)
	return finderPathMsg{path: path, manual: true}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.search.viewport.Width = m.width
		m.search.viewport.Height = m.height - 4
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if msg.String() == "ctrl+b" {
			m.blackBg = !m.blackBg
			blackBgOn = m.blackBg
			rebuildTheme()
			styleInput(&m.search.input)
			for i := range m.save.inputs {
				styleInput(&m.save.inputs[i])
			}
			if m.screen == screenSearch {
				m.search.viewport.SetContent(renderSearchResults(m.search, m.width))
			}
			return m, tea.Sequence(tea.ExitAltScreen, tea.EnterAltScreen)
		}
	case finderPathMsg:
		m.chooser.finderPath = msg.path
		m.chooser.reloading = false
		if msg.manual {
			m.chooser.reloaded = true
			m.chooser.reloadedVisible = len("Reloaded")
			m.chooser.reloadedID++
			id := m.chooser.reloadedID
			return m, tea.Tick(1*time.Second, func(time.Time) tea.Msg {
				return clearReloadedMsg{id}
			})
		}
		return m, nil
	case clearReloadedMsg:
		if msg.id == m.chooser.reloadedID {
			return m, fadeReloadedCmd(msg.id, len("Reloaded"), 700*time.Millisecond)
		}
		return m, nil
	case reloadedFadeMsg:
		if msg.id == m.chooser.reloadedID {
			m.chooser.reloadedVisible = msg.visible
			if msg.visible <= 0 {
				m.chooser.reloaded = false
			}
		}
		return m, nil
	case startSelectHintFadeMsg:
		if msg.id == m.chooser.selectHintID {
			return m, fadeSelectHintCmd(msg.id, len(selectHintWord), 700*time.Millisecond)
		}
		return m, nil
	case selectHintFadeMsg:
		if msg.id == m.chooser.selectHintID {
			m.chooser.selectHintVisible = msg.visible
			if msg.visible <= 0 {
				m.chooser.selectHint = false
			}
		}
		return m, nil
	case spinner.TickMsg:
		if m.chooser.reloading {
			var cmd tea.Cmd
			m.chooser.spinner, cmd = m.chooser.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	case clearFlashMsg:
		if msg.id == m.chooser.flashID {
			m.chooser.flashMsg = ""
		}
		return m, nil
	}

	switch m.screen {
	case screenChooser:
		return m.updateChooser(msg)
	case screenSearch:
		return m.updateSearch(msg)
	case screenSave:
		return m.updateSave(msg)
	case screenHelp:
		return m.updateHelp(msg)
	}
	return m, nil
}

func (m model) View() string {
	var out string
	switch m.screen {
	case screenChooser:
		out = m.viewChooser()
	case screenSearch:
		out = m.viewSearch()
	case screenSave:
		out = m.viewSave()
	case screenHelp:
		out = m.viewHelp()
	}
	if m.width > 0 && m.height > 0 {
		out = padToScreen(out, m.width, m.height)
		if m.blackBg {
			out = forceBlackBg(out)
		} else {
			out = "\x1b[0m" + out
		}
	}
	return out
}


// padToScreen pads output to exactly w × h cells using literal spaces and
// newlines. lipgloss's Height() pads with bare `\n`, which only moves the
// cursor and leaves prior-frame cells untouched — that's why toggling off
// black bg left ghost rows. Writing real spaces forces a repaint.
func padToScreen(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if vw := lipgloss.Width(l); vw < w {
			lines[i] = l + strings.Repeat(" ", w-vw)
		}
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return strings.Join(lines, "\n")
}

// forceBlackBg re-asserts truecolor #000000 background after every SGR
// escape lipgloss emits (matching what the styles themselves use), so any
// reset to terminal-default that would expose unstyled cells gets paved
// over with the same exact black as the styled spans.
var sgrRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

const blackBgEsc = "\x1b[48;2;0;0;0m"

func forceBlackBg(s string) string {
	s = sgrRe.ReplaceAllStringFunc(s, func(m string) string { return m + blackBgEsc })
	return blackBgEsc + s
}

// ---------- chooser ----------

type menuOption struct {
	label, value string
}

type chooserModel struct {
	finderPath     string
	selected       int
	flashMsg       string
	flashID        int
	reloading       bool
	reloaded        bool
	reloadedID      int
	reloadedVisible int
	spinner         spinner.Model
	confirmingExit  bool
	exitChoice      int // 0 = Yes, 1 = No
	// "No path selected" → click → "Select a path in Finder" → fade back.
	selectHint        bool
	selectHintID      int
	selectHintVisible int
}

const selectHintWord = "Select a path in Finder"
const noPathWord = "No path selected"

// renderNoPathLabel returns the label for the "no path" menu slot. By default
// it shows "No path selected" in muted grey. When the user has clicked it, it
// switches to "Select a path in Finder" and fades letter-by-letter (left→right)
// back to "No path selected" — same animation as Reloaded.
func renderNoPathLabel(c chooserModel) string {
	if !c.selectHint {
		return styleMuted.Render(noPathWord)
	}
	v := c.selectHintVisible
	if v > len(selectHintWord) {
		v = len(selectHintWord)
	}
	if v < 0 {
		v = 0
	}
	// Left-to-right fade: replace the first (len-v) chars with spaces.
	shown := strings.Repeat(" ", len(selectHintWord)-v) + selectHintWord[len(selectHintWord)-v:]
	return styleDim.Render(shown)
}

func newChooserModel() chooserModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styleAccent
	return chooserModel{spinner: sp}
}

func (c chooserModel) options() []menuOption {
	var opts []menuOption
	if c.finderPath != "" {
		opts = append(opts, menuOption{styleBold.Render("Bookmark selected path"), "save"})
	} else {
		opts = append(opts, menuOption{renderNoPathLabel(c), "nopath"})
	}
	opts = append(opts,
		menuOption{styleBold.Render("Add bookmark"), "add"},
		menuOption{styleBold.Render("Search bookmarks"), "search"},
	)
	reloadLabel := styleDim.Render("Reload Finder path")
	if c.reloading {
		reloadLabel = styleDim.Render("Reload Finder path  ") + c.spinner.View()
	} else if c.reloaded {
		word := "Reloaded"
		v := c.reloadedVisible
		if v > len(word) {
			v = len(word)
		}
		if v < 0 {
			v = 0
		}
		// Remove from the left: replace the first (len-v) letters with spaces,
		// keep the remaining suffix so position is preserved.
		shown := strings.Repeat(" ", len(word)-v) + word[len(word)-v:]
		reloadLabel = styleDim.Render("Reload Finder path  ") + styleBoldGreen.Render(shown)
	}
	opts = append(opts,
		menuOption{reloadLabel, "reload"},
		menuOption{styleDim.Render("Open data folder"), "data_folder"},
		menuOption{styleDim.Render("Help"), "help"},
		menuOption{styleDim.Render("Exit"), "exit"},
	)
	return opts
}

func (m model) updateChooser(msg tea.Msg) (tea.Model, tea.Cmd) {
	opts := m.chooser.options()
	if m.chooser.confirmingExit {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "left", "h", "right", "l", "tab", "shift+tab":
				m.chooser.exitChoice = 1 - m.chooser.exitChoice
			case "enter":
				if m.chooser.exitChoice == 0 {
					return m, tea.Quit
				}
				m.chooser.confirmingExit = false
			case "y", "Y":
				return m, tea.Quit
			case "n", "N", "esc":
				m.chooser.confirmingExit = false
			}
		case tea.MouseMsg:
			if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
				if c := m.exitChoiceAt(msg.X, msg.Y); c >= 0 {
					m.chooser.exitChoice = c
					if c == 0 {
						return m, tea.Quit
					}
					m.chooser.confirmingExit = false
				}
			} else if msg.Action == tea.MouseActionMotion {
				if c := m.exitChoiceAt(msg.X, msg.Y); c >= 0 {
					m.chooser.exitChoice = c
				}
			}
		}
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.chooser.selected = (m.chooser.selected - 1 + len(opts)) % len(opts)
		case "down", "j":
			m.chooser.selected = (m.chooser.selected + 1) % len(opts)
		case "enter":
			return m.chooserSelect()
		}
	case tea.MouseMsg:
		idx := m.chooserItemAt(msg.Y)
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.chooser.selected = (m.chooser.selected - 1 + len(opts)) % len(opts)
		case tea.MouseButtonWheelDown:
			m.chooser.selected = (m.chooser.selected + 1) % len(opts)
		case tea.MouseButtonLeft:
			if idx >= 0 && msg.Action == tea.MouseActionPress {
				m.chooser.selected = idx
				return m.chooserSelect()
			}
		default:
			if msg.Action == tea.MouseActionMotion && idx >= 0 {
				m.chooser.selected = idx
			}
		}
	}
	return m, nil
}

// chooserMenuStartLine is the Y offset (0-indexed) of the first menu option
// in viewChooser. Header: blank, brand, blank, path, blank = 5 lines.
const chooserMenuStartLine = 5

// Visible X offsets of "[ Yes ]" and "[ No ]" in the exit-confirm status bar.
// Layout: "  Exit?  [ Yes ]  [ No ]"
const (
	exitYesX = 9
	exitYesW = 7
	exitNoX  = 18
	exitNoW  = 6
)

func (m model) exitChoiceAt(x, y int) int {
	if m.height > 0 && y != m.height-1 {
		return -1
	}
	if x >= exitYesX && x < exitYesX+exitYesW {
		return 0
	}
	if x >= exitNoX && x < exitNoX+exitNoW {
		return 1
	}
	return -1
}

func (m model) chooserItemAt(y int) int {
	idx := y - chooserMenuStartLine
	if idx < 0 || idx >= len(m.chooser.options()) {
		return -1
	}
	return idx
}

func (m model) chooserSelect() (model, tea.Cmd) {
	opts := m.chooser.options()
	if m.chooser.selected >= len(opts) {
		return m, nil
	}
	switch opts[m.chooser.selected].value {
	case "save":
		path := resolveStored(m.chooser.finderPath)
		entries := load()
		var existing *bookmark
		if path != "" {
			for i := range entries {
				if entries[i].Path == path {
					existing = &entries[i]
					break
				}
			}
		}
		m.save = newSaveModel(path, existing)
		m.saveReturnTo = screenChooser
		m.screen = screenSave
	case "add":
		m.save = newSaveModel("", nil)
		m.saveReturnTo = screenChooser
		m.screen = screenSave
	case "nopath":
		m.chooser.selectHint = true
		m.chooser.selectHintVisible = len(selectHintWord)
		m.chooser.selectHintID++
		id := m.chooser.selectHintID
		return m, tea.Tick(1*time.Second, func(time.Time) tea.Msg {
			return startSelectHintFadeMsg{id: id}
		})
	case "search":
		m.search = newSearchModel("")
		m.search.viewport.Width = m.width
		m.search.viewport.Height = m.height - 4
		m.search.viewport.SetContent(renderSearchResults(m.search, m.width))
		m.screen = screenSearch
	case "reload":
		m.chooser.reloading = true
		m.chooser.reloaded = false
		return m, tea.Batch(reloadFinderPathCmd, m.chooser.spinner.Tick)
	case "data_folder":
		dir := filepath.Dir(dataFile)
		cmd := func() tea.Msg {
			os.MkdirAll(dir, 0755)
			exec.Command("/usr/bin/open", dir).Run()
			return openDoneMsg{}
		}
		m.chooser.flashMsg = "Opened " + styleDim.Render(dir)
		m.chooser.flashID++
		id := m.chooser.flashID
		return m, tea.Batch(cmd, tea.Tick(1800*time.Millisecond, func(time.Time) tea.Msg {
			return clearFlashMsg{id}
		}))
	case "help":
		m.screen = screenHelp
	case "exit":
		m.chooser.confirmingExit = true
		m.chooser.exitChoice = 0
		return m, nil
	}
	return m, nil
}

func (m model) chooserFlash(msg string, ms int) tea.Cmd {
	m.chooser.flashMsg = msg
	m.chooser.flashID++
	id := m.chooser.flashID
	return tea.Tick(time.Duration(ms)*time.Millisecond, func(time.Time) tea.Msg {
		return clearFlashMsg{id}
	})
}


func (m model) viewChooser() string {
	var sb strings.Builder

	// header
	sb.WriteString("\n")
	sb.WriteString(renderBrand("bookmark anything") + "\n\n")
	if m.chooser.finderPath != "" {
		sb.WriteString("  " + styleAccent2.Render("◆") + "  " + styleDim.Render(m.chooser.finderPath) + "\n")
	} else {
		sb.WriteString("  " + styleBarDim.Render("◇") + "  " + styleDim.Render("no Finder path") + "\n")
	}
	sb.WriteString("\n")

	// menu
	opts := m.chooser.options()
	for i, opt := range opts {
		var prefix string
		if i == m.chooser.selected {
			prefix = " " + styleBar.Render(barChar) + " "
		} else {
			prefix = "   "
		}
		sb.WriteString(prefix + opt.label + "\n")
	}

	// fill + status
	status := styleMuted.Render("  ↑↓/mouse navigate   enter/click select   ^c quit")
	if m.chooser.flashMsg != "" {
		status = "  " + m.chooser.flashMsg
	}
	if m.chooser.confirmingExit {
		yes, no := "[ Yes ]", "[ No ]"
		if m.chooser.exitChoice == 0 {
			yes = styleBoldYellow.Render(yes)
			no = styleDim.Render(no)
		} else {
			yes = styleDim.Render(yes)
			no = styleBoldYellow.Render(no)
		}
		status = "  " + styleBold.Render("Exit?") + "  " + yes + "  " + no + styleMuted.Render("   ←→ choose · enter confirm · esc cancel")
	}
	body := sb.String()
	if m.height > 0 {
		used := lipgloss.Height(body) + 1
		if pad := m.height - used; pad > 0 {
			body += strings.Repeat("\n", pad)
		}
	}
	return body + status
}

// ---------- search ----------

type searchMode int

const (
	modeNormal searchMode = iota
	modeDelete
	modeMulti
)

const maxVisible = 50

type searchModel struct {
	input       textinput.Model
	allEntries  []bookmark
	searchTexts []string
	matches     []bookmark
	lastQuery   string
	selected    int
	mode        searchMode
	marked      map[int]bool
	confirming  bool
	viewport    viewport.Model
	debounceID  int
}

func newSearchModel(initial string) searchModel {
	ti := textinput.New()
	ti.Placeholder = "Search bookmarks..."
	ti.SetValue(initial)
	ti.Focus()
	ti.KeyMap.DeleteCharacterForward.Unbind()
	ti.KeyMap.LineEnd.Unbind()
	styleInput(&ti)

	entries := load()
	texts := buildSearchTexts(entries)
	matches := filterEntries(initial, entries, texts)
	if len(matches) > maxVisible {
		matches = matches[:maxVisible]
	}

	vp := viewport.New(0, 0)

	sm := searchModel{
		input:       ti,
		allEntries:  entries,
		searchTexts: texts,
		matches:     matches,
		lastQuery:   strings.TrimSpace(initial),
		marked:      make(map[int]bool),
		viewport:    vp,
	}
	return sm
}

func (m model) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		// mode-specific keys
		if m.search.mode != modeNormal {
			return m.updateSearchMode(key)
		}

		// intercept special keys before textinput
		switch key {
		case "esc":
			m.screen = screenChooser
			return m, nil
		case "ctrl+d":
			if len(m.search.matches) > 0 {
				m.search = enterSearchMode(m.search, modeDelete)
			}
			return m, nil
		case "ctrl+e":
			if len(m.search.matches) > 0 && m.search.selected < len(m.search.matches) {
				e := m.search.matches[m.search.selected]
				ep := e
				m.save = newSaveModel(e.Path, &ep)
				m.saveReturnTo = screenSearch
				m.screen = screenSave
			}
			return m, nil
		case "ctrl+o":
			if len(m.search.matches) > 0 {
				m.search = enterSearchMode(m.search, modeMulti)
			}
			return m, nil
		case "up":
			if n := len(m.search.matches); n > 0 {
				m.search.selected = (m.search.selected - 1 + n) % n
				m.search.viewport.SetContent(renderSearchResults(m.search, m.width))
				m.scrollToSelected()
			}
			return m, nil
		case "down":
			if n := len(m.search.matches); n > 0 {
				m.search.selected = (m.search.selected + 1) % n
				m.search.viewport.SetContent(renderSearchResults(m.search, m.width))
				m.scrollToSelected()
			}
			return m, nil
		case "enter":
			if len(m.search.matches) > 0 && m.search.selected < len(m.search.matches) {
				path := m.search.matches[m.search.selected].Path
				return m, func() tea.Msg { openPath(path); return openDoneMsg{} }
			}
			return m, nil
		}

		// pass to textinput
		var cmd tea.Cmd
		m.search.input, cmd = m.search.input.Update(msg)
		q := m.search.input.Value()
		m.search.debounceID++
		debounce := debounceCmd(m.search.debounceID, q)
		return m, tea.Batch(cmd, debounce)

	case debounceMsg:
		if msg.id != m.search.debounceID {
			return m, nil
		}
		m.search = runFilter(m.search, msg.query)
		m.search.viewport.SetContent(renderSearchResults(m.search, m.width))
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.search.viewport.Width = m.width
		m.search.viewport.Height = m.height - 4
		m.search.viewport.SetContent(renderSearchResults(m.search, m.width))
		return m, nil

	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if n := len(m.search.matches); n > 0 {
				m.search.selected = (m.search.selected - 1 + n) % n
				m.search.viewport.SetContent(renderSearchResults(m.search, m.width))
				m.scrollToSelected()
			}
		case tea.MouseButtonWheelDown:
			if n := len(m.search.matches); n > 0 {
				m.search.selected = (m.search.selected + 1) % n
				m.search.viewport.SetContent(renderSearchResults(m.search, m.width))
				m.scrollToSelected()
			}
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.search.input, cmd = m.search.input.Update(msg)
	return m, cmd
}

func (m model) updateSearchMode(key string) (model, tea.Cmd) {
	n := len(m.search.matches)
	switch key {
	case "down":
		if n > 0 {
			m.search.selected = (m.search.selected + 1) % n
			m.search.viewport.SetContent(renderSearchResults(m.search, m.width))
			m.scrollToSelected()
		}
	case "up":
		if n > 0 {
			m.search.selected = (m.search.selected - 1 + n) % n
			m.search.viewport.SetContent(renderSearchResults(m.search, m.width))
			m.scrollToSelected()
		}
	case " ":
		idx := m.search.selected
		if n > 0 {
			if m.search.marked[idx] {
				delete(m.search.marked, idx)
			} else {
				m.search.marked[idx] = true
			}
			m.search.viewport.SetContent(renderSearchResults(m.search, m.width))
		}
	case "enter":
		if len(m.search.marked) > 0 {
			if m.search.mode == modeDelete {
				if !m.search.confirming {
					m.search.confirming = true
				} else {
					m.search = doDelete(m.search)
				}
			} else if m.search.mode == modeMulti {
				m.search = doMultiOpen(m.search)
			}
			m.search.viewport.SetContent(renderSearchResults(m.search, m.width))
		}
	case "esc":
		if m.search.confirming {
			m.search.confirming = false
		} else {
			m.search = exitSearchMode(m.search)
		}
		m.search.viewport.SetContent(renderSearchResults(m.search, m.width))
	}
	return m, nil
}

func enterSearchMode(sm searchModel, mode searchMode) searchModel {
	sm.mode = mode
	sm.marked = make(map[int]bool)
	sm.confirming = false
	sm.input.Blur()
	return sm
}

func exitSearchMode(sm searchModel) searchModel {
	sm.mode = modeNormal
	sm.marked = make(map[int]bool)
	sm.confirming = false
	sm.input.Focus()
	return sm
}

func doDelete(sm searchModel) searchModel {
	toDelete := make(map[string]bool)
	for i := range sm.marked {
		if i < len(sm.matches) {
			toDelete[sm.matches[i].Path] = true
		}
	}
	var kept []bookmark
	for _, e := range sm.allEntries {
		if !toDelete[e.Path] {
			kept = append(kept, e)
		}
	}
	persist(kept)
	sm.allEntries = kept
	sm.searchTexts = buildSearchTexts(kept)
	sm = runFilter(sm, sm.lastQuery)
	return exitSearchMode(sm)
}

func doMultiOpen(sm searchModel) searchModel {
	for i := range sm.marked {
		if i < len(sm.matches) {
			path := sm.matches[i].Path
			go openPath(path)
		}
	}
	return exitSearchMode(sm)
}

func runFilter(sm searchModel, q string) searchModel {
	var source []bookmark
	var texts []string
	if q != "" && sm.lastQuery != "" && strings.HasPrefix(q, sm.lastQuery) {
		source = sm.matches
		texts = buildSearchTexts(source)
	} else {
		source = sm.allEntries
		texts = sm.searchTexts
	}
	filtered := filterEntries(q, source, texts)
	if len(filtered) > maxVisible {
		filtered = filtered[:maxVisible]
	}
	sm.matches = filtered
	sm.lastQuery = q
	sm.selected = 0
	return sm
}

func (m *model) scrollToSelected() {
	if m.height == 0 {
		return
	}
	// compute line offset of selected item
	offset := 0
	for i := 0; i < m.search.selected && i < len(m.search.matches); i++ {
		lines := 2 // title + path
		if m.search.matches[i].Description != "" {
			lines++
		}
		lines++ // blank separator
		offset += lines
	}
	vpH := m.search.viewport.Height
	if offset < m.search.viewport.YOffset {
		m.search.viewport.SetYOffset(offset)
	} else if offset >= m.search.viewport.YOffset+vpH {
		m.search.viewport.SetYOffset(offset - vpH + 3)
	}
}

func renderBookmarkItem(e bookmark, index int, selected bool, mode searchMode, marked map[int]bool, width int) string {
	var indicator string
	if mode != modeNormal {
		if marked[index] {
			if mode == modeDelete {
				indicator = styleBoldRed.Render("✕")
			} else {
				indicator = styleBoldGreen.Render("✓")
			}
		} else {
			indicator = styleDim.Render("·")
		}
	} else {
		indicator = styleAccent2.Render(fmt.Sprintf("%d", index+1))
	}

	title := styleBold.Render(e.Title)
	var content []string
	content = append(content, indicator+"  "+title)
	if e.Description != "" {
		content = append(content, "   "+styleDim.Render(e.Description))
	}
	content = append(content, "   "+styleDimItalic.Render(e.Path))

	bar := styleBarDim.Render(barChar)
	if selected && mode == modeNormal {
		bar = styleBar.Render(barChar)
	}
	for i, l := range content {
		content[i] = " " + bar + "  " + l
	}
	return strings.Join(content, "\n") + "\n\n"
}

func renderSearchResults(sm searchModel, width int) string {
	var sb strings.Builder
	for i, e := range sm.matches {
		sb.WriteString(renderBookmarkItem(e, i, i == sm.selected, sm.mode, sm.marked, width))
	}
	return sb.String()
}

func renderSearchStatus(sm searchModel, total int) string {
	switch sm.mode {
	case modeDelete:
		n := len(sm.marked)
		if sm.confirming {
			noun := "bookmarks"
			if n == 1 {
				noun = "bookmark"
			}
			return styleBoldRed.Render(fmt.Sprintf(" Delete %d %s?", n, noun)) +
				styleMuted.Render("   enter confirm   esc cancel")
		}
		sel := ""
		if n > 0 {
			sel = "  " + styleBold.Render(fmt.Sprintf("%d selected", n))
		}
		return styleMuted.Render(" ↑↓ navigate   space toggle   enter delete") + sel + styleMuted.Render("   esc back")
	case modeMulti:
		n := len(sm.marked)
		sel := ""
		if n > 0 {
			sel = "  " + styleBold.Render(fmt.Sprintf("%d selected", n))
		}
		return styleMuted.Render(" ↑↓ navigate   space toggle   enter open all") + sel + styleMuted.Render("   esc back")
	default:
		matched := len(sm.matches)
		if matched > maxVisible {
			return styleMuted.Render(fmt.Sprintf(" showing %d of %d matches (%d total)   ↑↓ enter open   ^e edit   ^d delete   ^o multi   esc back", maxVisible, matched, total))
		}
		return styleMuted.Render(fmt.Sprintf(" %d/%d bookmarks   ↑↓ enter open   ^e edit   ^d delete   ^o multi   esc back", matched, total))
	}
}

func (m model) viewSearch() string {
	header := "\n" + renderBrand("search") + "\n\n  " +
		styleAccent.Render("›") + " " + m.search.input.View() + "\n"
	status := renderSearchStatus(m.search, len(m.search.allEntries))

	bodyHeight := m.height - lipgloss.Height(header) - 1
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	m.search.viewport.Height = bodyHeight
	m.search.viewport.Width = m.width

	body := m.search.viewport.View()
	return lipgloss.JoinVertical(lipgloss.Left, header, body, status)
}

// ---------- save ----------

type saveModel struct {
	showPath   bool
	inputs     [3]textinput.Model
	focused    int
	existing   *bookmark
	storedPath string
	headerText string
}

const (
	inputPath  = 0
	inputTitle = 1
	inputDesc  = 2
)

func newSaveModel(path string, existing *bookmark) saveModel {
	makeInput := func(placeholder, value string) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.SetValue(value)
		styleInput(&ti)
		return ti
	}

	showPath := existing != nil || path == ""
	pathVal := path
	titleVal := ""
	descVal := ""
	if existing != nil {
		pathVal = existing.Path
		titleVal = existing.Title
		descVal = existing.Description
	}

	sm := saveModel{
		showPath:   showPath,
		existing:   existing,
		storedPath: path,
	}
	sm.inputs[inputPath] = makeInput("Path / URL (required)", pathVal)
	sm.inputs[inputTitle] = makeInput("Title (required)", titleVal)
	sm.inputs[inputDesc] = makeInput("Description (optional)", descVal)

	if showPath {
		sm.focused = inputPath
		sm.inputs[inputPath].Focus()
	} else {
		sm.focused = inputTitle
		sm.inputs[inputTitle].Focus()
	}

	if existing != nil {
		sm.headerText = styleBoldYellow.Render("Editing bookmark")
	} else if path != "" {
		sm.headerText = styleBold.Render("Saving:") + " " + styleDim.Render(path)
	} else {
		sm.headerText = styleBold.Render("New bookmark")
	}

	return sm
}

func (m model) updateSave(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.screen = m.saveReturnTo
			if m.saveReturnTo == screenSearch {
				// reload entries in case nothing changed
				m.search.allEntries = load()
				m.search.searchTexts = buildSearchTexts(m.search.allEntries)
				m.search = runFilter(m.search, m.search.lastQuery)
				m.search.viewport.SetContent(renderSearchResults(m.search, m.width))
			}
			return m, nil
		case "enter":
			return m.saveAdvance()
		}
		// pass to focused input
		var cmd tea.Cmd
		m.save.inputs[m.save.focused], cmd = m.save.inputs[m.save.focused].Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.save.inputs[m.save.focused], cmd = m.save.inputs[m.save.focused].Update(msg)
	return m, cmd
}

func (m model) saveAdvance() (model, tea.Cmd) {
	sm := m.save
	focused := sm.focused

	switch focused {
	case inputPath:
		if strings.TrimSpace(sm.inputs[inputPath].Value()) != "" {
			sm.inputs[inputPath].Blur()
			sm.focused = inputTitle
			sm.inputs[inputTitle].Focus()
			m.save = sm
			return m, nil
		}
	case inputTitle:
		if strings.TrimSpace(sm.inputs[inputTitle].Value()) != "" {
			sm.inputs[inputTitle].Blur()
			sm.focused = inputDesc
			sm.inputs[inputDesc].Focus()
			m.save = sm
			return m, nil
		}
	case inputDesc:
		title := strings.TrimSpace(sm.inputs[inputTitle].Value())
		if title == "" {
			sm.inputs[inputDesc].Blur()
			sm.focused = inputTitle
			sm.inputs[inputTitle].Focus()
			m.save = sm
			return m, nil
		}
		var path string
		if sm.showPath {
			path = strings.TrimSpace(sm.inputs[inputPath].Value())
			if path == "" {
				sm.inputs[inputDesc].Blur()
				sm.focused = inputPath
				sm.inputs[inputPath].Focus()
				m.save = sm
				return m, nil
			}
		} else {
			path = sm.storedPath
		}
		desc := strings.TrimSpace(sm.inputs[inputDesc].Value())
		action, savedTitle := commitSave(path, title, desc, sm.existing)
		flash := styleBoldGreen.Render(capitalise(action)+":") + " " + savedTitle
		m.chooser.flashMsg = flash
		m.chooser.flashID++
		id := m.chooser.flashID
		// also refresh search if we came from there
		if m.saveReturnTo == screenSearch {
			m.search.allEntries = load()
			m.search.searchTexts = buildSearchTexts(m.search.allEntries)
			m.search = runFilter(m.search, m.search.lastQuery)
			m.search.viewport.SetContent(renderSearchResults(m.search, m.width))
			m.screen = screenSearch
		} else {
			m.screen = screenChooser
		}
		return m, tea.Tick(1800*time.Millisecond, func(time.Time) tea.Msg {
			return clearFlashMsg{id}
		})
	}
	m.save = sm
	return m, nil
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func commitSave(path, title, desc string, existing *bookmark) (action, savedTitle string) {
	stored := resolveStored(path)
	entries := load()
	oldPath := stored
	if existing != nil {
		oldPath = existing.Path
	}
	var target *bookmark
	for i := range entries {
		if entries[i].Path == oldPath {
			target = &entries[i]
			break
		}
	}
	if target == nil && existing == nil {
		for i := range entries {
			if entries[i].Path == stored {
				target = &entries[i]
				break
			}
		}
	}
	if target == nil {
		entries = append(entries, bookmark{Path: stored, Title: title, Description: desc})
		action = "saved"
	} else {
		target.Path = stored
		target.Title = title
		target.Description = desc
		action = "updated"
	}
	persist(entries)
	return action, title
}

func (m model) viewSave() string {
	sm := m.save
	var sb strings.Builder
	sb.WriteString("\n" + renderBrand("save") + "\n\n")
	sb.WriteString("  " + sm.headerText + "\n\n")

	if sm.showPath {
		sb.WriteString("  " + sm.inputs[inputPath].View() + "\n\n")
	}
	sb.WriteString("  " + sm.inputs[inputTitle].View() + "\n\n")
	sb.WriteString("  " + sm.inputs[inputDesc].View() + "\n")

	body := sb.String()
	status := styleMuted.Render("  enter confirm   esc cancel")
	if m.height > 0 {
		used := lipgloss.Height(body) + 1
		if pad := m.height - used; pad > 0 {
			body += strings.Repeat("\n", pad)
		}
	}
	return body + status
}

// ---------- help ----------

const helpText = `Usage:
  lk                     Open main menu (grab Finder path or search)
  lk /some/path          Save a folder or file
  lk https://example.com Save a URL
  lk something           Search bookmarks and open selected result

Search screen:
  ↑↓        navigate results
  enter     open selected bookmark
  ^e        edit selected bookmark
  ^d        enter delete mode (space to mark, enter to confirm)
  ^o        enter multi-open mode (space to mark, enter to open all)
  esc       go back

Save screen:
  enter     advance / confirm
  esc       cancel

Global:
  ^b        toggle black background
  ^c        quit`

func (m model) updateHelp(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "enter", "esc", "q":
			m.screen = screenChooser
		}
	}
	return m, nil
}

func (m model) viewHelp() string {
	body := "\n" + renderBrand("help") + "\n\n" +
		lipgloss.NewStyle().PaddingLeft(2).Render(helpText) + "\n"
	status := styleMuted.Render("  enter/esc close")
	if m.height > 0 {
		used := lipgloss.Height(body) + 1
		if pad := m.height - used; pad > 0 {
			body += strings.Repeat("\n", pad)
		}
	}
	return body + status
}
