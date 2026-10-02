// Package history locates and parses shell history files for bash, zsh and fish.
package history

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Shell identifies a supported shell.
type Shell string

const (
	Bash    Shell = "bash"
	Zsh     Shell = "zsh"
	Fish    Shell = "fish"
	Unknown Shell = "unknown"
)

// Source describes where history was read from.
type Source struct {
	Shell Shell
	Path  string
}

// DetectShell guesses the current shell from the $SHELL env var.
func DetectShell() Shell {
	return shellFromPath(os.Getenv("SHELL"))
}

func shellFromPath(p string) Shell {
	base := strings.ToLower(filepath.Base(p))
	switch {
	case strings.Contains(base, "bash"):
		return Bash
	case strings.Contains(base, "zsh"):
		return Zsh
	case strings.Contains(base, "fish"):
		return Fish
	default:
		return Unknown
	}
}

// DefaultPath returns the conventional history file path for a shell.
// An empty home disables detection and returns "".
func DefaultPath(sh Shell, home string) string {
	if home == "" {
		return ""
	}
	switch sh {
	case Bash:
		return filepath.Join(home, ".bash_history")
	case Zsh:
		return filepath.Join(home, ".zsh_history")
	case Fish:
		return filepath.Join(home, ".local", "share", "fish", "fish_history")
	default:
		return ""
	}
}

// Locate finds a usable history source. It honours an explicit path/shell when
// given, otherwise it detects the shell and probes known locations, falling
// back to whichever default history file actually exists on disk.
func Locate(explicitPath string, explicitShell Shell, home string) (Source, bool) {
	if explicitPath != "" {
		sh := explicitShell
		if sh == "" || sh == Unknown {
			sh = guessShellFromFile(explicitPath)
		}
		return Source{Shell: sh, Path: explicitPath}, true
	}

	if explicitShell != "" && explicitShell != Unknown {
		if p := DefaultPath(explicitShell, home); p != "" && fileExists(p) {
			return Source{Shell: explicitShell, Path: p}, true
		}
	}

	order := []Shell{DetectShell(), Zsh, Bash, Fish}
	for _, sh := range order {
		if sh == Unknown {
			continue
		}
		if p := DefaultPath(sh, home); p != "" && fileExists(p) {
			return Source{Shell: sh, Path: p}, true
		}
	}
	return Source{}, false
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func guessShellFromFile(path string) Shell {
	base := strings.ToLower(filepath.Base(path))
	switch {
	case strings.Contains(base, "fish"):
		return Fish
	case strings.Contains(base, "zsh"):
		return Zsh
	case strings.Contains(base, "bash"):
		return Bash
	default:
		return Unknown
	}
}

// Parse reads a history source and returns the command lines in order,
// stripped of shell-specific metadata and timestamps.
func Parse(src Source) ([]string, error) {
	f, err := os.Open(src.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseReader(src.Shell, f)
}

// ParseReader parses history from any reader for the given shell. It is the
// testable core of Parse.
func ParseReader(sh Shell, r io.Reader) ([]string, error) {
	switch sh {
	case Fish:
		return parseFish(r)
	case Zsh:
		return parseZsh(r)
	default:
		return parseBash(r)
	}
}

// parseBash handles plain one-command-per-line files. Lines beginning with a
// "#1700000000" epoch comment (HISTTIMEFORMAT) are treated as timestamps.
func parseBash(r io.Reader) ([]string, error) {
	var cmds []string
	sc := newScanner(r)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if isBashTimestamp(line) {
			continue
		}
		cmds = append(cmds, line)
	}
	return cmds, sc.Err()
}

func isBashTimestamp(line string) bool {
	if len(line) < 2 || line[0] != '#' {
		return false
	}
	for _, c := range line[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// parseZsh handles the extended format ": <epoch>:<elapsed>;cmd" as well as
// plain lines. Multi-line commands end with a trailing backslash.
func parseZsh(r io.Reader) ([]string, error) {
	var cmds []string
	sc := newScanner(r)
	var pending strings.Builder
	continuing := false
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if !continuing {
			line = stripZshMeta(line)
		}
		if strings.HasSuffix(line, "\\") {
			pending.WriteString(strings.TrimSuffix(line, "\\"))
			pending.WriteByte('\n')
			continuing = true
			continue
		}
		if continuing {
			pending.WriteString(line)
			cmd := strings.TrimSpace(pending.String())
			if cmd != "" {
				cmds = append(cmds, cmd)
			}
			pending.Reset()
			continuing = false
			continue
		}
		if line != "" {
			cmds = append(cmds, line)
		}
	}
	if pending.Len() > 0 {
		if cmd := strings.TrimSpace(pending.String()); cmd != "" {
			cmds = append(cmds, cmd)
		}
	}
	return cmds, sc.Err()
}

// stripZshMeta removes the ": <epoch>:<elapsed>;" prefix if present.
func stripZshMeta(line string) string {
	if !strings.HasPrefix(line, ": ") {
		return line
	}
	if i := strings.IndexByte(line, ';'); i != -1 {
		return line[i+1:]
	}
	return line
}

// parseFish handles the YAML-ish fish_history format:
//
//   - cmd: git status
//     when: 1700000000
func parseFish(r io.Reader) ([]string, error) {
	var cmds []string
	sc := newScanner(r)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimLeft(line, " ")
		const prefix = "- cmd: "
		if strings.HasPrefix(trimmed, prefix) {
			cmd := strings.TrimRight(strings.TrimPrefix(trimmed, prefix), "\r")
			cmd = unescapeFish(cmd)
			if cmd != "" {
				cmds = append(cmds, cmd)
			}
		}
	}
	return cmds, sc.Err()
}

// unescapeFish reverses fish's backslash escaping of newlines and backslashes.
func unescapeFish(s string) string {
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}

// newScanner builds a bufio.Scanner with a large buffer so long command lines
// are not truncated.
func newScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return sc
}
