package addon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Extra is a further layer of checks, given the others' findings, or
// nil: the AI review (ReviewLayer), with the model that read it ("" not
// read).
var Extra func(ctx context.Context, h *Held, files map[string][]byte, earlier []Finding) ([]Finding, string)

// Check runs every layer on h's files (basic, skill-veil, then Extra),
// and saves the report. taken are the names of the other skills (or
// servers) there are.
func (h *Held) Check(ctx context.Context, taken []string) error {
	files, err := h.Files()
	if err != nil {
		return err
	}
	fs := Basic(Subject{Kind: h.Kind, Name: h.Name, Files: files, Odd: h.Odd, Pinned: h.Pin != "" || h.Existing,
		Image: h.Image, ImageUser: h.ImageUser, Taken: taken})
	fs = append(fs, Veil(ctx, h.Kind, h.Name, files)...)
	ai := ""
	if Extra != nil {
		var more []Finding
		more, ai = Extra(ctx, h, files, slices.Clone(fs))
		fs = append(fs, more...)
	}
	h.Report = Report{Hash: Hash(files), At: time.Now().UTC().Truncate(time.Second), Findings: fs, AI: ai, Rules: Rules}
	if h.Update {
		if old, err := ApprovedFiles(h.User, h.Kind, h.Name); err == nil {
			h.Report.Changes = Changes(old, files)
		}
	}
	h.Report.Sort()
	return h.Save()
}

// ErrChanged: the held files aren't the ones checked; check again.
var ErrChanged = errors.New("its files changed since they were checked: checked again, see the report")

// ErrNeedsOK: the report needs the user's OK, the add-on's name typed.
var ErrNeedsOK = errors.New("its report needs your OK: type its name to install it")

// Install installs h with put, given the very files that were checked,
// records the approval (by, whether typed is h's name) and lets go of h.
// A report that blocks refuses; one that needs an OK wants typed to be
// h's name; files changed since the check are ErrChanged (check again).
func (h *Held) Install(ctx context.Context, by, typed string, put func(files map[string][]byte) error) error {
	files, err := h.Files()
	if err != nil {
		return err
	}
	if !h.Report.Current() || Hash(files) != h.Report.Hash {
		return ErrChanged
	}
	switch h.Report.Verdict() {
	case "block":
		for _, f := range h.Report.Findings {
			if f.Level == "block" {
				return fmt.Errorf("%s can't be installed as it is: %s (%s)", h.Name, f.Text, f.Next)
			}
		}
	case "ok":
		if typed != h.Name {
			return ErrNeedsOK
		}
	}
	if err := put(files); err != nil {
		return err
	}
	hash := h.Report.Hash
	if h.Kind == "mcp" {
		hash = SpecHash(files["spec"])
	}
	if err := Approve(h.User, Approval{Kind: h.Kind, Name: h.Name, Source: h.Source, Pin: h.Pin, Hash: hash,
		Findings: h.Report.Lines(), OK: h.Report.Verdict() == "ok", By: by}); err != nil {
		return err
	}
	if err := KeepCopy(h.User, h.Kind, h.Name, files); err != nil {
		return err
	}
	return h.Remove()
}

// SpecHash is an MCP spec's hash, as mcp-sync checks it (sha256sum).
func SpecHash(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Decline removes h: its files, and the image pulled for it (unless
// something else uses it: the error says so, the files are gone anyway).
func (h *Held) Decline(ctx context.Context) error {
	if err := h.Remove(); err != nil {
		return err
	}
	if h.Pulled && h.Pin != "" {
		return RemoveImage(ctx, h.Pin)
	}
	return nil
}
