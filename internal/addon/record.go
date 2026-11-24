package addon

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Version is yardo's, for the record (main sets it).
var Version = "dev"

// Approval is an add-on the user approved and yardo installed.
type Approval struct {
	Kind   string `toml:"kind"`
	Name   string `toml:"name"`
	Source string `toml:"source"`
	Pin    string `toml:"pin"`
	// Hash is the hash of the files installed (a skill) or of the spec.
	Hash string `toml:"hash"`
	// Findings are what the checks found, a line each; OK: the user typed
	// the name to approve what needed it.
	Findings []string  `toml:"findings,omitempty"`
	OK       bool      `toml:"typed_ok"`
	By       string    `toml:"approved_by"`
	At       time.Time `toml:"approved_at"`
	Yardo    string    `toml:"yardo"`
	// Tools is a remote server's tool list's hash, once it registered.
	Tools string `toml:"tools,omitempty"`
}

// recordFile is user's record (root's, 0644, in root's /var/lib/yardo).
func recordFile(user string) string { return filepath.Join(Base, "approved", user+".toml") }

// SpecsDir holds each user's approved MCP specs' list (beside the
// gateway's own settings, which its user reads).
var SpecsDir = "/etc/mcpjungle"

// SpecsFile lists user's approved MCP specs, "name hash [tools]" a line,
// for mcp-sync.
func SpecsFile(user string) string { return filepath.Join(SpecsDir, user+".approved") }

// Approvals are user's approved add-ons.
func Approvals(user string) ([]Approval, error) {
	if !validUser(user) {
		return nil, fmt.Errorf("not a user name: %q", user)
	}
	var rec struct {
		Addon []Approval `toml:"addon"`
	}
	if _, err := toml.DecodeFile(recordFile(user), &rec); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return rec.Addon, nil
}

// Approved is user's approval of kind name, if there is one.
func Approved(user, kind, name string) (Approval, bool) {
	as, _ := Approvals(user)
	for _, a := range as {
		if a.Kind == kind && a.Name == name {
			return a, true
		}
	}
	return Approval{}, false
}

// Approve records a (replacing one of the same kind and name).
func Approve(user string, a Approval) error {
	as, err := Approvals(user)
	if err != nil {
		return err
	}
	as = slices.DeleteFunc(as, func(x Approval) bool { return x.Kind == a.Kind && x.Name == a.Name })
	if a.At.IsZero() {
		a.At = time.Now().UTC().Truncate(time.Second)
	}
	if a.Yardo == "" {
		a.Yardo = Version
	}
	return writeRecord(user, append(as, a))
}

// Forget removes kind name's approval and approved copy (uninstalled,
// deleted).
func Forget(user, kind, name string) error {
	as, err := Approvals(user)
	if err != nil {
		return err
	}
	if validUser(user) && name != "" && !strings.ContainsAny(name, "/\x00") && name != "." && name != ".." {
		if err := os.RemoveAll(copyDir(user, kind, name)); err != nil {
			return err
		}
	}
	n := len(as)
	as = slices.DeleteFunc(as, func(x Approval) bool { return x.Kind == kind && x.Name == name })
	if len(as) == n {
		return nil
	}
	return writeRecord(user, as)
}

func writeRecord(user string, as []Approval) error {
	slices.SortFunc(as, func(a, b Approval) int {
		return strings.Compare(fmt.Sprintf("%s/%s", a.Kind, a.Name), fmt.Sprintf("%s/%s", b.Kind, b.Name))
	})
	d := filepath.Join(Base, "approved")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "# Add-ons from elsewhere that %s approved: written by yardo, not to edit.\n\n", user)
	if err := toml.NewEncoder(&b).Encode(struct {
		Addon []Approval `toml:"addon"`
	}{as}); err != nil {
		return err
	}
	var specs strings.Builder
	for _, a := range as {
		if a.Kind == "mcp" {
			fmt.Fprintln(&specs, strings.TrimSpace(fmt.Sprintf("%s %s %s", a.Name, a.Hash, a.Tools)))
		}
	}
	if err := atomic(recordFile(user), b.Bytes()); err != nil {
		return err
	}
	if err := os.MkdirAll(SpecsDir, 0o755); err != nil {
		return err
	}
	return atomic(SpecsFile(user), []byte(specs.String()))
}

// atomic writes p (root's, 0644) all at once.
func atomic(p string, b []byte) error {
	t := p + ".new"
	if err := os.WriteFile(t, b, 0o644); err != nil {
		return err
	}
	if err := os.Chmod(t, 0o644); err != nil {
		return err
	}
	return os.Rename(t, p)
}

// Lines are a report's findings as the record keeps them.
func (r Report) Lines() []string {
	var out []string
	for _, f := range r.Findings {
		out = append(out, strings.TrimSpace(fmt.Sprintf("%s %s %s %s %s", f.Level, f.Layer, f.Rule, f.Where(), f.Text)))
	}
	return out
}
