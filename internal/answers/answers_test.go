package answers

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestReadMissingIsEmpty(t *testing.T) {
	a, notes, err := Read(t.TempDir())
	if err != nil || len(notes) != 0 || len(a.Picks) != 0 || a.Version != Version {
		t.Fatalf("a=%+v notes=%v err=%v", a, notes, err)
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	yes := true
	in := &Answers{User: "ada", Picks: []string{"lazygit", "zsh"}, MiseExtra: []string{"starship"}, MCPListen: "127.0.0.1",
		MCPServers: []string{"github"}, WireAgents: &yes, Git: Git{"Ada L", "ada@example.com"}, TZ: "UTC",
		GitHub: GitHost{Mode: "agent"}, NPM: NPM{"@acme", "https://npm.pkg.github.com"}, Registries: []string{"ghcr.io"}}
	if err := Write(state, in); err != nil {
		t.Fatal(err)
	}
	got, _, err := Read(state)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round trip:\n got %+v\nwant %+v", got, in)
	}
	for p, want := range map[string]os.FileMode{state: 0o700, filepath.Join(state, File): 0o600} {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s: mode %v, want %v (%v)", p, fi.Mode().Perm(), want, err)
		}
	}
}

func TestMigratePicks(t *testing.T) {
	got, notes := MigratePicks([]string{"cli", "shell", "kubectl", "helm", "chrony", "zsh", "krew", "tmuxcfg", "agekey"})
	if want := []string{"zsh", "k8s", "tmux", "sops"}; !reflect.DeepEqual(got, want) {
		t.Errorf("picks = %v, want %v", got, want)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "clock sync") {
		t.Errorf("notes = %v", notes)
	}
}

func TestBadVersion(t *testing.T) {
	state := t.TempDir()
	if err := os.WriteFile(filepath.Join(state, File), []byte(`{"version": 7}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Read(state); err == nil || !strings.Contains(err.Error(), "version 7") {
		t.Errorf("err = %v", err)
	}
}

func TestLegacyConverted(t *testing.T) {
	state := t.TempDir()
	legacy := "A_USER=ubuntu\nA_PICKS=cli\\ shell\nA_GIT_NAME=$'Ada \\'L\\''\nA_WIRE_CLAUDE=yes\nA_TZ='Europe/Berlin'\nNOT_OURS=1\n"
	if err := os.WriteFile(filepath.Join(state, Legacy), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	a, notes, err := Read(state)
	if err != nil {
		t.Fatal(err)
	}
	if a.User != "ubuntu" || a.Git.Name != "Ada 'L'" || a.TZ != "Europe/Berlin" || a.WireAgents == nil || !*a.WireAgents {
		t.Errorf("a = %+v", a)
	}
	if want := []string{"zsh"}; !reflect.DeepEqual(a.Picks, want) {
		t.Errorf("picks = %v, want %v", a.Picks, want)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "NOT_OURS") {
		t.Errorf("notes = %v", notes)
	}
	if _, err := os.Stat(filepath.Join(state, Legacy+".old")); err != nil {
		t.Error("legacy file not kept as answers.old")
	}
	if _, err := os.Stat(filepath.Join(state, File)); err != nil {
		t.Error("answers.json not written")
	}
}

func TestUnquoteRejectsCode(t *testing.T) {
	for _, v := range []string{"$(touch /tmp/x)", "`id`", "a;b", `"$HOME"`, "a b", "'open", "x|y"} {
		if _, err := Unquote(v); !errors.Is(err, ErrUnsafe) {
			t.Errorf("Unquote(%q) err = %v, want ErrUnsafe", v, err)
		}
	}
}

// Values bash's printf %q wrote decode to the original, in the C and UTF-8
// locales (which quote non-ASCII differently).
func TestUnquoteBashRoundTrip(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	values := []string{"plain", "with space", "it's", `back\slash`, "tab\there", "new\nline", "Zoë Çelik — ✓", "$(id)", "`x`", "a;b|c&d", ""}
	for _, loc := range []string{"C", "en_US.UTF-8"} {
		for _, v := range values {
			cmd := exec.Command(bash, "-c", `printf '%q' "$V"`)
			cmd.Env = append(os.Environ(), "V="+v, "LC_ALL="+loc)
			out, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			got, err := Unquote(string(out))
			if err != nil || got != v {
				t.Errorf("%s: %q → %q → %q (%v)", loc, v, out, got, err)
			}
		}
	}
}

// The bash installer (picker.py) reads what Go writes, and the other way
// round, while both exist.
func TestSharedWithPickerPy(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("no python3")
	}
	picker := "../assets/picker.py"
	if _, err := os.Stat(picker); err != nil {
		t.Skip("picker.py gone")
	}
	state := t.TempDir()
	if err := Write(state, &Answers{User: "ada", Picks: []string{"lazygit"}, GitHub: GitHost{Mode: "token"}, NPM: NPM{Scope: "@acme"}}); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("python3", picker, "answers", "read", state).Output()
	if err != nil {
		t.Fatal(err)
	}
	vals := map[string]string{}
	parts := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	for i := 0; i+1 < len(parts); i += 2 {
		vals[parts[i]] = parts[i+1]
	}
	if vals["A_USER"] != "ada" || vals["A_PICKS"] != "lazygit" || vals["A_GH_MODE"] != "token" || vals["A_NPM_SCOPE"] != "@acme" {
		t.Errorf("picker.py read %v", vals)
	}

	cmd := exec.Command("python3", picker, "answers", "write", state)
	cmd.Stdin = strings.NewReader("A_USER\x00bob\x00A_PICKS\x00zsh go\x00A_REGISTRIES\x00ghcr.io\x00")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	a, _, err := Read(state)
	if err != nil {
		t.Fatal(err)
	}
	if a.User != "bob" || !reflect.DeepEqual(a.Picks, []string{"zsh", "go"}) || !reflect.DeepEqual(a.Registries, []string{"ghcr.io"}) {
		t.Errorf("Go read %+v", a)
	}
	var raw map[string]any
	b, _ := os.ReadFile(filepath.Join(state, File))
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"version", "user", "picks", "github", "npm", "registries"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("picker.py's file has no %q", k)
		}
	}
}

func TestMerge(t *testing.T) {
	if got := Merge([]string{"a", "b"}, []string{"b", "c", ""}); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("Merge = %v", got)
	}
}

func TestCheckRepo(t *testing.T) {
	for u, ok := range map[string]bool{"https://github.com/ada/dotfiles": true, "git@github.com:ada/dotfiles.git": true, "ssh://git@host/x": true,
		"--upload-pack=x": false, "https://x/y;rm": false, "file:///etc": false, "/tmp/x": false, "": false} {
		if err := CheckRepo(u); (err == nil) != ok {
			t.Errorf("%q: err = %v", u, err)
		}
	}
}

// The old gitkey item becomes the host items in the "sent" mode; other
// hosts keep it. glab is the GitLab item; the sshkey item is gone.
func TestMigrateGitKey(t *testing.T) {
	state := t.TempDir()
	if err := Write(state, &Answers{User: "ada", Picks: []string{"zsh", "gitkey", "glab", "sshkey"},
		GitKeyHosts: []string{"github.com", "gitlab.com", "git.acme.io:2222"}}); err != nil {
		t.Fatal(err)
	}
	a, notes, err := Read(state)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"github", "gitkey", "gitlab", "zsh"}; !slices.Equal(slices.Sorted(slices.Values(a.Picks)), want) {
		t.Errorf("picks = %v, want %v", a.Picks, want)
	}
	if a.GitHub.Mode != "sent" || a.GitLab.Mode != "sent" || a.Bitbucket.Mode != "" || !slices.Equal(a.GitKeyHosts, []string{"git.acme.io:2222"}) {
		t.Errorf("github %+v gitlab %+v bitbucket %+v hosts %v", a.GitHub, a.GitLab, a.Bitbucket, a.GitKeyHosts)
	}
	if len(notes) != 2 {
		t.Errorf("notes = %q", notes)
	}
	if got := a.SentKeyHosts(); !slices.Equal(got, []string{"github.com", "gitlab.com", "git.acme.io:2222"}) {
		t.Errorf("SentKeyHosts = %v", got)
	}
	// The old default: no hosts is GitHub.
	if err := Write(state, &Answers{User: "ada", Picks: []string{"gitkey"}, GitHub: GitHost{Mode: "agent"}}); err != nil {
		t.Fatal(err)
	}
	if a, _, _ = Read(state); !slices.Equal(a.Picks, []string{"github"}) || a.GitHub.Mode != "sent" {
		t.Errorf("picks %v github %+v", a.Picks, a.GitHub)
	}
}

func TestUnpick(t *testing.T) {
	a := &Answers{Picks: []string{"zsh", "docker", "tmux"}, Hand: []string{"docker"}, Layers: map[string][]string{"team": {"docker"}}}
	a.Unpick([]string{"docker", "nope"})
	if !reflect.DeepEqual(a.Picks, []string{"zsh", "tmux"}) || len(a.Hand) != 0 {
		t.Errorf("Picks = %v, Hand = %v", a.Picks, a.Hand)
	}
	if !reflect.DeepEqual(a.Layers["team"], []string{"docker"}) {
		t.Errorf("a profile's layer is its own: %v", a.Layers)
	}
}
