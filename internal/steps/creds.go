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
