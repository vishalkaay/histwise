// Package analyze turns a stream of history command lines into usage statistics
// and ranked alias suggestions.
package analyze

import (
	"sort"
	"strings"
)

// CommandStat is an aggregated count for a normalized command line.
type CommandStat struct {
	Command string `json:"command"`
	Count   int    `json:"count"`
}

// ProgramStat is an aggregated count for a program (first token).
type ProgramStat struct {
	Program string `json:"program"`
	Count   int    `json:"count"`
}

// AliasSuggestion is a proposed alias for a frequently typed command line.
type AliasSuggestion struct {
	Alias   string `json:"alias"`
	Command string `json:"command"`
	Count   int    `json:"count"`
	Saved   int    `json:"keystrokes_saved"`
	perUse  int
}

// Report is the full analysis result.
type Report struct {
	TotalCommands  int               `json:"total_commands"`
	UniqueCommands int               `json:"unique_commands"`
	TopCommands    []CommandStat     `json:"top_commands"`
	TopPrograms    []ProgramStat     `json:"top_programs"`
	Suggestions    []AliasSuggestion `json:"alias_suggestions"`
}

// Options tunes the analysis.
type Options struct {
	TopN         int      // how many entries in each top list
	MinCount     int      // ignore commands seen fewer than this many times
	MaxAliasLen  int      // only suggest aliases for commands at least this long
	Ignore       []string // programs to skip entirely (e.g. "ls", "cd")
	ReservedKeep []string // reserved alias names never to generate
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options {
	return Options{
		TopN:        15,
		MinCount:    3,
		MaxAliasLen: 6,
	}
}

// trimmedFields splits a command into fields, collapsing runs of whitespace.
func normalize(cmd string) string {
	return strings.Join(strings.Fields(cmd), " ")
}

func firstToken(cmd string) string {
	for _, f := range strings.Fields(cmd) {
		// Skip leading VAR=value environment assignments.
		if strings.ContainsRune(f, '=') && !strings.ContainsAny(f, "/.") {
			continue
		}
		if f == "sudo" {
			continue
		}
		return f
	}
	return ""
}

// Analyze computes a full report from command lines.
func Analyze(commands []string, opts Options) Report {
	cmdCounts := make(map[string]int)
	progCounts := make(map[string]int)
	ignore := toSet(opts.Ignore)

	total := 0
	for _, raw := range commands {
		cmd := normalize(raw)
		if cmd == "" {
			continue
		}
		total++
		cmdCounts[cmd]++
		if prog := firstToken(cmd); prog != "" {
			if _, skip := ignore[prog]; !skip {
				progCounts[prog]++
			}
		}
	}

	rep := Report{
		TotalCommands:  total,
		UniqueCommands: len(cmdCounts),
		TopCommands:    topCommands(cmdCounts, opts.TopN),
		TopPrograms:    topPrograms(progCounts, opts.TopN),
		Suggestions:    suggestAliases(cmdCounts, opts),
	}
	return rep
}

func toSet(items []string) map[string]struct{} {
	s := make(map[string]struct{}, len(items))
	for _, it := range items {
		s[it] = struct{}{}
	}
	return s
}

func topCommands(counts map[string]int, n int) []CommandStat {
	stats := make([]CommandStat, 0, len(counts))
	for c, n := range counts {
		stats = append(stats, CommandStat{Command: c, Count: n})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Count != stats[j].Count {
			return stats[i].Count > stats[j].Count
		}
		return stats[i].Command < stats[j].Command
	})
	return clampCmd(stats, n)
}

func topPrograms(counts map[string]int, n int) []ProgramStat {
	stats := make([]ProgramStat, 0, len(counts))
	for p, c := range counts {
		stats = append(stats, ProgramStat{Program: p, Count: c})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Count != stats[j].Count {
			return stats[i].Count > stats[j].Count
		}
		return stats[i].Program < stats[j].Program
	})
	return clampProg(stats, n)
}

func clampCmd(s []CommandStat, n int) []CommandStat {
	if n > 0 && len(s) > n {
		return s[:n]
	}
	return s
}

func clampProg(s []ProgramStat, n int) []ProgramStat {
	if n > 0 && len(s) > n {
		return s[:n]
	}
	return s
}

// suggestAliases ranks commands by total keystrokes saved. The alias name is a
// short mnemonic built from the command, and we only propose it when it is
// meaningfully shorter than the command itself and does not collide.
func suggestAliases(counts map[string]int, opts Options) []AliasSuggestion {
	reserved := toSet(opts.ReservedKeep)
	usedAlias := make(map[string]string) // alias -> command

	var out []AliasSuggestion
	for cmd, count := range counts {
		if count < opts.MinCount {
			continue
		}
		if len([]rune(cmd)) < opts.MaxAliasLen {
			continue
		}
		alias := mnemonic(cmd)
		if alias == "" {
			continue
		}
		if _, bad := reserved[alias]; bad {
			continue
		}
		// Saving per use is command length minus alias length plus the space
		// you would have typed after the alias; require a real win.
		perUse := len([]rune(cmd)) - len([]rune(alias))
		if perUse <= 2 {
			continue
		}
		// Resolve alias collisions: keep the higher-count command.
		if existing, taken := usedAlias[alias]; taken {
			if counts[existing] >= count {
				continue
			}
		}
		usedAlias[alias] = cmd
		out = append(out, AliasSuggestion{
			Alias:   alias,
			Command: cmd,
			Count:   count,
			Saved:   perUse * count,
			perUse:  perUse,
		})
	}

	// Drop superseded collisions (an alias may appear twice before resolution).
	final := out[:0]
	for _, s := range out {
		if usedAlias[s.Alias] == s.Command {
			final = append(final, s)
		}
	}

	sort.Slice(final, func(i, j int) bool {
		if final[i].Saved != final[j].Saved {
			return final[i].Saved > final[j].Saved
		}
		return final[i].Command < final[j].Command
	})
	if opts.TopN > 0 && len(final) > opts.TopN {
		final = final[:opts.TopN]
	}
	return final
}

// mnemonic builds a short alias from a command line: initials of each
// alphanumeric token, lowercased, capped at 5 characters.
func mnemonic(cmd string) string {
	fields := strings.Fields(cmd)
	var b strings.Builder
	for _, f := range fields {
		if i := strings.LastIndexByte(f, '/'); i != -1 && i+1 < len(f) {
			f = f[i+1:] // use basename of path-like tokens
		}
		r := firstAlnum(f)
		if r == 0 {
			continue
		}
		b.WriteRune(r)
		if b.Len() >= 5 {
			break
		}
	}
	return strings.ToLower(b.String())
}

func firstAlnum(s string) rune {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
	}
	return 0
}
