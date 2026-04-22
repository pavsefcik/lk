package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type debounceMsg struct {
	id    int
	query string
}

func debounceCmd(id int, query string) tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg {
		return debounceMsg{id: id, query: query}
	})
}

func buildSearchTexts(entries []bookmark) []string {
	texts := make([]string, len(entries))
	for i, e := range entries {
		texts[i] = strings.ToLower(fmt.Sprintf("%s %s %s", e.Title, e.Description, e.Path))
	}
	return texts
}

func lcsLen(a, b []rune) int {
	if len(b) < len(a) {
		a, b = b, a
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for _, ca := range a {
		for j, cb := range b {
			if ca == cb {
				curr[j+1] = prev[j] + 1
			} else if curr[j] > prev[j+1] {
				curr[j+1] = curr[j]
			} else {
				curr[j+1] = prev[j+1]
			}
		}
		prev, curr = curr, prev
		for i := range curr {
			curr[i] = 0
		}
	}
	return prev[len(b)]
}

func matchRatio(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 && lb == 0 {
		return 1.0
	}
	if la == 0 || lb == 0 {
		return 0.0
	}
	m := lcsLen(ra, rb)
	return float64(2*m) / float64(la+lb)
}

func wordMatches(word, haystack string) bool {
	if strings.Contains(haystack, word) {
		return true
	}
	for _, hw := range strings.Fields(haystack) {
		if matchRatio(word, hw) >= 0.8 {
			return true
		}
	}
	return false
}

func filterEntries(query string, entries []bookmark, texts []string) []bookmark {
	query = strings.TrimSpace(query)
	if query == "" {
		return entries
	}
	words := strings.Fields(strings.ToLower(query))
	var out []bookmark
	for i, e := range entries {
		ok := true
		for _, w := range words {
			if !wordMatches(w, texts[i]) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, e)
		}
	}
	return out
}
