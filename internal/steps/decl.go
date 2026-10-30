package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
)

// --- apt ---------------------------------------------------------------------

type aptStep struct{ s catalog.Step }

func (a aptStep) Plan(e *Env) []Action {
	pkgs := a.Packages(e)
	for i, p := range pkgs {
		if v := e.lockedApt(p); v != "" {
			pkgs[i] = p + "=" + v
		}
	}
	return []Action{{"apt", strings.Join(pkgs, " ")}}
}

func (a aptStep) Check(ctx context.Context, e *Env) (bool, error) {
	missing, err := MissingPackages(ctx, e, a.Packages(e))
	return len(missing) == 0, err
}

func (a aptStep) Apply(ctx context.Context, e *Env) error { return AptInstall(ctx, e, a.Packages(e)) }

// Packages returns the apt step's packages, with {{kernel}} filled in
// (the runner batches them).
func (a aptStep) Packages(e *Env) []string {
	out := make([]string, len(a.s.Packages))
	for i, p := range a.s.Packages {
		out[i] = strings.ReplaceAll(p, "{{kernel}}", e.Kernel)
	}
	return out
}

// MissingPackages returns the packages that aren't installed.
func MissingPackages(ctx context.Context, e *Env, pkgs []string) ([]string, error) {
	if c := e.checks; c != nil {
		if c.debs == nil {
			// Every package's status at once, instead of one dpkg-query each.
			c.debs = map[string]bool{}
			out, _ := e.Exec.Run(ctx, Cmd{Name: "dpkg-query", Args: []string{"-W", "-f=${Package}\t${Status}\n"}})
			for _, l := range strings.Split(string(out), "\n") {
				if name, status, ok := strings.Cut(l, "\t"); ok && strings.Contains(status, "install ok installed") {
					c.debs[name] = true
				}
			}
		}
		var missing []string
		for _, p := range pkgs {
			if !c.debs[strings.SplitN(p, ":", 2)[0]] {
				missing = append(missing, p)
			}
		}
		return missing, nil
	}
	var missing []string
	for _, p := range pkgs {
		out, err := e.Exec.Run(ctx, Cmd{Name: "dpkg-query", Args: []string{"-W", "-f=${Status}", p}})
		if err != nil || !strings.Contains(string(out), "install ok installed") {
			missing = append(missing, p)
		}
	}
	return missing, nil
}

// AptOpts are apt-get's options for unattended installs: wait for another
// apt's lock, and keep existing config files instead of asking.
var AptOpts = []string{"-q", "-y", "-o", "DPkg::Lock::Timeout=900",
	"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold"}

// AptEnv keeps apt quiet: no questions, and needrestart lists instead of
// restarting (a docker restart mid-run would kill MCP containers).
var AptEnv = []string{"DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_MODE=l"}

// AptInstall installs pkgs, at their locked versions while the archive
// still has them (otherwise the current one, with a note).
func AptInstall(ctx context.Context, e *Env, pkgs []string) error {
	if len(pkgs) == 0 {
		return nil
	}
	args := append(append([]string{}, AptOpts...), "install")
	for _, p := range pkgs {
		v := e.lockedApt(p)
		switch {
		case v == "":
			args = append(args, p)
		case aptHasVersion(ctx, e, p, v):
			args = append(args, p+"="+v)
		default:
			e.note(fmt.Sprintf("apt: %s %s (locked) is no longer in the archive; installed the current version", p, v))
			args = append(args, p)
		}
	}
	_, err := e.run(ctx, Cmd{Name: "apt-get", Args: args, Env: AptEnv})
	return err
}

// aptHasVersion reports whether the package lists have pkg at version v.
func aptHasVersion(ctx context.Context, e *Env, pkg, v string) bool {
	out, err := e.Exec.Run(ctx, Cmd{Name: "apt-cache", Args: []string{"show", "--no-all-versions", pkg + "=" + v}})
	return err == nil && strings.Contains(string(out), "Version: "+v)
}

func (e *Env) lockedApt(pkg string) string {
	if e.Lock == nil {
		return ""
	}
	return e.Lock.Apt[pkg]
}

// AptUpdate refreshes the package lists. DPkg::Lock::Timeout covers
// installs, not update's lists lock, which cloud-init or apt-daily can
// hold right after boot: it retries while another apt holds it, for up
// to 15 minutes.
func AptUpdate(ctx context.Context, e *Env) error {
	for tries := 0; ; tries++ {
		out, err := e.run(ctx, Cmd{Name: "apt-get", Args: append(append([]string{}, AptOpts...), "update"), Env: AptEnv})
		if err == nil || !strings.Contains(string(out), "Could not get lock") || tries >= 180 {
			return err
		}
		if e.Log != nil && tries == 0 {
			fmt.Fprintln(e.Log, "Waiting for another apt process to finish…")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(AptRetryDelay):
		}
	}
}

// AptRetryDelay is the wait between apt-get update tries (tests shorten it).
var AptRetryDelay = 5 * time.Second

// --- mise --------------------------------------------------------------------

type miseStep struct{ s catalog.Step }

func (m miseStep) Plan(e *Env) []Action {
	return []Action{{"mise", strings.Join(LockedTools(e, m.s.Tools), " ")}}
}

// LockedTools returns tools with each locked one's version replaced by
// the lock's.
func LockedTools(e *Env, tools []string) []string {
	if e.Lock == nil || len(e.Lock.Mise) == 0 {
		return tools
	}
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = t
		if v := e.Lock.Mise[MiseName(t)]; v != "" {
			if j := strings.LastIndexByte(t, '@'); j > 0 && j > strings.LastIndexByte(t, ']') {
				t = t[:j]
			}
			out[i] = t + "@" + v
		}
	}
	return out
}

// MiseName is the name `mise ls` reports for a tool spec: no version, no
// [options].
func MiseName(spec string) string {
	if i := strings.IndexByte(spec, '['); i >= 0 {
		spec = spec[:i]
	}
	if i := strings.LastIndexByte(spec, '@'); i > 0 {
		spec = spec[:i]
	}
	return spec
}

func (m miseStep) Check(ctx context.Context, e *Env) (bool, error) {
	if e.Reinstall && slices.ContainsFunc(m.s.Tools, fromGit) {
		return false, nil
	}
	return MiseHas(ctx, e, m.s.Tools)
}

// fromGit reports whether a mise tool installs from a git repo
// (pipx:git+https://…): its version is just "latest", whatever the repo
// is at now.
func fromGit(tool string) bool { return strings.Contains(MiseName(tool), "git+") }

// MiseHas reports whether every tool is in the user's global mise config,
// at its locked version when there is one.
func MiseHas(ctx context.Context, e *Env, tools []string) (bool, error) {
	have, ok := miseList(ctx, e)
	if !ok {
		return false, nil
	}
	for _, t := range tools {
		name := MiseName(t)
		vs, ok := have[name]
		if !ok {
			return false, nil
		}
		if e.Lock != nil && e.Lock.Mise[name] != "" && !slices.ContainsFunc(vs, func(v miseVersion) bool { return v.Version == e.Lock.Mise[name] }) {
			return false, nil
		}
	}
	return true, nil
}

type miseVersion struct {
	Version   string `json:"version"`
	Installed bool   `json:"installed"`
	Active    bool   `json:"active"`
}

// miseList is `mise ls --global --json`: tool → its versions.
func miseList(ctx context.Context, e *Env) (map[string][]miseVersion, bool) {
	if c := e.checks; c != nil {
		if !c.miseSet {
			c.mise, c.miseOK = miseListNow(ctx, e)
			c.miseSet = true
		}
		return c.mise, c.miseOK
	}
	return miseListNow(ctx, e)
}

func miseListNow(ctx context.Context, e *Env) (map[string][]miseVersion, bool) {
	out, err := e.Exec.Run(ctx, Cmd{Name: "mise", Args: []string{"ls", "--global", "--json"}, AsUser: true})
	if err != nil {
		return nil, false
	}
	var have map[string][]miseVersion
	if json.Unmarshal(out, &have) != nil {
		return nil, false
	}
	return have, true
}

// Versions reads the installed versions of mise tools (the active
// one) and apt packages, for `export --lock`. missing lists what isn't
// installed (so isn't locked).
func Versions(ctx context.Context, e *Env, tools, pkgs []string) (l *answers.Lock, missing []string) {
	l = &answers.Lock{Mise: map[string]string{}, Apt: map[string]string{}}
	have, _ := miseList(ctx, e)
	for _, t := range tools {
		name := MiseName(t)
		v := ""
		for _, x := range have[name] {
			if x.Installed && (v == "" || x.Active) {
				v = x.Version
			}
		}
		if v == "" {
			missing = append(missing, name)
			continue
		}
		l.Mise[name] = v
	}
	for _, p := range pkgs {
		out, err := e.Exec.Run(ctx, Cmd{Name: "dpkg-query", Args: []string{"-W", "-f=${Status}\t${Version}", p}})
		st, v, _ := strings.Cut(string(out), "\t")
		if err != nil || !strings.Contains(st, "install ok installed") || v == "" {
			missing = append(missing, p)
			continue
		}
		l.Apt[p] = v
	}
	return l, missing
}

func (m miseStep) Apply(ctx context.Context, e *Env) error {
	if err := MiseUse(ctx, e, m.s.Tools); err != nil || !e.Reinstall {
		return err
	}
	for _, t := range m.s.Tools {
		if fromGit(t) {
			if _, err := e.run(ctx, Cmd{Name: "mise", Args: []string{"install", "--force", "--yes", MiseName(t) + "@latest"}, AsUser: true}); err != nil {
				return err
			}
		}
	}
	return nil
}

// MiseUse installs tools globally for the user, at their locked versions.
func MiseUse(ctx context.Context, e *Env, tools []string) error {
	if _, err := e.run(ctx, Cmd{Name: "mise", Args: append([]string{"use", "-g", "--yes"}, LockedTools(e, tools)...), AsUser: true}); err != nil {
		return err
	}
	_, err := e.run(ctx, Cmd{Name: "mise", Args: []string{"reshim"}, AsUser: true})
	return err
}

// --- apt_repo ----------------------------------------------------------------

type aptRepoStep struct{ s catalog.Step }

const keyrings = "/etc/apt/keyrings"

// Repo marks the step for the runner's apt batch.
func (aptRepoStep) Repo() bool { return true }

func (r aptRepoStep) keyPath() string { return filepath.Join(keyrings, r.s.Name+".asc") }
func (r aptRepoStep) listPath() string {
	return filepath.Join("/etc/apt/sources.list.d", r.s.Name+".sources")
}

func (r aptRepoStep) source(e *Env) string {
	s := strings.NewReplacer("{{arch}}", e.Arch, "{{codename}}", e.Codename, "{{distro}}", distro(e), "{{keyring}}", r.keyPath()).Replace(r.s.Source)
	return strings.TrimSpace(s) + "\n"
}

// keyURL is the signing key's URL, {{distro}} filled in.
func (r aptRepoStep) keyURL(e *Env) string {
	return strings.ReplaceAll(r.s.KeyURL, "{{distro}}", distro(e))
}

// distro is e's apt family, ubuntu when unknown.
func distro(e *Env) string {
	if e.Distro == "" {
		return "ubuntu"
	}
	return e.Distro
}

func (r aptRepoStep) Plan(e *Env) []Action {
	return []Action{{"repo", r.listPath() + " (key " + r.keyURL(e) + ")"}}
}

func (r aptRepoStep) Check(_ context.Context, e *Env) (bool, error) {
	if _, err := os.Stat(e.Path(r.keyPath())); err != nil {
		return false, nil
	}
	b, err := os.ReadFile(e.Path(r.listPath()))
	return err == nil && string(b) == r.source(e), nil
}

func (r aptRepoStep) Apply(ctx context.Context, e *Env) error {
	kp, url := e.Path(r.keyPath()), r.keyURL(e)
	if _, err := os.Stat(kp); err != nil {
		if e.Download == nil {
			return errors.New("no downloader")
		}
		key, err := e.Download(ctx, url)
		if err != nil {
			return fmt.Errorf("download the signing key %s: %w", url, err)
		}
		if len(key) == 0 {
			return fmt.Errorf("download the signing key %s: empty", url)
		}
		if r.s.KeyFingerprint != "" {
			if err := checkFingerprint(ctx, e, key, r.s.KeyFingerprint); err != nil {
				return fmt.Errorf("%s: %w", url, err)
			}
		}
		if err := writeFile(kp, key, 0o644); err != nil {
			return err
		}
	}
	return writeFile(e.Path(r.listPath()), []byte(r.source(e)), 0o644)
}

// checkFingerprint checks that key (armored or binary) is the key with fpr.
func checkFingerprint(ctx context.Context, e *Env, key []byte, fpr string) error {
	out, err := e.Exec.Run(ctx, Cmd{Name: "gpg", Args: []string{"--show-keys", "--with-colons"}, Stdin: key})
	if err != nil {
		return fmt.Errorf("gpg --show-keys: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if f := strings.Split(line, ":"); len(f) > 9 && f[0] == "fpr" && f[9] == fpr {
			return nil
		}
	}
	return fmt.Errorf("signing key fingerprint is not %s", fpr)
}

// --- file / template ---------------------------------------------------------

type fileStep struct {
	s        catalog.Step
	template bool
}

func (f fileStep) Plan(*Env) []Action { return []Action{{"file", f.s.To}} }

func (f fileStep) mode() fs.FileMode {
	if f.s.Mode == "" {
		return 0o644
	}
	m, _ := strconv.ParseUint(f.s.Mode, 8, 32)
	return fs.FileMode(m)
}

func (f fileStep) content(e *Env) ([]byte, error) {
	b, err := fs.ReadFile(e.Files, f.s.Src)
	if err != nil || !f.template {
		return b, err
	}
	t, err := template.New(f.s.Src).Option("missingkey=error").Parse(string(b))
	if err != nil {
		return nil, err
	}
	vars := map[string]string{"user": e.User, "home": e.Home, "accent": accentOf(e)}
	for k, v := range f.s.Vars {
		vars[k] = v
	}
	var out bytes.Buffer
	if err := t.Execute(&out, vars); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (f fileStep) Check(_ context.Context, e *Env) (bool, error) {
	p := e.Path(f.s.To)
	cur, err := os.ReadFile(p)
	if err != nil {
		return false, nil
	}
	if f.s.IfAbsent {
		return true, nil
	}
	want, err := f.content(e)
	if err != nil {
		return false, err
	}
	fi, _ := os.Stat(p)
	return bytes.Equal(cur, want) && fi.Mode().Perm() == f.mode(), nil
}

func (f fileStep) Apply(_ context.Context, e *Env) error {
	p := e.Path(f.s.To)
	want, err := f.content(e)
	if err != nil {
		return err
	}
	cur, err := os.ReadFile(p)
	if err == nil && f.s.IfAbsent {
		return nil
	}
	if err == nil && f.s.KeepOrig && !bytes.Equal(cur, want) {
		if _, err := os.Stat(p + ".orig"); errors.Is(err, fs.ErrNotExist) {
			fi, _ := os.Stat(p)
			if err := writeFile(p+".orig", cur, fi.Mode().Perm()); err != nil {
				return err
			}
		}
	}
	if err := mkdirFor(e, p, f.s); err != nil {
		return err
	}
	if err := writeFile(p, want, f.mode()); err != nil {
		return err
	}
	uid, gid, _ := e.owner(f.s)
	return e.chown(p, uid, gid)
}

// mkdirFor creates p's directory, owned like the file under the home.
func mkdirFor(e *Env, p string, s catalog.Step) error {
	dir := filepath.Dir(p)
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	uid, gid, user := e.owner(s)
	home := e.Path("~")
	if !user {
		return nil
	}
	for d := dir; strings.HasPrefix(d, home+"/"); d = filepath.Dir(d) {
		if err := e.chown(d, uid, gid); err != nil {
			return err
		}
	}
	return nil
}

// writeFile writes b to p atomically with mode.
func writeFile(p string, b []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr, os.Chmod(tmp.Name(), mode)); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// --- line --------------------------------------------------------------------

type lineStep struct{ s catalog.Step }

func (l lineStep) Plan(*Env) []Action { return []Action{{"line", l.s.To + ": " + l.s.Line}} }

// want is the file with the line in it, or nil when the file is missing
// and IfExists says to leave it.
func (l lineStep) want(cur []byte, missing bool) []byte {
	if missing && l.s.IfExists {
		return nil
	}
	if l.s.Top {
		return withLineFirst(cur, l.s.Line)
	}
	return WithLine(cur, l.s.Line, l.s.Match)
}

func (l lineStep) Check(_ context.Context, e *Env) (bool, error) {
	cur, err := os.ReadFile(e.Path(l.s.To))
	missing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !missing {
		return false, err
	}
	want := l.want(cur, missing)
	return want == nil || (!missing && bytes.Equal(cur, want)), nil
}

func (l lineStep) Apply(_ context.Context, e *Env) error {
	p := e.Path(l.s.To)
	cur, err := os.ReadFile(p)
	missing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !missing {
		return err
	}
	want := l.want(cur, missing)
	if want == nil {
		return nil
	}
	mode := fs.FileMode(0o644)
	if fi, err := os.Stat(p); err == nil {
		mode = fi.Mode().Perm()
	} else if err := mkdirFor(e, p, l.s); err != nil {
		return err
	}
	if err := writeFile(p, want, mode); err != nil {
		return err
	}
	uid, gid, _ := e.owner(l.s)
	return e.chown(p, uid, gid)
}

// LastMark starts the block that sources a user's local override file;
// it stays at the end of the file (WithLine adds lines before it).
const LastMark = "# yardo: local overrides (keep last)"

// WithLine returns b with line in it: in place of every line matching the
// regexp match (when there is one and it matches), else appended after a
// blank line (none in an empty file), unless b already has it. An
// appended line goes before the LastMark block when there is one.
func WithLine(b []byte, line, match string) []byte {
	lines := strings.Split(string(b), "\n")
	if match != "" {
		re := regexp.MustCompile(match)
		hit := false
		for i, x := range lines {
			if re.MatchString(x) {
				lines[i], hit = line, true
			}
		}
		if hit {
			return []byte(strings.Join(lines, "\n"))
		}
	}
	for _, x := range lines {
		if x == line {
			return b
		}
	}
	if len(b) == 0 {
		return []byte(line + "\n")
	}
	if i := slices.Index(lines, LastMark); i >= 0 {
		return []byte(strings.Join(slices.Insert(lines, i, line, ""), "\n"))
	}
	return append(append([]byte{}, b...), "\n"+line+"\n"...)
}

// withLineFirst returns b with line as its first line, unless b already
// has it.
func withLineFirst(b []byte, line string) []byte {
	for _, x := range strings.Split(string(b), "\n") {
		if x == line {
			return b
		}
	}
	return append([]byte(line+"\n"), b...)
}

// --- link --------------------------------------------------------------------

type linkStep struct{ s catalog.Step }

func (l linkStep) Plan(*Env) []Action { return []Action{{"link", l.s.To + " → " + l.s.Src}} }

func (l linkStep) Check(_ context.Context, e *Env) (bool, error) {
	got, err := os.Readlink(e.Path(l.s.To))
	return err == nil && got == l.s.Src, nil
}

func (l linkStep) Apply(_ context.Context, e *Env) error {
	p := e.Path(l.s.To)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	_ = os.Remove(p)
	if err := os.Symlink(l.s.Src, p); err != nil {
		return err
	}
	uid, gid, _ := e.owner(l.s)
	return e.chown(p, uid, gid)
}

// --- git_clone ---------------------------------------------------------------

type gitCloneStep struct{ s catalog.Step }

func (g gitCloneStep) Plan(*Env) []Action { return []Action{{"clone", g.s.Repo + " → " + g.s.To}} }

func (g gitCloneStep) Check(_ context.Context, e *Env) (bool, error) {
	_, err := os.Stat(e.Path(g.s.To))
	return err == nil, nil
}

func (g gitCloneStep) Apply(ctx context.Context, e *Env) error {
	args := []string{"clone", "-q", "--depth", "1"}
	if g.s.Ref != "" {
		args = append(args, "--branch", g.s.Ref)
	}
	_, _, user := e.owner(g.s)
	_, err := e.run(ctx, Cmd{Name: "git", Args: append(args, g.s.Repo, e.Path(g.s.To)), AsUser: user})
	return err
}

// --- systemctl ---------------------------------------------------------------

type systemctlStep struct{ s catalog.Step }

func (u systemctlStep) action() string {
	if u.s.Action == "" {
		return "enable-now"
	}
	return u.s.Action
}

func (u systemctlStep) Plan(*Env) []Action {
	return []Action{{"unit", strings.TrimSpace(u.action() + " " + u.s.Unit)}}
}

// Follows: a restart or daemon-reload runs only after a change.
func (u systemctlStep) Follows() bool { return u.action() != "enable-now" }

func (u systemctlStep) Check(ctx context.Context, e *Env) (bool, error) {
	if u.Follows() {
		return true, nil
	}
	_, err1 := e.Exec.Run(ctx, Cmd{Name: "systemctl", Args: []string{"is-enabled", "--quiet", u.s.Unit}})
	_, err2 := e.Exec.Run(ctx, Cmd{Name: "systemctl", Args: []string{"is-active", "--quiet", u.s.Unit}})
	return err1 == nil && err2 == nil, nil
}

// Down: the unit is enabled but not running.
func (u systemctlStep) Down(ctx context.Context, e *Env) (bool, string) {
	return unitDown(ctx, e, u.s.Unit), u.s.Unit
}

func unitDown(ctx context.Context, e *Env, unit string) bool {
	_, en := e.Exec.Run(ctx, Cmd{Name: "systemctl", Args: []string{"is-enabled", "--quiet", unit}})
	_, act := e.Exec.Run(ctx, Cmd{Name: "systemctl", Args: []string{"is-active", "--quiet", unit}})
	return en == nil && act != nil
}

func (u systemctlStep) Apply(ctx context.Context, e *Env) error {
	var args []string
	switch u.action() {
	case "enable-now":
		args = []string{"enable", "--now", u.s.Unit}
	case "restart":
		args = []string{"restart", u.s.Unit}
	case "daemon-reload":
		args = []string{"daemon-reload"}
	}
	_, err := e.run(ctx, Cmd{Name: "systemctl", Args: args})
	return err
}

// --- sysctl ------------------------------------------------------------------

type sysctlStep struct{ s catalog.Step }

func (s sysctlStep) file() fileStep {
	f := s.s
	f.Owner = "root"
	return fileStep{s: f}
}

func (s sysctlStep) Plan(*Env) []Action { return []Action{{"sysctl", s.s.To}} }

func (s sysctlStep) Check(ctx context.Context, e *Env) (bool, error) { return s.file().Check(ctx, e) }

func (s sysctlStep) Apply(ctx context.Context, e *Env) error {
	if err := s.file().Apply(ctx, e); err != nil {
		return err
	}
	_, err := e.run(ctx, Cmd{Name: "sysctl", Args: []string{"-q", "-p", s.s.To}})
	return err
}

// --- git_config --------------------------------------------------------------

type gitConfigStep struct{ s catalog.Step }

// values are the settings to make, in a stable order, without the ones
// whose answer is empty.
func (g gitConfigStep) values(e *Env) [][2]string {
	name, email := "", ""
	if e.Answers != nil {
		name, email = e.Answers.Git.Name, e.Answers.Git.Email
	}
	r := strings.NewReplacer("{{git_name}}", name, "{{git_email}}", email)
	keys := make([]string, 0, len(g.s.Values))
	for k := range g.s.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out [][2]string
	for _, k := range keys {
		if v := r.Replace(g.s.Values[k]); v != "" {
			out = append(out, [2]string{k, v})
		}
	}
	return out
}

func (g gitConfigStep) Plan(e *Env) []Action {
	var out []Action
	for _, kv := range g.values(e) {
		out = append(out, Action{"git", kv[0] + "=" + kv[1]})
	}
	return out
}

func (g gitConfigStep) applies(ctx context.Context, e *Env) bool {
	return g.s.IfTool == "" || root(ctx, e, g.s.IfTool)
}

func (g gitConfigStep) Check(ctx context.Context, e *Env) (bool, error) {
	if !g.applies(ctx, e) {
		return true, nil
	}
	for _, kv := range g.values(e) {
		out, err := e.Exec.Run(ctx, Cmd{Name: "git", Args: []string{"config", "--global", "--get", kv[0]}, AsUser: true})
		if err != nil || strings.TrimSuffix(string(out), "\n") != kv[1] {
			return false, nil
		}
	}
	return true, nil
}

func (g gitConfigStep) Apply(ctx context.Context, e *Env) error {
	if !g.applies(ctx, e) {
		return nil
	}
	for _, kv := range g.values(e) {
		if _, err := e.run(ctx, Cmd{Name: "git", Args: []string{"config", "--global", kv[0], kv[1]}, AsUser: true}); err != nil {
			return err
		}
	}
	return nil
}

// --- user_shell --------------------------------------------------------------

type userShellStep struct{ s catalog.Step }

func (u userShellStep) Plan(e *Env) []Action {
	return []Action{{"shell", e.User + " → " + u.s.Shell}}
}

func (u userShellStep) Check(ctx context.Context, e *Env) (bool, error) {
	out, err := e.Exec.Run(ctx, Cmd{Name: "getent", Args: []string{"passwd", e.User}})
	if err != nil {
		return false, nil
	}
	f := strings.Split(strings.TrimSpace(string(out)), ":")
	return len(f) == 7 && f[6] == u.s.Shell, nil
}

func (u userShellStep) Apply(ctx context.Context, e *Env) error {
	_, err := e.run(ctx, Cmd{Name: "usermod", Args: []string{"-s", u.s.Shell, e.User}})
	return err
}

// --- run ---------------------------------------------------------------------

type runStep struct{ s catalog.Step }

var homeRef = regexp.MustCompile(`^~(/|$)`)

// fill puts the user, their group and home, and the dpkg architecture into
// a run step's command.
func fill(e *Env, cmd string) string {
	return strings.NewReplacer("{{user}}", e.User, "{{group}}", e.Group, "{{home}}", e.Home, "{{arch}}", e.Arch).Replace(cmd)
}

func (r runStep) Plan(e *Env) []Action { return []Action{{"run", r.cmd(e)}} }

// cmd is the filled-in command; {{sha256}} is the step's pin.
func (r runStep) cmd(e *Env) string {
	return strings.ReplaceAll(fill(e, r.s.Cmd), "{{sha256}}", r.s.SHA256)
}

// Follows: an on_change step runs only after a change.
func (r runStep) Follows() bool { return r.s.OnChange }

func (r runStep) Check(ctx context.Context, e *Env) (bool, error) {
	switch {
	case r.s.Creates != "":
		_, err := os.Stat(e.Path(homeRef.ReplaceAllString(r.s.Creates, "~/")))
		return err == nil, nil
	case r.s.Check == "":
		return true, nil // on_change
	}
	_, err := e.Exec.Run(ctx, Cmd{Name: "sh", Args: []string{"-c", fill(e, r.s.Check)}, AsUser: r.s.AsUser})
	return err == nil, nil
}

// --- note --------------------------------------------------------------------

type noteStep struct{ s catalog.Step }

func (n noteStep) Plan(e *Env) []Action { return []Action{{"note", fill(e, n.s.Text)}} }

// Always: a note is checked on every run and changes nothing.
func (noteStep) Always() bool { return true }

func (noteStep) Check(context.Context, *Env) (bool, error) { return true, nil }

func (n noteStep) Apply(ctx context.Context, e *Env) error {
	if n.s.When != "" {
		if _, err := e.Exec.Run(ctx, Cmd{Name: "sh", Args: []string{"-c", fill(e, n.s.When)}}); err != nil {
			return nil
		}
	}
	e.note(fill(e, n.s.Text))
	return nil
}

func (r runStep) Apply(ctx context.Context, e *Env) error {
	_, err := e.run(ctx, Cmd{Name: "sh", Args: []string{"-c", r.cmd(e)}, AsUser: r.s.AsUser})
	if err == nil && r.s.Text != "" {
		e.note(fill(e, r.s.Text))
	}
	return err
}

// --- block -------------------------------------------------------------------

// The markers of a block step's block (comment lines with #).
const (
	BlockBegin = "# >>> yardo (managed; your own lines go outside this block)"
	BlockEnd   = "# <<< yardo"
)

// WithBlock returns cur with the block that starts at the line begin and
// ends at the line end replaced by block (which carries both), or with
// block put first when there is none. Everything else is kept.
func WithBlock(cur []byte, begin, end, block string) []byte {
	s := string(cur)
	if i, j := strings.Index(s, begin), strings.Index(s, end); i >= 0 && j > i {
		s = s[:i] + strings.TrimPrefix(s[j+len(end):], "\n")
	}
	s = strings.TrimLeft(s, "\n")
	if s != "" {
		s = "\n" + s
	}
	return []byte(block + s)
}

// blockStep keeps a marked block of Src (in the catalog's files) in the
// file To, first in it, and leaves the lines outside the block alone, so
// yardo can update its defaults while the user's own lines stay.
type blockStep struct{ s catalog.Step }

func (b blockStep) Plan(*Env) []Action {
	return []Action{{"block", b.s.To + " (yardo's block from " + b.s.Src + ")"}}
}

func (b blockStep) want(e *Env) (cur, want []byte, err error) {
	body, err := fs.ReadFile(e.Files, b.s.Src)
	if err != nil {
		return nil, nil, err
	}
	cur, err = os.ReadFile(e.Path(b.s.To))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, err
	}
	block := BlockBegin + "\n" + strings.TrimRight(string(body), "\n") + "\n" + BlockEnd + "\n"
	return cur, WithBlock(cur, BlockBegin, BlockEnd, block), nil
}

func (b blockStep) Check(_ context.Context, e *Env) (bool, error) {
	cur, want, err := b.want(e)
	return err == nil && bytes.Equal(cur, want), err
}

func (b blockStep) Apply(_ context.Context, e *Env) error {
	_, want, err := b.want(e)
	if err != nil {
		return err
	}
	p := e.Path(b.s.To)
	mode := fileStep{s: b.s}.mode()
	if fi, err := os.Stat(p); err == nil {
		mode = fi.Mode().Perm()
	} else if err := mkdirFor(e, p, b.s); err != nil {
		return err
	}
	if err := writeFile(p, want, mode); err != nil {
		return err
	}
	uid, gid, _ := e.owner(b.s)
	return e.chown(p, uid, gid)
}

// Outdated is what has a newer version: the user's mise tools and the apt
// packages (name → "current → newer"). It refreshes apt's lists first, so
// it's slow, and only for a status run that asks for it.
func Outdated(ctx context.Context, e *Env) (mise, apt map[string]string) {
	mise, apt = map[string]string{}, map[string]string{}
	if out, err := e.Exec.Run(ctx, Cmd{Name: "mise", Args: []string{"outdated", "--json"}, AsUser: true}); err == nil {
		var m map[string]struct{ Current, Latest string }
		if json.Unmarshal(out, &m) == nil {
			for name, v := range m {
				if v.Latest != "" && v.Latest != v.Current {
					mise[name] = v.Current + " → " + v.Latest
				}
			}
		}
	}
	_, _ = e.Exec.Run(ctx, Cmd{Name: "apt-get", Args: []string{"update", "-q"}, Env: AptEnv})
	out, _ := e.Exec.Run(ctx, Cmd{Name: "apt", Args: []string{"list", "--upgradable"}, Env: AptEnv})
	// pkg/suite 1.2-3 amd64 [upgradable from: 1.2-2]
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.Fields(l)
		if len(f) < 6 || !strings.Contains(l, "[upgradable from:") {
			continue
		}
		name, _, _ := strings.Cut(f[0], "/")
		apt[name] = strings.TrimSuffix(f[len(f)-1], "]") + " → " + f[1]
	}
	return mise, apt
}

// ItemUpdates are the newer versions of an item's mise tools and apt
// packages, from Outdated ("terraform 1.9.0 → 1.10.1", …).
func ItemUpdates(e *Env, s []catalog.Step, mise, apt map[string]string) []string {
	var out []string
	m, a := ItemOutdated(e, s, mise, apt)
	for _, t := range m {
		out = append(out, t+" "+mise[t])
	}
	for _, p := range a {
		out = append(out, p+" "+apt[p])
	}
	return out
}

// ItemOutdated are an item's mise tools and apt packages that Outdated
// found newer versions of.
func ItemOutdated(e *Env, s []catalog.Step, mise, apt map[string]string) (tools, pkgs []string) {
	for _, st := range s {
		switch st.Type {
		case "mise":
			for _, t := range st.Tools {
				if mise[MiseName(t)] != "" {
					tools = append(tools, MiseName(t))
				}
			}
		case "apt":
			for _, p := range (aptStep{st}).Packages(e) {
				if apt[p] != "" {
					pkgs = append(pkgs, p)
				}
			}
		}
	}
	return tools, pkgs
}

// Upgrade installs the newer versions of these mise tools (the user's,
// mise up; a locked one is left at its version) and apt packages (only
// upgrading what's installed). Nothing else changes.
func Upgrade(ctx context.Context, e *Env, tools, pkgs []string) error {
	tools = slices.DeleteFunc(slices.Compact(slices.Sorted(slices.Values(tools))), func(t string) bool {
		return e.Lock != nil && e.Lock.Mise[t] != ""
	})
	pkgs = slices.Compact(slices.Sorted(slices.Values(pkgs)))
	var cmds []Cmd
	if len(tools) > 0 {
		cmds = append(cmds, Cmd{Name: "mise", Args: append([]string{"up", "--yes"}, tools...), AsUser: true})
	}
	if len(pkgs) > 0 {
		cmds = append(cmds, Cmd{Name: "apt-get", Args: append([]string{"install", "-y", "-q", "--only-upgrade"}, pkgs...), Env: AptEnv})
	}
	for _, c := range cmds {
		if e.Log != nil {
			fmt.Fprintf(e.Log, "$ %s\n", c)
		}
		out, err := e.Exec.Run(ctx, c)
		if e.Log != nil {
			_, _ = e.Log.Write(out)
		}
		if err != nil {
			last := strings.TrimSpace(string(out))
			if i := strings.LastIndexByte(last, '\n'); i >= 0 {
				last = last[i+1:]
			}
			return fmt.Errorf("%s: %w: %s", c.Name, err, last)
		}
	}
	return nil
}
