package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/eugenetaranov/yardo/internal/ui"
)

// yardo version [up|upgrade]: the version, or an upgrade to the latest
// release (a Homebrew install upgrades through brew).

const (
	upgradeRepo    = "eugenetaranov/yardo"
	upgradeFormula = "eugenetaranov/tap/yardo"
	maxBinarySize  = 100 << 20
)

// Seams for tests.
var (
	githubAPIBase    = "https://api.github.com"
	executablePathFn = os.Executable
	// runAttachedFn runs a command with the terminal attached (brew).
	runAttachedFn = func(name string, args ...string) error {
		c := exec.Command(name, args...)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		return c.Run()
	}
	confirmUpgradeFn = func(q string) (bool, error) { return ui.Confirm(q, "", true) }
	interactiveFn    = func() bool { return isTerminal(os.Stdin) && isTerminal(os.Stdout) }
	upgradeHTTP      = &http.Client{Timeout: 30 * time.Second}
)

// versionCmd runs `yardo version [up|upgrade] [--check] [--yes] [--version vX.Y.Z]`.
func versionCmd(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintln(stdout, version)
		return nil
	}
	if args[0] != "up" && args[0] != "upgrade" {
		return fmt.Errorf("version: unknown argument %q (want: yardo version up)", args[0])
	}
	fs := flag.NewFlagSet("yardo version up", flag.ContinueOnError)
	fs.SetOutput(stderr)
	check := fs.Bool("check", false, "only say whether a newer release is out")
	yes := fs.Bool("yes", false, "upgrade without asking")
	want := fs.String("version", "", "install this release (vX.Y.Z) instead of the latest")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	return upgrade(context.Background(), stdout, stderr, *check, *yes, *want)
}

// brewInstall reports whether exe lives in a Homebrew Cellar, and brew's path.
func brewInstall(exe string) (string, bool) {
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	i := strings.Index(exe, "/Cellar/yardo/")
	if i < 0 {
		return "", false
	}
	brew := filepath.Join(exe[:i], "bin", "brew")
	if _, err := os.Stat(brew); err != nil {
		if p, err := exec.LookPath("brew"); err == nil {
			brew = p
		}
	}
	return brew, true
}

// needsRootUpgrade: replacing a plain binary needs root; brew must not
// run as root, and --check changes nothing.
func needsRootUpgrade(args []string) bool {
	if slices.Contains(args, "--check") || slices.Contains(args, "-check") {
		return false
	}
	exe, err := executablePathFn()
	if err != nil {
		return true
	}
	_, brew := brewInstall(exe)
	return !brew
}

func upgrade(ctx context.Context, stdout, stderr io.Writer, check, yes bool, wantTag string) error {
	exe, err := executablePathFn()
	if err != nil {
		return err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	brew, isBrew := brewInstall(exe)
	if isBrew && wantTag != "" {
		return errors.New("--version doesn't apply to a Homebrew install; brew installs the formula's version")
	}
	rel, err := fetchRelease(ctx, wantTag)
	if err != nil {
		return err
	}
	target, ok := parseVersion(rel.Tag)
	if !ok {
		return fmt.Errorf("release tag %q is not a version", rel.Tag)
	}
	cur, isRelease := parseVersion(version)
	if isRelease && !cur.less(target) && wantTag == "" {
		fmt.Fprintf(stdout, "✓ yardo %s is up to date\n", cur)
		return nil
	}
	from := version
	if isRelease {
		from = cur.String()
	}
	how := "download and replace " + exe
	if isBrew {
		how = "Homebrew: brew upgrade " + upgradeFormula
	}
	if check {
		fmt.Fprintf(stdout, "yardo %s → %s available (%s)\n", from, target, how)
		return nil
	}
	if !yes {
		if !interactiveFn() {
			return fmt.Errorf("yardo %s → %s is available; pass --yes to upgrade without a terminal", from, target)
		}
		ok, err := confirmUpgradeFn(fmt.Sprintf("Upgrade yardo %s → %s (%s)?", from, target, how))
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("upgrade declined")
		}
	}
	if isBrew {
		// brew upgrade reads the tap as Homebrew last fetched it: fetch first.
		if err := runAttachedFn(brew, "update", "--quiet"); err != nil {
			fmt.Fprintf(stderr, "warning: brew update: %v\n", err)
		}
		if err := runAttachedFn(brew, "upgrade", upgradeFormula); err != nil {
			return fmt.Errorf("brew upgrade %s: %w", upgradeFormula, err)
		}
		fmt.Fprintf(stdout, "✓ yardo upgraded %s → %s (Homebrew)\n", from, target)
		return nil
	}
	name := fmt.Sprintf("yardo_%s_%s_%s.tar.gz", target, runtime.GOOS, runtime.GOARCH)
	archiveAsset, ok := rel.asset(name)
	if !ok {
		return fmt.Errorf("release %s has no %s", rel.Tag, name)
	}
	sumsAsset, ok := rel.asset("checksums.txt")
	if !ok {
		return fmt.Errorf("release %s has no checksums.txt", rel.Tag)
	}
	archive, err := fetchAsset(ctx, archiveAsset.URL, maxBinarySize)
	if err != nil {
		return err
	}
	sums, err := fetchAsset(ctx, sumsAsset.URL, 1<<20)
	if err != nil {
		return err
	}
	if err := verifyChecksum(sums, name, archive); err != nil {
		return err
	}
	bin, err := extractBinary(archive)
	if err != nil {
		return err
	}
	if err := replaceBinary(exe, bin, target); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "✓ yardo upgraded %s → %s (%s)\n", from, target, exe)
	return nil
}

type semver [3]int

// parseVersion reads "1.2.3" or "v1.2.3"; ok is false for anything else
// (dev builds, git describe strings).
func parseVersion(s string) (v semver, ok bool) {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(s), "v"), ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

func (a semver) less(b semver) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func (a semver) String() string { return fmt.Sprintf("%d.%d.%d", a[0], a[1], a[2]) }

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type release struct {
	Tag    string         `json:"tag_name"`
	Assets []releaseAsset `json:"assets"`
}

func (r *release) asset(name string) (releaseAsset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return releaseAsset{}, false
}

func githubGet(ctx context.Context, url, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" && strings.HasPrefix(url, githubAPIBase) {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := upgradeHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		msg := fmt.Sprintf("GET %s: %s", url, resp.Status)
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			msg += " (GitHub rate limit? set GITHUB_TOKEN)"
		}
		return nil, errors.New(msg)
	}
	return resp, nil
}

// fetchRelease returns the latest release, or the one tagged tag.
func fetchRelease(ctx context.Context, tag string) (*release, error) {
	url := githubAPIBase + "/repos/" + upgradeRepo + "/releases/latest"
	if tag != "" {
		if !strings.HasPrefix(tag, "v") {
			tag = "v" + tag
		}
		url = githubAPIBase + "/repos/" + upgradeRepo + "/releases/tags/" + tag
	}
	resp, err := githubGet(ctx, url, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var r release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("read release: %w", err)
	}
	return &r, nil
}

func fetchAsset(ctx context.Context, url string, limit int64) ([]byte, error) {
	resp, err := githubGet(ctx, url, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("download %s: larger than %d bytes", url, limit)
	}
	return b, nil
}

// verifyChecksum checks data's sha256 against name's line in a
// goreleaser checksums.txt ("<hex>  <name>").
func verifyChecksum(checksums []byte, name string, data []byte) error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	sc := bufio.NewScanner(bytes.NewReader(checksums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			if !strings.EqualFold(f[0], got) {
				return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", name, f[0], got)
			}
			return nil
		}
	}
	return fmt.Errorf("%s is not listed in checksums.txt", name)
}

// extractBinary returns the "yardo" entry of a .tar.gz archive.
func extractBinary(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, errors.New("the archive has no yardo binary")
		}
		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}
		if h.Typeflag == tar.TypeReg && filepath.Base(h.Name) == "yardo" {
			b, err := io.ReadAll(io.LimitReader(tr, maxBinarySize+1))
			if err != nil {
				return nil, fmt.Errorf("read archive: %w", err)
			}
			if len(b) > maxBinarySize {
				return nil, errors.New("the yardo binary in the archive is too large")
			}
			return b, nil
		}
	}
}

// replaceBinary puts bin at target atomically, once the new binary runs
// and reports want.
func replaceBinary(target string, bin []byte, want semver) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), ".yardo-upgrade-*")
	if err != nil {
		return fmt.Errorf("can't write next to %s (run it with sudo): %w", target, err)
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(bin); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o755); err != nil {
		return err
	}
	out, err := exec.Command(name, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("the downloaded yardo doesn't run: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	if strings.TrimSpace(string(out)) != want.String() {
		return fmt.Errorf("the downloaded yardo reports %q, want %s", strings.TrimSpace(string(out)), want)
	}
	return os.Rename(name, target)
}
