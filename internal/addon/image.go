package addon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
)

// Docker is the docker command; tests put a fake one first on PATH.
var Docker = "docker"

var imageRef = regexp.MustCompile(`^[a-z0-9][a-z0-9._/:@-]*$`)

// Image is a pulled image: Ref pinned to its digest (repo@sha256:…), and
// the user it runs as ("" is root).
type Image struct {
	Ref, User string
	// New: it wasn't here before the pull.
	New bool
}

// PullImage pulls ref and reads its digest and config. It never creates
// or runs a container: an image held is only on disk.
func PullImage(ctx context.Context, ref string) (Image, error) {
	if !imageRef.MatchString(ref) || strings.Contains(ref, "..") {
		return Image{}, fmt.Errorf("%q: not an image name", ref)
	}
	had := exec.CommandContext(ctx, Docker, "image", "inspect", "--format", "{{.Id}}", ref).Run() == nil
	if out, err := exec.CommandContext(ctx, Docker, "pull", "--quiet", ref).CombinedOutput(); err != nil {
		return Image{}, fmt.Errorf("docker pull %s: %s", ref, lastLine(out, err))
	}
	out, err := exec.CommandContext(ctx, Docker, "image", "inspect", "--format", "{{json .}}", ref).Output()
	if err != nil {
		return Image{}, fmt.Errorf("docker image inspect %s: %w", ref, err)
	}
	var in struct {
		RepoDigests []string
		Config      struct{ User string }
	}
	if err := json.Unmarshal(out, &in); err != nil {
		return Image{}, err
	}
	repo := ref
	if i := strings.LastIndex(repo, "@"); i >= 0 {
		repo = repo[:i]
	}
	if i := strings.LastIndex(repo, ":"); i > strings.LastIndex(repo, "/") {
		repo = repo[:i]
	}
	for _, d := range in.RepoDigests {
		if r, digest, ok := strings.Cut(d, "@"); ok && (r == repo || strings.HasSuffix(r, "/"+repo)) {
			return Image{Ref: r + "@" + digest, User: in.Config.User, New: !had}, nil
		}
	}
	return Image{}, fmt.Errorf("%s: no digest to pin it by (a local build?): refused", ref)
}

// RemoveImage removes a pulled image (declined, or no longer held).
func RemoveImage(ctx context.Context, ref string) error {
	if out, err := exec.CommandContext(ctx, Docker, "image", "rm", ref).CombinedOutput(); err != nil {
		return fmt.Errorf("docker image rm %s: %s", ref, lastLine(out, err))
	}
	return nil
}

func lastLine(out []byte, err error) string {
	ls := strings.Split(strings.TrimSpace(string(out)), "\n")
	if l := ls[len(ls)-1]; l != "" {
		return l
	}
	return err.Error()
}

// CheckURL is u when it's an https address with a host, or why not.
func CheckURL(u string) (string, error) {
	p, err := url.Parse(strings.TrimSpace(u))
	switch {
	case err != nil:
		return "", err
	case p.Scheme != "https":
		return "", fmt.Errorf("%s: the address must be https", u)
	case p.Host == "" || p.User != nil:
		return "", fmt.Errorf("%s: not a server's address", u)
	}
	return p.String(), nil
}
