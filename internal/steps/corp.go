package steps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eugenetaranov/yardo/internal/answers"
	"github.com/eugenetaranov/yardo/internal/catalog"
)

func init() {
	RegisterGo("corpca", func(catalog.Step) Step { return corpCAStep{} })
}

// --- go:corpca ---------------------------------------------------------------

// corpCAStep installs an internal CA bundle into the system store (one
// file per certificate), so curl, apt, Docker and git trust it; shell.sh
// points Node, Python requests and the AWS CLI at the store. It runs with
// the apt repositories, so apt can reach an internal mirror in the same run.
type corpCAStep struct{}

const (
	caDir    = "/usr/local/share/ca-certificates"
	caPrefix = "yardo-corp-"
)

func (corpCAStep) BeforeApt() bool { return true }

func caOf(e *Env) answers.CA {
	if e.Answers == nil {
		return answers.CA{}
	}
	return e.Answers.CA
}

func (corpCAStep) Plan(e *Env) []Action {
	if caOf(e).Source == "" {
		return []Action{{"note", "no CA bundle set"}}
	}
	return []Action{{"file", caDir + "/" + caPrefix + "*.crt (from " + caOf(e).Source + ")"}, {"run", "update-ca-certificates"}}
}

// certs fetches the bundle, checks it, and splits it into PEM certificates.
func (corpCAStep) certs(ctx context.Context, e *Env) ([][]byte, error) {
	c := caOf(e)
	var b []byte
	var err error
	if strings.HasPrefix(c.Source, "https://") {
		if e.Download == nil {
			return nil, errors.New("no downloader")
		}
		if b, err = e.Download(ctx, c.Source); err != nil {
			return nil, err
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != c.SHA256 {
			return nil, fmt.Errorf("%s: sha256 is %s, not the %s given", c.Source, got, c.SHA256)
		}
	} else if b, err = os.ReadFile(e.Path(c.Source)); err != nil {
		return nil, err
	}
	var out [][]byte
	for rest := b; ; {
		var blk *pem.Block
		blk, rest = pem.Decode(rest)
		if blk == nil {
			break
		}
		if blk.Type == "CERTIFICATE" {
			out = append(out, pem.EncodeToMemory(blk))
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no PEM certificates in it", c.Source)
	}
	return out, nil
}

func (s corpCAStep) want(ctx context.Context, e *Env) ([]wantFile, error) {
	certs, err := s.certs(ctx, e)
	if err != nil {
		return nil, err
	}
	var out []wantFile
	for i, c := range certs {
		out = append(out, wantFile{filepath.Join(caDir, fmt.Sprintf("%s%d.crt", caPrefix, i+1)), c, 0o644})
	}
	return out, nil
}

// stale lists the yardo certificates no longer in the bundle.
func stale(e *Env, keep []wantFile) []string {
	have, _ := filepath.Glob(e.Path(filepath.Join(caDir, caPrefix+"*.crt")))
	var out []string
	for _, p := range have {
		found := false
		for _, w := range keep {
			if e.Path(w.to) == p {
				found = true
			}
		}
		if !found {
			out = append(out, p)
		}
	}
	return out
}

func (s corpCAStep) Check(ctx context.Context, e *Env) (bool, error) {
	if caOf(e).Source == "" {
		return true, nil
	}
	want, err := s.want(ctx, e)
	if err != nil {
		return false, err
	}
	for _, w := range want {
		if !same(e.Path(w.to), w) {
			return false, nil
		}
	}
	return len(stale(e, want)) == 0, nil
}

func (s corpCAStep) Apply(ctx context.Context, e *Env) error {
	if caOf(e).Source == "" {
		return nil
	}
	want, err := s.want(ctx, e)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(e.Path(caDir), 0o755); err != nil {
		return err
	}
	for _, p := range stale(e, want) {
		if err := os.Remove(p); err != nil {
			return err
		}
	}
	for _, w := range want {
		if err := writeFile(e.Path(w.to), w.b, w.mode); err != nil {
			return err
		}
	}
	if _, err := e.run(ctx, Cmd{Name: "update-ca-certificates", Args: []string{"--fresh"}}); err != nil {
		return err
	}
	if root(ctx, e, "dockerd") {
		_, err := e.run(ctx, Cmd{Name: "systemctl", Args: []string{"try-restart", "docker"}})
		return err
	}
	return nil
}
