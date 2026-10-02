package analyze

import "testing"

func TestAnalyzeCountsAndNormalizes(t *testing.T) {
	cmds := []string{
		"git   status", // extra spaces normalize to "git status"
		"git status",
		"ls -la",
		"ls -la",
		"ls -la",
	}
	opts := DefaultOptions()
	opts.Ignore = []string{"ls"}
	rep := Analyze(cmds, opts)

	if rep.TotalCommands != 5 {
		t.Errorf("total: got %d want 5", rep.TotalCommands)
	}
	if rep.UniqueCommands != 2 {
		t.Errorf("unique: got %d want 2", rep.UniqueCommands)
	}
	// "ls" is ignored for program stats, so only git should appear.
	if len(rep.TopPrograms) != 1 || rep.TopPrograms[0].Program != "git" {
		t.Errorf("top programs: got %+v", rep.TopPrograms)
	}
	if rep.TopPrograms[0].Count != 2 {
		t.Errorf("git count: got %d want 2", rep.TopPrograms[0].Count)
	}
}

func TestFirstTokenSkipsSudoAndEnv(t *testing.T) {
	cases := map[string]string{
		"sudo apt update":    "apt",
		"FOO=bar make build": "make",
		"git push":           "git",
		"./configure":        "./configure",
		"":                   "",
	}
	for in, want := range cases {
		if got := firstToken(in); got != want {
			t.Errorf("firstToken(%q): got %q want %q", in, got, want)
		}
	}
}

func TestSuggestAliasesRanksBySaved(t *testing.T) {
	counts := map[string]int{
		"git status":                10, // alias gs, saves (10-2)*10=80
		"docker compose up --build": 4,  // longer, fewer uses
		"ls":                        50, // too short, no alias
	}
	opts := DefaultOptions()
	opts.MinCount = 2
	out := suggestAliases(counts, opts)
	if len(out) < 2 {
		t.Fatalf("expected >=2 suggestions, got %d: %+v", len(out), out)
	}
	// Verify sorted by Saved descending.
	for i := 1; i < len(out); i++ {
		if out[i-1].Saved < out[i].Saved {
			t.Errorf("not sorted by saved: %+v", out)
		}
	}
	// "ls" must never produce an alias (too short).
	for _, s := range out {
		if s.Command == "ls" {
			t.Errorf("ls should not get an alias: %+v", s)
		}
	}
}

func TestAliasCollisionKeepsHigherCount(t *testing.T) {
	// Both normalize to mnemonic "gc"; higher count wins.
	counts := map[string]int{
		"git commit": 20,
		"go clean":   5,
	}
	opts := DefaultOptions()
	opts.MinCount = 2
	out := suggestAliases(counts, opts)
	seen := map[string]string{}
	for _, s := range out {
		if prev, dup := seen[s.Alias]; dup {
			t.Fatalf("duplicate alias %q for %q and %q", s.Alias, prev, s.Command)
		}
		seen[s.Alias] = s.Command
	}
	if cmd, ok := seen["gc"]; ok && cmd != "git commit" {
		t.Errorf("collision winner: got %q want \"git commit\"", cmd)
	}
}
