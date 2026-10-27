package steps

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
	"github.com/eugenetaranov/yardo/internal/creds"
)

func credEnv(t *testing.T, x *fakeExec) *Env {
	t.Helper()
	e := testEnv(t, x)
	e.State = t.TempDir()
	e.Answers = &answers.Answers{}
	e.Secrets = creds.New()
	return e
}

func catalogStep(t *testing.T, id string, typ string) (string, catalog.Step) {
	t.Helper()
	c, err := catalog.Load(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range c.Item(id).Steps {
		if s.Type == typ {
			return id, s
		}
	}
	t.Fatalf("%s has no %s step", id, typ)
	return "", catalog.Step{}
}

func credOf(t *testing.T, id string) Step {
	t.Helper()
	id, s := catalogStep(t, id, "cred")
	st, err := New(id, s)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// agent.env: the same skeleton as the bash installer wrote, the typed key
// in it, 0600, recorded as created.
func TestCredEnv(t *testing.T) {
	e := credEnv(t, &fakeExec{})
	st := credOf(t, "agentkeys")
	e.Secrets.Cred["agentkeys OPENAI_API_KEY"] = "sk-1"
	apply(t, e, st)
	p := e.Path("~/.config/yardo/agent.env")
	want := `# Agent API keys, written by yardo and read by interactive shells.
# Fill in a key, then open a new shell. Not shared with services or cron.

# Anthropic API key (empty: log in with 'claude' instead): https://console.anthropic.com/settings/keys
ANTHROPIC_API_KEY=

# OpenAI API key (Codex): https://platform.openai.com/api-keys
OPENAI_API_KEY=sk-1

# Gemini API key: https://aistudio.google.com/apikey
GEMINI_API_KEY=
`
	if got := read(t, p); got != want {
		t.Errorf("agent.env:\n%s\nwant:\n%s", got, want)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode())
	}
	if !check(t, e, st) {
		t.Error("Check after Apply = false")
	}
	// A later run without typing keeps what the user filled in.
	put(t, p, strings.Replace(read(t, p), "GEMINI_API_KEY=", "GEMINI_API_KEY=mine", 1))
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
	e.Secrets = creds.New()
	if !check(t, e, st) {
		t.Error("nothing typed: Check = false")
	}
	if got := answers.ReadCreated(e.State); len(got) != 1 || got[0].Value != "/home/ada/.config/yardo/agent.env" {
		t.Errorf("created = %+v", got)
	}
}

func TestAgeKey(t *testing.T) {
	x := &fakeExec{reply: func(c Cmd) ([]byte, error) {
		if c.Name == "sh" && strings.Contains(c.Args[1], "age-keygen -o") {
			return nil, os.WriteFile(c.Args[3], []byte("# created\nAGE-SECRET-KEY-1GEN\n"), 0o600)
		}
		if c.Name == "age-keygen" {
			return []byte("age1pub\n"), nil
		}
		return nil, errors.New("unexpected")
	}}
	e := credEnv(t, x)
	var notes []string
	e.Note = func(s string) { notes = append(notes, s) }
	st := step(t, catalog.Step{Type: "go:agekey"})
	p := e.Path(ageKeys)
	// Later: a commented file to fill in.
	e.Secrets.AgeChoice = "later"
	apply(t, e, st)
	if got := read(t, p); !strings.HasPrefix(got, "# age key for sops") {
		t.Errorf("keys.txt = %q", got)
	}
	if !check(t, e, st) {
		t.Error("later: Check after Apply = false")
	}
	// Paste: the key goes in; the comment stays.
	e.Secrets.AgeChoice, e.Secrets.AgeKey = "paste", " AGE-SECRET-KEY-1PASTED \n"
	if check(t, e, st) {
		t.Error("paste: Check = true without a key")
	}
	apply(t, e, st)
	if got := read(t, p); !strings.HasSuffix(got, "\nAGE-SECRET-KEY-1PASTED\n") {
		t.Errorf("keys.txt = %q", got)
	}
	// Generate with a key there: nothing to do.
	e.Secrets.AgeChoice = "generate"
	if !check(t, e, st) {
		t.Error("generate with a key: Check = false")
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode())
	}
	// Generate on a fresh home.
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	apply(t, e, st)
	if !strings.Contains(read(t, p), "AGE-SECRET-KEY-1GEN") || len(notes) != 1 || !strings.Contains(notes[0], "age1pub") {
		t.Errorf("keys.txt = %q, notes %q", read(t, p), notes)
	}
}
