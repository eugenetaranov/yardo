// Package addon is the way in for skills and MCP servers from outside
// yardo's catalog: each is fetched at a fixed version (a commit, an image
// digest) into a holding area only root can read, checked (basic rules,
// then skill-veil), shown to the user with what was found, and installed
// only with their approval, exactly as checked. Approvals are recorded.
package addon

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Base is where yardo keeps its state; tests point it elsewhere.
var Base = "/var/lib/yardo"

// Finding is one thing a check found.
type Finding struct {
	// Layer is who found it: basic (yardo's rules), skill-veil, claude.
	Layer string `json:"layer"`
	Rule  string `json:"rule"`
	// Level is block (can't be installed as it is), ok (needs the user's
	// OK, typed) or note.
	Level string `json:"level"`
	File  string `json:"file,omitempty"`
	Line  int    `json:"line,omitempty"`
	// Text says what was found; Next what to do about it.
	Text string `json:"text"`
	Next string `json:"next,omitempty"`
	// Fix is a fix yardo can make (remove:<file>, drop-key:<key>,
	// drop-opt:<option>), after which it checks again.
	Fix string `json:"fix,omitempty"`
	// Role is what its file is to the agent (see Roles); Kind what the
	// finding is, in a few words ("Contacts the network"); Why says why
	// it matters here, a line each; Look: it's worth the user's eyes
	// (every block and ok is; a note may be).
	Role string   `json:"role,omitempty"`
	Kind string   `json:"kind,omitempty"`
	Why  []string `json:"why,omitempty"`
	Look bool     `json:"look,omitempty"`
	// Also are the other layers that found the same file and line.
	Also []string `json:"also,omitempty"`
	// More is how many more places the same finding is in that file.
	More int `json:"more,omitempty"`
	// ID names it across reports (rule, file, what it matched; not the
	// line), so an update can tell what's new since an approval.
	ID string `json:"id,omitempty"`
}

// Roles a file has to the agent, most weight first: instructions (it
// reads them: SKILL.md, what SKILL.md links to), script (SKILL.md tells
// it to run them), code (its own, imported by those), vendored
// (third-party, bundled, generated, minified), asset, doc, license.
var Roles = []string{"instructions", "script", "code", "vendored", "asset", "doc", "license"}

// Cap is something the add-on can do, found in its own code: Kind is
// exec, network, env, home, secrets, writes, agent, grants, hooks,
// install; What says it ("node, git, open", "tt-a1i.github.io");
// Where are file:line places, first few; Count how many places in all;
// Look: worth the user's eyes (a host it calls, writes outside its
// folder, the agent's rules or settings).
type Cap struct {
	Kind  string   `json:"kind"`
	What  string   `json:"what"`
	Where []string `json:"where,omitempty"`
	Count int      `json:"count,omitempty"`
	Look  bool     `json:"look,omitempty"`
}

// Coverage is what each layer read: Files of All; Skipped and Unread
// say what wasn't, in words ("389 licences, assets, vendored", "14 own
// files past the budget"); Looks which files worth a look it read, and
// how (Claude's); Off why a layer didn't run ("not installed").
type Coverage struct {
	Layer   string `json:"layer"`
	Files   int    `json:"files"`
	All     int    `json:"all"`
	Skipped string `json:"skipped,omitempty"`
	Unread  string `json:"unread,omitempty"`
	Looks   string `json:"looks,omitempty"`
	Off     string `json:"off,omitempty"`
}

// Where is the finding's file:line, or file, or "".
func (f Finding) Where() string {
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	return f.File
}

// Report is what the checks found in a held add-on's files (Hash).
type Report struct {
	Hash     string    `json:"hash"`
	At       time.Time `json:"at"`
	Findings []Finding `json:"findings"`
	// AI is the model Claude read it with ("" when it didn't).
	AI string `json:"ai,omitempty"`
	// Changes are an update's, from the approved copy.
	Changes []Change `json:"changes,omitempty"`
	// Rules is the version of yardo's rules it was made by.
	Rules int `json:"rules,omitempty"`
	// Caps is what it can do; Coverage what each layer read; Files each
	// file's role (path → role).
	Caps     []Cap             `json:"caps,omitempty"`
	Coverage []Coverage        `json:"coverage,omitempty"`
	Files    map[string]string `json:"files,omitempty"`
	// Since: an update's report, against the approved one: Caps gained
	// (+) and lost (−), and the IDs of findings the user OK'd then.
	Gained   []Cap    `json:"gained,omitempty"`
	Lost     []Cap    `json:"lost,omitempty"`
	Accepted []string `json:"accepted,omitempty"`
}

// Looks are the findings worth the user's eyes, in report order: not
// those the user OK'd in the version approved.
func (r Report) Looks() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Look || f.Level == "block" || f.Level == "ok" && !slices.Contains(r.Accepted, f.ID) {
			out = append(out, f)
		}
	}
	return out
}

// Rules is the version of yardo's rules (Basic): a report made by others
// is checked again, as what blocked then may not now, or the reverse.
// Raise it when a rule changes what it finds.
const Rules = 4

// Current: the report was made, by these rules.
func (r Report) Current() bool { return r.Hash != "" && r.Rules == Rules }

// Verdict is the worst of the findings: block, ok (needs the user's OK),
// note, or clean.
func (r Report) Verdict() string {
	v := "clean"
	for _, f := range r.Findings {
		if rank(f.Level) > rank(v) {
			v = f.Level
		}
	}
	return v
}

func rank(level string) int {
	return map[string]int{"note": 1, "ok": 2, "block": 3}[level]
}

// Sort puts the findings worst first, then by layer, file and line.
func (r *Report) Sort() {
	slices.SortStableFunc(r.Findings, func(a, b Finding) int {
		if d := rank(b.Level) - rank(a.Level); d != 0 {
			return d
		}
		if a.File != b.File {
			return strings.Compare(a.File, b.File)
		}
		return a.Line - b.Line
	})
}

// Held is an add-on in the holding area.
type Held struct {
	ID   string `json:"id"`
	User string `json:"user"`
	// Kind is skill or mcp.
	Kind string `json:"kind"`
	Name string `json:"name"`
	// Source is what the user gave (acme/skills/a@main, an image, a URL);
	// Pin what it was fixed to: a commit, an image digest, the URL.
	Source string `json:"source"`
	Pin    string `json:"pin"`
	// Odd are entries of the source that weren't regular files (links,
	// devices): never fetched, and a reason not to install as it is.
	Odd []string `json:"odd,omitempty"`
	// Image: an MCP server from a Docker image (Pin is its digest);
	// ImageUser the user it runs as ("" is root).
	Image     bool   `json:"image,omitempty"`
	ImageUser string `json:"image_user,omitempty"`
	// Pulled: yardo pulled the image for this, and it wasn't here before:
	// declined, it's removed (one the user had stays).
	Pulled bool `json:"pulled,omitempty"`
	// Existing: a skill that was in a skills folder already, held off
	// while it's checked (or a server, checked again as it is).
	Existing bool `json:"existing,omitempty"`
	// Update: a newer version of one approved (its approved copy is what
	// it's compared with).
	Update bool      `json:"update,omitempty"`
	Added  time.Time `json:"added"`
	Report Report    `json:"report"`
	// Found are the findings as the layers found them, before those on
	// one line were merged: Check again carefully replaces Claude's.
	Found []Finding `json:"found,omitempty"`
}

// dir is the holding area of user (root's, 0700).
func dir(user string) string { return filepath.Join(Base, "quarantine", user) }

func (h *Held) dir() string { return filepath.Join(dir(h.User), h.ID) }

// Path is file's place on disk among h's held files, to open it: "" when
// file isn't a path inside them; file "" is their folder.
func (h *Held) Path(file string) string {
	switch {
	case !validUser(h.User) || !validUser(h.ID):
		return ""
	case file == "":
		return filepath.Join(h.dir(), "files")
	case !cleanRel(file):
		return ""
	}
	return filepath.Join(h.dir(), "files", filepath.FromSlash(file))
}

// Hold puts files in a new held item for h (its ID is made here).
func Hold(h *Held, files map[string][]byte) error {
	if !validUser(h.User) {
		return fmt.Errorf("not a user name: %q", h.User)
	}
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	h.ID = hex.EncodeToString(b)
	if h.Added.IsZero() {
		h.Added = time.Now().UTC()
	}
	for _, d := range []string{filepath.Join(Base, "quarantine"), dir(h.User), h.dir()} {
		if err := os.Mkdir(d, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		if err := os.Chmod(d, 0o700); err != nil {
			return err
		}
	}
	if err := h.setFiles(files); err != nil {
		_ = os.RemoveAll(h.dir())
		return err
	}
	if err := h.Save(); err != nil {
		return err
	}
	// It replaces one of its kind and name held before (fetched again).
	held, err := List(h.User)
	if err != nil {
		return err
	}
	for _, o := range held {
		if o.ID != h.ID && o.Kind == h.Kind && o.Name == h.Name {
			if err := o.Remove(); err != nil {
				return err
			}
		}
	}
	return nil
}

func validUser(u string) bool {
	return u != "" && !strings.ContainsAny(u, "/\x00") && u != "." && u != ".."
}

// setFiles replaces the held files with files (each 0600, never run).
func (h *Held) setFiles(files map[string][]byte) error {
	r, err := os.OpenRoot(h.dir())
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	if err := r.RemoveAll("files"); err != nil {
		return err
	}
	if err := r.Mkdir("files", 0o700); err != nil {
		return err
	}
	for rel, b := range files {
		if !cleanRel(rel) {
			return fmt.Errorf("%s: not a path inside the add-on", rel)
		}
		p := path.Join("files", rel)
		if err := r.MkdirAll(path.Dir(p), 0o700); err != nil {
			return err
		}
		if err := r.WriteFile(p, b, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// cleanRel: rel is a relative path that stays inside its folder.
func cleanRel(rel string) bool {
	return rel != "" && !path.IsAbs(rel) && path.Clean(rel) == rel && rel != "." && !strings.HasPrefix(rel, "../") && rel != ".."
}

// Save writes h's meta (with its report).
func (h *Held) Save() error {
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	t := filepath.Join(h.dir(), "meta.json.new")
	if err := os.WriteFile(t, b, 0o600); err != nil {
		return err
	}
	return os.Rename(t, filepath.Join(h.dir(), "meta.json"))
}

// Files are h's held files, path → content.
func (h *Held) Files() (map[string][]byte, error) {
	r, err := os.OpenRoot(filepath.Join(h.dir(), "files"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	out := map[string][]byte{}
	err = fs.WalkDir(r.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s: not a file", p)
		}
		b, err := r.ReadFile(p)
		out[p] = b
		return err
	})
	return out, err
}

// Hash is the hash of files: their paths and contents, in order (the
// skills step's treeHash).
func Hash(files map[string][]byte) string {
	h := sha256.New()
	for _, n := range slices.Sorted(maps.Keys(files)) {
		h.Write([]byte(n + "\x00"))
		h.Write(files[n])
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// List are user's held add-ons, oldest first.
func List(user string) ([]*Held, error) {
	ents, err := os.ReadDir(dir(user))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Held
	for _, e := range ents {
		if h, err := Load(user, e.Name()); err == nil {
			out = append(out, h)
		}
	}
	slices.SortFunc(out, func(a, b *Held) int { return a.Added.Compare(b.Added) })
	return out, nil
}

// Load reads user's held add-on id.
func Load(user, id string) (*Held, error) {
	if !validUser(user) || !validUser(id) {
		return nil, fmt.Errorf("no held add-on %q", id)
	}
	b, err := os.ReadFile(filepath.Join(dir(user), id, "meta.json"))
	if err != nil {
		return nil, err
	}
	var h Held
	if err := json.Unmarshal(b, &h); err != nil {
		return nil, err
	}
	h.ID, h.User = id, user
	return &h, nil
}

// Remove deletes h from the holding area: nothing of it is left.
func (h *Held) Remove() error { return os.RemoveAll(h.dir()) }
