package history

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseBash(t *testing.T) {
	in := "git status\n#1700000000\nls -la\n\n   \ngit commit -m hi\n"
	got, err := ParseReader(Bash, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"git status", "ls -la", "git commit -m hi"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestParseZshExtendedAndMultiline(t *testing.T) {
	in := ": 1700000000:0;git status\n" +
		": 1700000001:5;echo one\\\n" +
		"two\n" +
		"plain line\n"
	got, err := ParseReader(Zsh, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"git status", "echo one\ntwo", "plain line"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestParseZshMalformedMetaKeepsLine(t *testing.T) {
	// A line that starts like meta but has no semicolon must not be dropped.
	in := ": broken meta no semi\n"
	got, _ := ParseReader(Zsh, strings.NewReader(in))
	want := []string{": broken meta no semi"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestParseFish(t *testing.T) {
	in := "- cmd: git status\n  when: 1700000000\n" +
		"- cmd: echo line1\\nline2\n  when: 1700000001\n" +
		"  paths:\n    - foo\n"
	got, err := ParseReader(Fish, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"git status", "echo line1\nline2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDefaultPath(t *testing.T) {
	home := "/home/u"
	cases := map[Shell]string{
		Bash: filepath.Join(home, ".bash_history"),
		Zsh:  filepath.Join(home, ".zsh_history"),
		Fish: filepath.Join(home, ".local", "share", "fish", "fish_history"),
	}
	for sh, want := range cases {
		if got := DefaultPath(sh, home); got != want {
			t.Errorf("%s: got %q want %q", sh, got, want)
		}
	}
	if got := DefaultPath(Bash, ""); got != "" {
		t.Errorf("empty home should yield empty path, got %q", got)
	}
}

func TestLocateExplicitPathInfersShell(t *testing.T) {
	src, ok := Locate("/tmp/whatever/fish_history", "", "/home/u")
	if !ok {
		t.Fatal("expected ok")
	}
	if src.Shell != Fish {
		t.Errorf("got shell %q want fish", src.Shell)
	}
}
