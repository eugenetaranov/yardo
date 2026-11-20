package steps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
)

func init() { RegisterGo("gitkey", func(catalog.Step) Step { return gitKeyStep{} }) }

// gitKeyStep sets up SSH for the git hosts the GitHub, GitLab and
// Bitbucket items reach (and any others yardo gitkey was given), each with
// the key chosen for it: one yardo made or was given, in
// ~/.ssh/identities/<item> (pasted, or sent with yardo gitkey; a key there
// is replaced only when the user asked, moved aside first), one of the
// user's under ~/.ssh, or none (a forwarded agent). Hosts set up by an
// earlier yardo keep what their mode says: ~/.ssh/id_ed25519 (made when the
// user has no key), ~/.ssh/<item>, or ~/.ssh/yardo_git_key (also other
// hosts' sent key).
//
// Its settings are in ~/.ssh/config.d/yardo-git, included before the first
// Host of ~/.ssh/config; a host the user's config sets is left to them.
// Its defaults (accepting new hosts' keys, a setting) are in
// ~/.ssh/config.d/yardo-defaults, included last, so the user's own win.
// GitHub's and Bitbucket's published host keys go in known_hosts; other
// hosts are trusted on first use.
type gitKeyStep struct{}

const (
	// GitKeyPath is where the sent key for other hosts goes (and the sent
	// key of hosts set up so by an earlier yardo).
	GitKeyPath = "~/.ssh/yardo_git_key"
	// ownKey is the key "key" mode used before keys were named for their
	// host: kept where it's there.
	ownKey     = "~/.ssh/id_ed25519"
	sshConfig  = "~/.ssh/config"
	knownHosts = "~/.ssh/known_hosts"
	// gitBegin and gitEnd mark the block an earlier yardo wrote at the top
	// of ~/.ssh/config: moved to gitConfig.
	gitBegin = "# >>> yardo gitkey (managed)"
	gitEnd   = "# <<< yardo gitkey"
	// gitConfig and sshDefaults are yardo's files, included by
	// ~/.ssh/config (relative paths there are under ~/.ssh).
	gitConfig      = "~/.ssh/config.d/yardo-git"
	sshDefaults    = "~/.ssh/config.d/yardo-defaults"
	includeGit     = "config.d/yardo-git"
	includeDefault = "config.d/yardo-defaults"
)

// gitHost is a git host ("host" or "host:port") and how it is reached.
type gitHost struct {
	host, mode string
	// id is the host item (github, gitlab, bitbucket; "" for another host).
	id string
	// keys is where to add a key made here.
	keys string
	// key is its key's path: the one chosen ("path" mode) or, in "key"
	// mode, once resolved (see ownKeys).
	key string
}

// identity reports whether the host's key is yardo's to make or write:
// ~/.ssh/identities/<item>.
func (g gitHost) identity() bool {
	return g.mode == "path" && g.id != "" && g.key == answers.IdentityKey(g.id)
}

// names are the host names its entry is for.
func (g gitHost) names() []string {
	if g.id == "bitbucket" {
		return []string{"bitbucket.org", "ssh.bitbucket.org"}
	}
	h, _, _ := strings.Cut(g.host, ":")
	return []string{h}
}

// OwnKey is the key "key" mode uses for the host item id (github, gitlab,
// bitbucket): ~/.ssh/<id>, or ~/.ssh/id_ed25519 when only that one is
// there (an earlier yardo's, already on the accounts). exists reports
// whether a key file is there.
func OwnKey(id string, exists func(string) bool) string {
	p := "~/.ssh/" + id
	if !exists(p) && exists(ownKey) {
		return ownKey
	}
	return p
}

// ownKeys resolves the "key" mode hosts' keys.
func ownKeys(ctx context.Context, e *Env, hosts []gitHost) error {
	var rerr error
	exists := func(p string) bool {
		b, err := sshRead(ctx, e, p)
		if err != nil {
			rerr = err
		}
		return b != nil
	}
	for i, g := range hosts {
		if g.mode == "key" {
			hosts[i].key = OwnKey(g.id, exists)
		}
	}
	return rerr
}

const gitHubKeys = "a deploy key (repo → Settings → Deploy keys; one repo) or on your account (https://github.com/settings/keys; all your repos)"

// gitHosts are the picked host items' hosts (those with no key here, and
// GitHub over a token, need no SSH), then the other hosts the sent key is
// for.
func gitHosts(e *Env) []gitHost {
	a := e.Answers
	if a == nil {
		a = &answers.Answers{}
	}
	var out []gitHost
	add := func(id, host string, m answers.GitHost, keys string) {
		if !slices.Contains(a.Picks, id) {
			return
		}
		g := gitHost{host: host, id: id, keys: keys, mode: m.Mode}
		switch {
		case m.Key == answers.NoKey, m.Key == "" && m.Mode == "token":
			return
		case m.Key != "":
			g.mode, g.key = "path", m.Key
		case m.Mode == "":
			g.mode = "own"
		}
		out = append(out, g)
	}
	add("github", "github.com", a.GitHub, gitHubKeys)
	add("gitlab", a.GitLabHost(), a.GitLab, "a deploy key (project → Settings → Repository → Deploy keys) or on your account (https://"+a.GitLabHost()+"/-/user_settings/ssh_keys)")
	add("bitbucket", "bitbucket.org", a.Bitbucket, "an access key (repository settings → Access keys) or on your account (https://bitbucket.org/account/settings/ssh-keys/)")
	for _, h := range a.GitKeyHosts {
		if !slices.ContainsFunc(out, func(g gitHost) bool { return g.host == h }) {
			out = append(out, gitHost{host: h, mode: "sent", keys: "a deploy key or on your account there"})
		}
	}
	return out
}

func withMode(hosts []gitHost, mode string) []gitHost {
	return slices.DeleteFunc(slices.Clone(hosts), func(g gitHost) bool { return g.mode != mode || (mode == "key" && g.host == "github.com") })
}

func names(hosts []gitHost) string {
	var n []string
	for _, g := range hosts {
		n = append(n, g.host)
	}
	return strings.Join(n, ", ")
}

func sentKey(e *Env) []byte {
	if e.Secrets == nil {
		return nil
	}
	return e.Secrets.GitKey
}

// given is the key pasted or sent this run for the host item id, and
// whether the user chose to replace the one at ~/.ssh/identities/<id>.
func given(e *Env, id string) (key []byte, replace bool) {
	if e.Secrets == nil {
		return nil, false
	}
	return e.Secrets.GitKeys[id], e.Secrets.GitReplace[id]
}

func (gitKeyStep) Plan(e *Env) []Action {
	hosts := gitHosts(e)
	var out []Action
	if sent := withMode(hosts, "sent"); len(sent) > 0 {
		key := "keep " + GitKeyPath + " (or generate one)"
		if sentKey(e) != nil {
			key = "write the key sent on stdin to " + GitKeyPath
		}
		out = append(out, Action{"file", key + " for " + names(sent)})
	}
	for _, g := range withMode(hosts, "key") {
		out = append(out, Action{"file", "keep ~/.ssh/" + g.id + " (or generate one) for " + g.host})
	}
	for _, g := range withMode(hosts, "path") {
		key, replace := given(e, g.id)
		switch {
		case !g.identity():
			out = append(out, Action{"file", "use " + g.key + " for " + g.host})
		case key != nil:
			out = append(out, Action{"file", "write the key given to " + g.key + " for " + g.host})
		case replace:
			out = append(out, Action{"file", "replace " + g.key + " (the old one moved aside) for " + g.host})
		default:
			out = append(out, Action{"file", "keep " + g.key + " (or generate one) for " + g.host})
		}
	}
	if wantsOwnKey(hosts) {
		out = append(out, Action{"file", "keep your SSH key (or generate " + ownKey + ")"})
	}
	out = append(out, Action{"file", gitConfig + " and " + sshDefaults + ", included by " + sshConfig})
	return out
}

// entries are yardo's settings for the hosts, but those the user's config
// sets (own: their names, and the line setting them), which are told
// apart (left to the user).
func entries(hosts []gitHost, own map[string]int) (cfg string, left []string) {
	var b strings.Builder
	for _, g := range hosts {
		host, port, _ := strings.Cut(g.host, ":")
		key := ""
		switch {
		case g.mode == "sent":
			key = GitKeyPath
		case g.mode == "key" && g.key != ownKey, g.mode == "path":
			key = g.key // ssh looks for id_ed25519 by itself, not this one
		}
		if key == "" && host == "github.com" && port == "" {
			continue
		}
		if i := slices.IndexFunc(g.names(), func(n string) bool { return own[n] > 0 }); i >= 0 {
			n := g.names()[i]
			left = append(left, fmt.Sprintf("~/.ssh/config sets %s (line %d): yardo leaves it to you; its key is %s", n, own[n], cmpOr(key, ownKey)))
			continue
		}
		if g.id == "bitbucket" {
			// Bitbucket serves git over SSH at ssh.bitbucket.org: remotes
			// naming either go there, its host keys pinned.
			b.WriteString("Host bitbucket.org ssh.bitbucket.org\n  HostName ssh.bitbucket.org\n  User git\n")
		} else {
			fmt.Fprintf(&b, "Host %s\n  User git\n", host)
		}
		if key != "" {
			fmt.Fprintf(&b, "  IdentityFile %s\n  IdentitiesOnly yes\n", key)
		}
		if port != "" {
			fmt.Fprintf(&b, "  Port %s\n", port)
		}
		if host != "github.com" && g.id != "bitbucket" {
			b.WriteString("  StrictHostKeyChecking accept-new\n")
		}
	}
	if b.Len() == 0 {
		return "", left
	}
	return "# yardo's SSH settings for your git hosts (sudo yardo rewrites this file).\n" +
		"# A host your ~/.ssh/config sets is left to you.\n" + b.String(), left
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// defaults is ~/.ssh/config.d/yardo-defaults: empty unless ssh accepts
// new hosts' keys (still refusing a changed one).
func defaults(e *Env) string {
	if e.Answers != nil && e.Answers.SSHAcceptNew == "on" {
		return "# yardo's SSH defaults (Settings); your ~/.ssh/config wins over them.\nStrictHostKeyChecking accept-new\n"
	}
	return ""
}

// directive is a config line's keyword (lower case) and its arguments.
func directive(line string) (string, []string) {
	f := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '=' })
	if len(f) == 0 || strings.HasPrefix(f[0], "#") {
		return "", nil
	}
	return strings.ToLower(f[0]), f[1:]
}

// includes reports whether the line includes the file (config.d/x, or
// ~/.ssh/config.d/x).
func includes(line, file string) bool {
	k, args := directive(line)
	return k == "include" && len(args) == 1 && (args[0] == file || args[0] == "~/.ssh/"+file)
}

// withIncludes is ~/.ssh/config without the block an earlier yardo wrote
// at its top (every other line as it was), including yardo's settings
// before its first Host or Match (per-host settings must come before the
// user's Host *) and its defaults at its end, after a Match all (or they'd
// apply only within the user's last Host). Lines already there are kept
// where they are.
func withIncludes(cfg []byte) []byte {
	lines := strings.SplitAfter(string(cfg), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if i := slices.IndexFunc(lines, func(l string) bool { return strings.TrimSpace(l) == gitBegin }); i >= 0 {
		if j := slices.IndexFunc(lines[i:], func(l string) bool { return strings.TrimSpace(l) == gitEnd }); j >= 0 {
			j += i + 1
			if i == 0 && j < len(lines) && strings.TrimSpace(lines[j]) == "" {
				j++ // the blank line it was written with
			}
			lines = slices.Delete(lines, i, j)
		}
	}
	if len(lines) > 0 && !strings.HasSuffix(lines[len(lines)-1], "\n") {
		lines[len(lines)-1] += "\n"
	}
	if !slices.ContainsFunc(lines, func(l string) bool { return includes(l, includeGit) }) {
		i := slices.IndexFunc(lines, func(l string) bool { k, _ := directive(l); return k == "host" || k == "match" })
		if i < 0 {
			i = len(lines)
		}
		lines = slices.Insert(lines, i, "Include "+includeGit+"\n")
	}
	if !slices.ContainsFunc(lines, func(l string) bool { return includes(l, includeDefault) }) {
		lines = append(lines, "Match all\n", "Include "+includeDefault+"\n")
	}
	return []byte(strings.Join(lines, ""))
}

// userHosts are the hosts' names the user's ~/.ssh/config (cfg, as
// withIncludes leaves it) sets, with the line of the first Host matching
// each. A pattern of wildcards only (Host *) and negations don't count;
// the user's own Includes aren't followed.
func userHosts(cfg []byte, hosts []gitHost) map[string]int {
	out := map[string]int{}
	for n, l := range strings.Split(string(cfg), "\n") {
		k, pats := directive(l)
		if k != "host" {
			continue
		}
		for _, p := range pats {
			p = strings.ToLower(p)
			if strings.HasPrefix(p, "!") || strings.Trim(p, "*?") == "" {
				continue
			}
			for _, g := range hosts {
				for _, name := range g.names() {
					if ok, _ := path.Match(p, name); ok && out[name] == 0 {
						out[name] = n + 1
					}
				}
			}
		}
	}
	return out
}

// sshRead and sshWrite reach the user's ~/.ssh as the user when running
// as root (a symlink there leads nowhere the user couldn't write), and
// directly otherwise (tests).
func sshRead(ctx context.Context, e *Env, p string) ([]byte, error) {
	if !e.Chown {
		b, err := os.ReadFile(e.Path(p))
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return b, err
	}
	out, err := e.Exec.Run(ctx, Cmd{Name: "sh", Args: []string{"-c", `[ -e "$1" ] || exit 0; printf x; cat -- "$1"`, "_", e.Path(p)}, AsUser: true})
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out[1:], nil
}

func sshWrite(ctx context.Context, e *Env, p string, b []byte) error {
	return sshWriteMode(ctx, e, p, b, 0o600)
}

// sshWriteMode writes p with mode, its directory made (700) when missing.
func sshWriteMode(ctx context.Context, e *Env, p string, b []byte, mode os.FileMode) error {
	if !e.Chown {
		if err := os.MkdirAll(filepath.Dir(e.Path(p)), 0o700); err != nil {
			return err
		}
		return writeFile(e.Path(p), b, mode)
	}
	_, err := e.run(ctx, Cmd{Name: "sh", Args: []string{"-c",
		`umask 077 && mkdir -p -- "$(dirname -- "$1")" && chmod 700 -- "$(dirname -- "$1")" && cat > "$1.tmp" && chmod "$2" "$1.tmp" && mv -f -- "$1.tmp" "$1"`, "_", e.Path(p), fmt.Sprintf("%o", mode)},
		Stdin: b, AsUser: true})
	return err
}

// sshMove renames from to to, as the user.
func sshMove(ctx context.Context, e *Env, from, to string) error {
	if !e.Chown {
		return os.Rename(e.Path(from), e.Path(to))
	}
	_, err := e.run(ctx, Cmd{Name: "mv", Args: []string{"-n", "--", e.Path(from), e.Path(to)}, AsUser: true})
	return err
}

// moveAside moves the key at p and its .pub to p.old-<date> (-2, -3… when
// that's taken), and returns where.
func moveAside(ctx context.Context, e *Env, p string) (string, error) {
	base := p + ".old-" + time.Now().Format("2006-01-02")
	to := base
	for n := 2; ; n++ {
		b, err := sshRead(ctx, e, to)
		if err != nil {
			return "", err
		}
		if pub, err := sshRead(ctx, e, to+".pub"); err != nil {
			return "", err
		} else if b == nil && pub == nil {
			break
		}
		to = base + "-" + strconv.Itoa(n)
	}
	if err := sshMove(ctx, e, p, to); err != nil {
		return "", err
	}
	if pub, err := sshRead(ctx, e, p+".pub"); err != nil {
		return "", err
	} else if pub != nil {
		return to, sshMove(ctx, e, p+".pub", to+".pub")
	}
	return to, nil
}

// files are what Apply writes: ~/.ssh/config, yardo's two files, and the
// notes on hosts left to the user.
func files(cfg []byte, e *Env, hosts []gitHost) (want []byte, git, defs string, left []string) {
	want = withIncludes(cfg)
	git, left = entries(hosts, userHosts(want, hosts))
	return want, git, defaults(e), left
}

func (s gitKeyStep) Check(ctx context.Context, e *Env) (bool, error) {
	hosts := gitHosts(e)
	if err := ownKeys(ctx, e, hosts); err != nil {
		return false, err
	}
	if len(withMode(hosts, "sent")) > 0 {
		key, err := sshRead(ctx, e, GitKeyPath)
		if err != nil || key == nil {
			return false, err
		}
		if sent := sentKey(e); sent != nil && !bytes.Equal(key, sent) {
			return false, nil
		}
	}
	for _, g := range slices.Concat(withMode(hosts, "key"), withMode(hosts, "path")) {
		key, err := sshRead(ctx, e, g.key)
		if err != nil || key == nil {
			return false, err
		}
		if k, replace := given(e, g.id); g.identity() && (replace || k != nil && !bytes.Equal(key, k)) {
			return false, nil
		}
	}
	if wantsOwnKey(hosts) {
		if has, err := hasOwnKey(ctx, e); err != nil || !has {
			return false, err
		}
	}
	cfg, err := sshRead(ctx, e, sshConfig)
	if err != nil {
		return false, err
	}
	want, git, defs, _ := files(cfg, e, hosts)
	for p, b := range map[string][]byte{sshConfig: want, gitConfig: []byte(git), sshDefaults: []byte(defs)} {
		cur, err := sshRead(ctx, e, p)
		if err != nil || cur == nil || !bytes.Equal(cur, b) {
			return false, err
		}
	}
	return s.knownDone(ctx, e, hosts)
}

// pinned are the hosts whose published host keys yardo adds to
// known_hosts: their file in the catalog's, and the lines it looks for.
var pinned = []struct {
	file  string
	lines []string
}{
	{"github_known_hosts", []string{"github.com ssh-ed25519 "}},
	{"bitbucket_known_hosts", []string{"bitbucket.org ssh-ed25519 ", "ssh.bitbucket.org ssh-ed25519 "}},
}

// pinnedFor are the pinned files the hosts need (GitHub's, Bitbucket's).
func pinnedFor(hosts []gitHost) []int {
	var out []int
	if slices.ContainsFunc(hosts, func(g gitHost) bool { return g.host == "github.com" }) {
		out = append(out, 0)
	}
	if slices.ContainsFunc(hosts, func(g gitHost) bool { return g.id == "bitbucket" }) {
		out = append(out, 1)
	}
	return out
}

// hasPinned reports whether known_hosts has the pinned file's keys.
func hasPinned(kh []byte, i int) bool {
	for _, l := range pinned[i].lines {
		if !bytes.HasPrefix(kh, []byte(l)) && !bytes.Contains(kh, []byte("\n"+l)) {
			return false
		}
	}
	return true
}

// knownDone reports whether GitHub's and Bitbucket's host keys are in
// known_hosts (when they're among the hosts).
func (gitKeyStep) knownDone(ctx context.Context, e *Env, hosts []gitHost) (bool, error) {
	need := pinnedFor(hosts)
	if len(need) == 0 {
		return true, nil
	}
	kh, err := sshRead(ctx, e, knownHosts)
	if err != nil {
		return false, err
	}
	return !slices.ContainsFunc(need, func(i int) bool { return !hasPinned(kh, i) }), nil
}

// userKeys are the keys ssh tries by itself: one of them is the user's
// own key.
var userKeys = []string{ownKey, "~/.ssh/id_ecdsa", "~/.ssh/id_rsa"}

// wantsOwnKey: no host is reached another way (a sent key, one named for
// it, the agent), so git takes the user's own key, made when missing.
func wantsOwnKey(hosts []gitHost) bool {
	return !slices.ContainsFunc(hosts, func(g gitHost) bool { return g.mode != "own" })
}

// hasOwnKey reports whether the user has a key of their own.
func hasOwnKey(ctx context.Context, e *Env) (bool, error) {
	for _, p := range userKeys {
		if b, err := sshRead(ctx, e, p); err != nil || b != nil {
			return b != nil, err
		}
	}
	return false, nil
}

// generate makes an ed25519 key at path (its directory 700, the .pub
// 644) and shows it, with where to add it.
func generate(ctx context.Context, e *Env, path, comment string, hosts []gitHost) error {
	if _, err := e.run(ctx, Cmd{Name: "sh", Args: []string{"-c",
		`umask 077 && mkdir -p -- "$(dirname -- "$2")" && chmod 700 -- "$(dirname -- "$2")" && ssh-keygen -q -t ed25519 -N "" -C "$1" -f "$2" && chmod 644 -- "$2.pub"`, "_", comment, e.Path(path)}, AsUser: true}); err != nil {
		return err
	}
	showKey(ctx, e, path, hosts)
	return nil
}

// showKey shows the key at path's public key, with where to add it.
func showKey(ctx context.Context, e *Env, path string, hosts []gitHost) {
	pub, _ := sshRead(ctx, e, path+".pub")
	for _, g := range hosts {
		e.key(NewKey{Host: g.host, Where: g.keys, Path: path + ".pub", Pub: strings.TrimSpace(string(pub))})
	}
}

// identityKey makes or writes the host's key at ~/.ssh/identities/<item>:
// the key given (pasted, or sent), else one made when there's none or
// the user chose to replace it. A key there is replaced only when the
// user chose to, moved aside first.
func (s gitKeyStep) identityKey(ctx context.Context, e *Env, g gitHost) error {
	cur, err := sshRead(ctx, e, g.key)
	if err != nil {
		return err
	}
	key, replace := given(e, g.id)
	if key != nil && bytes.Equal(cur, key) || key == nil && cur != nil && !replace {
		return nil
	}
	if !bytes.Contains(key, []byte("PRIVATE KEY-----")) && key != nil {
		return errors.New("the key given for " + g.host + " isn't an SSH private key")
	}
	if cur != nil {
		if !replace {
			return fmt.Errorf("%s is there already: yardo replaces it only when asked (Replace it, on %s's key in its settings; or yardo gitkey %s --replace)", g.key, g.host, g.id)
		}
		to, err := moveAside(ctx, e, g.key)
		if err != nil {
			return err
		}
		e.note("SSH key for " + g.host + ": the one replaced is at " + to)
	}
	if e.Secrets != nil {
		delete(e.Secrets.GitReplace, g.id) // done: not again this run
	}
	if key == nil {
		return generate(ctx, e, g.key, e.User+"@"+hostname()+" ("+g.id+")", []gitHost{g})
	}
	if err := sshWrite(ctx, e, g.key, key); err != nil {
		return err
	}
	if err := s.publicKey(ctx, e, g.key); err != nil {
		e.note("SSH key for " + g.host + ": " + err.Error())
	}
	showKey(ctx, e, g.key, []gitHost{g})
	return nil
}

func (s gitKeyStep) Apply(ctx context.Context, e *Env) error {
	hosts := gitHosts(e)
	if err := ownKeys(ctx, e, hosts); err != nil {
		return err
	}
	if sent := withMode(hosts, "sent"); len(sent) > 0 {
		key, err := sshRead(ctx, e, GitKeyPath)
		if err != nil {
			return err
		}
		switch k := sentKey(e); {
		case k != nil:
			if !bytes.Contains(k, []byte("PRIVATE KEY-----")) {
				return errors.New("what was sent isn't an SSH private key")
			}
			if err := sshWrite(ctx, e, GitKeyPath, k); err != nil {
				return err
			}
			if err := s.publicKey(ctx, e, GitKeyPath); err != nil {
				e.note("git SSH key: " + err.Error())
			}
		case key == nil:
			if err := generate(ctx, e, GitKeyPath, e.User+"@"+hostname()+" (git)", sent); err != nil {
				return err
			}
		}
	}
	for _, g := range withMode(hosts, "key") {
		key, err := sshRead(ctx, e, g.key)
		if err != nil {
			return err
		}
		if key == nil {
			if err := generate(ctx, e, g.key, e.User+"@"+hostname()+" ("+g.id+")", []gitHost{g}); err != nil {
				return err
			}
		}
	}
	for _, g := range withMode(hosts, "path") {
		if g.identity() {
			if err := s.identityKey(ctx, e, g); err != nil {
				return err
			}
			continue
		}
		if key, err := sshRead(ctx, e, g.key); err != nil {
			return err
		} else if key == nil {
			return fmt.Errorf("%s, %s's key, isn't there: pick another on its settings", g.key, g.host)
		}
	}
	if has, err := hasOwnKey(ctx, e); err != nil {
		return err
	} else if !has && wantsOwnKey(hosts) {
		own := withMode(hosts, "own")
		if err := generate(ctx, e, ownKey, e.User+"@"+hostname(), own); err != nil {
			return err
		}
		if len(own) == 0 {
			pub, _ := sshRead(ctx, e, ownKey+".pub")
			e.note("SSH key: " + ownKey + ".pub, made here; add it where your repos are (GitHub: https://github.com/settings/keys): " + strings.TrimSpace(string(pub)))
		}
	}
	cfg, err := sshRead(ctx, e, sshConfig)
	if err != nil {
		return err
	}
	want, git, defs, left := files(cfg, e, hosts)
	for _, n := range left {
		e.note(n)
	}
	// yardo's files first: the Include lines then find them.
	for _, f := range []struct {
		p string
		b []byte
	}{{gitConfig, []byte(git)}, {sshDefaults, []byte(defs)}, {sshConfig, want}} {
		cur, err := sshRead(ctx, e, f.p)
		if err != nil {
			return err
		}
		if cur == nil || !bytes.Equal(cur, f.b) {
			if err := sshWrite(ctx, e, f.p, f.b); err != nil {
				return err
			}
		}
	}
	kh, err := sshRead(ctx, e, knownHosts)
	if err != nil {
		return err
	}
	add := kh
	for _, i := range pinnedFor(hosts) {
		if hasPinned(kh, i) {
			continue
		}
		b, err := fs.ReadFile(e.Files, pinned[i].file)
		if err != nil {
			return err
		}
		if len(add) > 0 && !bytes.HasSuffix(add, []byte("\n")) {
			add = append(add, '\n')
		}
		add = append(add, b...)
	}
	if len(add) != len(kh) {
		return sshWrite(ctx, e, knownHosts, add)
	}
	return nil
}

// publicKey writes the key at p's .pub next to it (ssh-keygen -y); a key
// with a passphrase can't be read unattended, which is reported.
func (gitKeyStep) publicKey(ctx context.Context, e *Env, p string) error {
	if !e.Chown {
		return nil
	}
	out, err := e.Exec.Run(ctx, Cmd{Name: "ssh-keygen", Args: []string{"-y", "-P", "", "-f", e.Path(p)}, AsUser: true})
	if err != nil {
		return errors.New("it has a passphrase, so git asks for it on each use; a key without one (or a forwarded agent) works unattended")
	}
	return sshWriteMode(ctx, e, p+".pub", out, 0o644)
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}
