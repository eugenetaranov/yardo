package steps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/eugenetaranov/yardo/internal/catalog"
)

// Credentials: yardo never stores them. A cred step writes the
// tool's own file (0600) with what was typed this run; a skipped value
// stays empty, and ~/yardo.txt lists it under "Secrets to fill in".

func init() {
	RegisterGo("agekey", func(catalog.Step) Step { return ageKeyStep{} })
}

type credStep struct {
	id string
	s  catalog.Step
}

func (c credStep) Plan(*Env) []Action { return []Action{{"cred", c.s.To + " (0600)"}} }

// typedCred is what was typed for the step's item this run.
func (c credStep) typed(e *Env, key string) string {
	if e.Secrets == nil {
		return ""
	}
	return e.Secrets.Cred[c.id+" "+key]
}

// want is the file's content: the env file with what was typed this run.
func (c credStep) want(e *Env, cur []byte, exists bool) []byte {
	b := cur
	if !exists {
		b = c.skeleton()
	}
	for _, k := range c.s.Keys {
		if v := c.typed(e, k.Name); v != "" {
			b = EnvSet(b, k.Name, v)
		}
	}
	return b
}

// skeleton is a new env file: the header, then each key, empty, under its
// prompt and where to get it.
func (c credStep) skeleton() []byte {
	var b strings.Builder
	b.WriteString(strings.TrimRight(c.s.Header, "\n") + "\n")
	for _, k := range c.s.Keys {
		fmt.Fprintf(&b, "\n# %s: %s\n%s=\n", k.Prompt, k.How, k.Name)
	}
	return []byte(b.String())
}

// Missing: the keys still empty, but for those whose tool has its own
// login, or isn't installed.
func (c credStep) Missing(ctx context.Context, e *Env) []string {
	b, _ := os.ReadFile(e.Path(c.s.To))
	var out []string
	for _, k := range c.s.Keys {
		if k.LoggedIn != "" {
			if _, err := os.Stat(e.Path(k.LoggedIn)); err == nil {
				continue
			}
		}
		if k.When != "" && !has(ctx, e, k.When) {
			continue
		}
		if EnvGet(b, k.Name) == "" {
			out = append(out, c.s.To+": "+k.Name)
		}
	}
	return out
}

func (c credStep) Check(_ context.Context, e *Env) (bool, error) {
	p := e.Path(c.s.To)
	cur, err := os.ReadFile(p)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	want := c.want(e, cur, exists)
	if want == nil {
		return true, nil
	}
	if !exists || !bytes.Equal(cur, want) {
		return false, nil
	}
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().Perm() == 0o600, nil
}

func (c credStep) Apply(_ context.Context, e *Env) error {
	p := e.Path(c.s.To)
	cur, err := os.ReadFile(p)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	want := c.want(e, cur, exists)
	if want == nil {
		return nil
	}
	// A file yardo created is its own (forget removes it).
	if !exists {
		if err := e.record("file", filepath.Join(e.Home, strings.TrimPrefix(c.s.To, "~/"))); err != nil {
			return err
		}
	}
	if err := mkdirFor(e, p, c.s); err != nil {
		return err
	}
	if err := writeFile(p, want, 0o600); err != nil {
		return err
	}
	return e.chown(p, e.UID, e.GID)
}

// --- go:agekey ---------------------------------------------------------------

// ageKeyStep sets up ~/.config/sops/age/keys.txt (0600) as chosen this run:
// generate a key, paste one, or leave a commented file to fill in.
type ageKeyStep struct{}

const ageKeys = "~/.config/sops/age/keys.txt"

func (ageKeyStep) choice(e *Env) (string, string) {
	if e.Secrets == nil {
		return "", ""
	}
	return e.Secrets.AgeChoice, e.Secrets.AgeKey
}

func (ageKeyStep) Plan(*Env) []Action { return []Action{{"cred", ageKeys + " (0600)"}} }

// HasAgeKey reports whether an age keys file has a secret key line.
func HasAgeKey(b []byte) bool {
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "AGE-SECRET-KEY-") {
			return true
		}
	}
	return false
}

func (a ageKeyStep) Check(_ context.Context, e *Env) (bool, error) {
	p := e.Path(ageKeys)
	b, err := os.ReadFile(p)
	if err != nil {
		return false, nil
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		return false, nil
	}
	choice, _ := a.choice(e)
	return (choice != "generate" && choice != "paste") || HasAgeKey(b), nil
}

func (a ageKeyStep) Apply(ctx context.Context, e *Env) error {
	p := e.Path(ageKeys)
	home := filepath.Join(e.Home, strings.TrimPrefix(ageKeys, "~/"))
	cur, err := os.ReadFile(p)
	exists := err == nil
	if err := mkdirFor(e, p, catalog.Step{To: ageKeys}); err != nil {
		return err
	}
	choice, key := a.choice(e)
	if !HasAgeKey(cur) && (choice == "generate" || choice == "paste") {
		if !exists {
			if err := e.record("file", home); err != nil {
				return err
			}
		}
		switch choice {
		case "generate":
			_ = os.Remove(p)
			if _, err := e.run(ctx, Cmd{Name: "sh", Args: []string{"-c", `umask 077; age-keygen -o "$1"`, "_", p}, AsUser: true}); err != nil {
				return err
			}
			if pub, err := e.Exec.Run(ctx, Cmd{Name: "age-keygen", Args: []string{"-y", p}, AsUser: true}); err == nil {
				e.note("age: new key; its public key is " + strings.TrimSpace(string(pub)))
			}
		case "paste":
			if err := writeFile(p, append(cur, strings.TrimSpace(key)+"\n"...), 0o600); err != nil {
				return err
			}
		}
		exists = true
	}
	if !exists {
		if err := e.record("file", home); err != nil {
			return err
		}
		if err := writeFile(p, []byte("# age key for sops: paste a line AGE-SECRET-KEY-1… here, or run: age-keygen -o ~/.config/sops/age/keys.txt\n"), 0o600); err != nil {
			return err
		}
	}
	if err := os.Chmod(p, 0o600); err != nil {
		return err
	}
	return e.chown(p, e.UID, e.GID)
}
