package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeRelease serves release v<ver> of yardo whose binary is a script
// printing ver; bad makes checksums.txt disagree.
func fakeRelease(t *testing.T, ver string, bad bool) {
	t.Helper()
	bin := []byte("#!/bin/sh\necho " + ver + "\n")
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	_ = tw.WriteHeader(&tar.Header{Name: "yardo", Mode: 0o755, Size: int64(len(bin)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(bin)
	_ = tw.Close()
	_ = zw.Close()
	archive := buf.Bytes()
	name := fmt.Sprintf("yardo_%s_%s_%s.tar.gz", ver, runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(archive)
	if bad {
		sum[0] ^= 0xff
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/" + upgradeRepo + "/releases/latest", "/repos/" + upgradeRepo + "/releases/tags/v" + ver:
			_ = json.NewEncoder(w).Encode(release{Tag: "v" + ver, Assets: []releaseAsset{
				{Name: name, URL: srv.URL + "/dl/" + name}, {Name: "checksums.txt", URL: srv.URL + "/dl/checksums.txt"}}})
		case "/dl/" + name:
			_, _ = w.Write(archive)
		case "/dl/checksums.txt":
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), name)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	old := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = old })
}

func withVersion(t *testing.T, v string) {
	t.Helper()
	old := version
	version = v
	t.Cleanup(func() { version = old })
}

func installed(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "yardo")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho old\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := executablePathFn
	executablePathFn = func() (string, error) { return p, nil }
	t.Cleanup(func() { executablePathFn = old })
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return p
}

func TestUpgradeReplacesBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script binary")
	}
	fakeRelease(t, "9.9.9", false)
	withVersion(t, "0.1.0")
	p := installed(t)
	var out bytes.Buffer
	if err := upgrade(context.Background(), &out, &out, false, true, ""); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if b, _ := os.ReadFile(p); !strings.Contains(string(b), "echo 9.9.9") || !strings.Contains(out.String(), "✓ yardo upgraded 0.1.0 → 9.9.9") {
		t.Errorf("binary %q, output %q", b, out.String())
	}
}

func TestUpgradeCheckAndUpToDate(t *testing.T) {
	fakeRelease(t, "0.2.0", false)
	installed(t)
	withVersion(t, "0.1.0")
	var out bytes.Buffer
	if err := upgrade(context.Background(), &out, &out, true, false, ""); err != nil || !strings.Contains(out.String(), "yardo 0.1.0 → 0.2.0 available") {
		t.Errorf("check: %v %q", err, out.String())
	}
	withVersion(t, "0.2.0")
	out.Reset()
	if err := upgrade(context.Background(), &out, &out, false, true, ""); err != nil || !strings.Contains(out.String(), "up to date") {
		t.Errorf("up to date: %v %q", err, out.String())
	}
}

func TestUpgradeChecksumMismatchKeepsBinary(t *testing.T) {
	fakeRelease(t, "9.9.9", true)
	withVersion(t, "0.1.0")
	p := installed(t)
	if err := upgrade(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, false, true, ""); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("err = %v", err)
	}
	if b, _ := os.ReadFile(p); !strings.Contains(string(b), "echo old") {
		t.Errorf("binary replaced: %q", b)
	}
}

func TestUpgradeNeedsYesWithoutTerminal(t *testing.T) {
	fakeRelease(t, "9.9.9", false)
	withVersion(t, "0.1.0")
	installed(t)
	old := interactiveFn
	interactiveFn = func() bool { return false }
	t.Cleanup(func() { interactiveFn = old })
	if err := upgrade(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, false, false, ""); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("err = %v", err)
	}
}

func TestUpgradeHomebrew(t *testing.T) {
	fakeRelease(t, "9.9.9", false)
	withVersion(t, "0.1.0")
	cellar := filepath.Join(t.TempDir(), "Cellar", "yardo", "0.1.0", "bin", "yardo")
	old := executablePathFn
	executablePathFn = func() (string, error) { return cellar, nil }
	t.Cleanup(func() { executablePathFn = old })
	var calls []string
	oldRun := runAttachedFn
	runAttachedFn = func(name string, args ...string) error {
		calls = append(calls, filepath.Base(name)+" "+strings.Join(args, " "))
		return nil
	}
	t.Cleanup(func() { runAttachedFn = oldRun })
	if needsRootUpgrade(nil) {
		t.Error("a Homebrew install must not run brew as root")
	}
	if err := upgrade(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, false, true, ""); err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls, "; ") != "brew update --quiet; brew upgrade "+upgradeFormula {
		t.Errorf("calls = %q", calls)
	}
}

func TestVersionCmd(t *testing.T) {
	withVersion(t, "1.2.3")
	var out bytes.Buffer
	if err := versionCmd(nil, &out, &out); err != nil || out.String() != "1.2.3\n" {
		t.Errorf("version: %v %q", err, out.String())
	}
	if err := versionCmd([]string{"down"}, &out, &out); err == nil {
		t.Error("want an error for 'version down'")
	}
	// up, update and upgrade all upgrade.
	fakeRelease(t, "1.3.0", false)
	installed(t)
	for _, cmd := range []string{"up", "update", "upgrade"} {
		out.Reset()
		if err := versionCmd([]string{cmd, "--check"}, &out, &out); err != nil || !strings.Contains(out.String(), "1.2.3 → 1.3.0 available") {
			t.Errorf("version %s --check: %v %q", cmd, err, out.String())
		}
	}
}
