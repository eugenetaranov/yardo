package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/eugenetaranov/yardo/internal/assets"
	"github.com/eugenetaranov/yardo/internal/steps"
)

// newEnv describes this machine and the target user for the steps; file
// steps read from share's conf directory. With planned, a user that
// doesn't exist yet (a dry run before creating it) gets /home/<user>.
func newEnv(username, share string, log io.Writer, planned bool) (*steps.Env, error) {
	u, err := user.Lookup(username)
	if err != nil && planned {
		u, err = &user.User{Username: username, HomeDir: "/home/" + username, Uid: "-1", Gid: "-1"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("user %q: %w", username, err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	group := username
	if g, err := user.LookupGroupId(u.Gid); err == nil {
		group = g.Name
	}
	rel := readOSRelease("/etc/os-release")
	return &steps.Env{
		User: username, Group: group, UID: uid, GID: gid, Home: u.HomeDir,
		Arch: debArch(), Codename: rel.codename(), Distro: rel.distro(), Kernel: kernel(),
		Exec:     steps.OSExec{User: username},
		Files:    assets.Conf(),
		Log:      log,
		Chown:    os.Geteuid() == 0,
		Download: download,
	}, nil
}

func debArch() string {
	if out, err := exec.Command("dpkg", "--print-architecture").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return "amd64"
}

func kernel() string {
	out, _ := exec.Command("uname", "-r").Output()
	return strings.TrimSpace(string(out))
}

// osRelease is /etc/os-release's fields.
type osRelease map[string]string

func readOSRelease(path string) osRelease {
	r := osRelease{}
	f, err := os.Open(path)
	if err != nil {
		return r
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "="); ok {
			r[k] = strings.Trim(v, `"'`)
		}
	}
	return r
}

// distro is the family whose apt repos this machine uses: "ubuntu" or
// "debian" (a derivative goes by its ID_LIKE), "" for neither.
func (r osRelease) distro() string {
	for _, id := range append([]string{r["ID"]}, strings.Fields(r["ID_LIKE"])...) {
		if id == "ubuntu" || id == "debian" {
			return id
		}
	}
	return ""
}

// codename is the release's codename in its family's repos (a
// derivative's Ubuntu base, such as Mint's), noble when unknown.
func (r osRelease) codename() string {
	if r.distro() == "ubuntu" && r["UBUNTU_CODENAME"] != "" {
		return r["UBUNTU_CODENAME"]
	}
	if r["VERSION_CODENAME"] != "" {
		return r["VERSION_CODENAME"]
	}
	return "noble"
}

// supported says why yardo can't set up this system, or nil when it can.
func (r osRelease) supported() error {
	if r.distro() != "" {
		return nil
	}
	name := r["PRETTY_NAME"]
	if name == "" {
		name = "a system without /etc/os-release"
	}
	return fmt.Errorf("yardo sets up Ubuntu (22.04 or later) and Debian (12 or later); this is %s", name)
}

func download(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 16<<20))
}
