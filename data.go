package main

import (
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var dataFile = filepath.Join(os.Getenv("HOME"), ".local", "share", "lk", "lk_data.json")

type bookmark struct {
	Path        string `json:"path"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

func load() []bookmark {
	data, err := os.ReadFile(dataFile)
	if err != nil {
		return []bookmark{}
	}
	var entries []bookmark
	if err := json.Unmarshal(data, &entries); err != nil {
		return []bookmark{}
	}
	return entries
}

func persist(entries []bookmark) error {
	if err := os.MkdirAll(filepath.Dir(dataFile), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dataFile, data, 0644)
}

func normalisePath(s string) string {
	if strings.HasPrefix(s, "smb://") {
		u, err := url.Parse(s)
		if err == nil {
			decoded, _ := url.PathUnescape(u.Path)
			return "/Volumes" + decoded
		}
	}
	if strings.HasPrefix(s, "file://") {
		u, err := url.Parse(s)
		if err == nil {
			decoded, _ := url.PathUnescape(u.Path)
			return decoded
		}
	}
	if strings.HasPrefix(s, "~") {
		return filepath.Join(os.Getenv("HOME"), s[1:])
	}
	return s
}

func resolveStored(s string) string {
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	}
	normalised := normalisePath(s)
	abs, err := filepath.Abs(normalised)
	if err != nil {
		return normalised
	}
	return abs
}

func isPathOrURL(s string) bool {
	for _, prefix := range []string{"http://", "https://", "smb://", "/", "./", "../", "~"} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func openPath(path string) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		exec.Command("/usr/bin/open", path).Run()
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		exec.Command("/usr/bin/open", path).Run()
		return
	}
	if info.Mode().IsRegular() {
		exec.Command("osascript", "-e", `
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
		`, path).Run()
	} else {
		exec.Command("osascript", "-e", `
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
		`, path).Run()
	}
}

func getFinderPath() string {
	out, err := exec.Command("osascript", "-e", `
		tell app "Finder"
			set sel to selection
			if sel is {} then
				POSIX path of (target of front window as alias)
			else
				POSIX path of (item 1 of sel as alias)
			end if
		end tell
	`).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
