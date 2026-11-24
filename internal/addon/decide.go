package addon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Extra is a further layer of checks, given the others' findings, or
// nil: the AI review (ReviewLayer), with the model that read it ("" not
// read).
var Extra func(ctx context.Context, h *Held, files map[string][]byte, earlier []Finding) ([]Finding, string)

// Stage, if set, is told each layer Check starts on h: basic, skill-veil
// (Extra says Claude's itself).
var Stage func(h *Held, layer string)

func (h *Held) stage(layer string) {
	if Stage != nil {
		Stage(h, layer)
	}
}

// Check runs every layer on h's files (basic, skill-veil, then Extra),
// and saves the report: its findings (layers on one line merged), what
// it can do, what each layer read and, for an update, what's new since
// the approval. taken are the names of the other skills (or servers)
// there are.
func (h *Held) Check(ctx context.Context, taken []string) error {
	files, err := h.Files()
	if err != nil {
		return err
	}
	h.stage("basic")
	r := Scan(Subject{Kind: h.Kind, Name: h.Name, Files: files, Odd: h.Odd, Pinned: h.Pin != "" || h.Existing,
		Image: h.Image, ImageUser: h.ImageUser, Taken: taken})
	h.stage("skill-veil")
	vs := Veil(ctx, h.Kind, h.Name, files)
	r.Findings = append(r.Findings, vs...)
	r.Coverage = append(r.Coverage, veilCoverage(vs, len(files)))
	ai := ""
	var more []Finding
	if Extra != nil {
		more, ai = Extra(ctx, h, files, slices.Clone(r.Findings))
	}
	r.Coverage = append(r.Coverage, h.claudeCoverage(files, r.Findings, more, ai, Extra != nil))
	r.Findings = append(r.Findings, more...)
	r.Hash, r.At, r.AI, r.Rules = Hash(files), time.Now().UTC().Truncate(time.Second), ai, Rules
	if h.Update {
		if old, err := ApprovedFiles(h.User, h.Kind, h.Name); err == nil {
			r.Changes = Changes(old, files)
		}
	}
	h.Found = r.Findings
	h.Report = r
	h.finish()
	return h.Save()
}

// finish makes h's report from its findings as found: layers on one
// line merged, each with its role and ID, an update's against what was
// approved, worst first.
func (h *Held) finish() {
	r := &h.Report
	r.Findings = merge(h.Found, r.Files)
	r.Gained, r.Lost, r.Accepted = nil, nil, nil
	if h.Update {
		if a, ok := Approved(h.User, h.Kind, h.Name); ok {
			r.Since(a)
		}
	}
	r.Sort()
}

// merge keeps one finding of each file and line the layers found: the
// worst (the first layer's, at a tie), the others' layers in its Also.
// Each finding gets its file's role and an ID, if it has none.
func merge(fs []Finding, rl map[string]string) []Finding {
	var out []Finding
	at := map[string]int{}
	for _, f := range fs {
		if f.Role == "" && f.File != "" {
			f.Role = rl[f.File]
		}
		if f.ID == "" {
			f.ID = findingID(f.Layer, f.Rule, f.File)
		}
		k := fmt.Sprintf("%s:%d", f.File, f.Line)
		n, ok := at[k]
		if f.File == "" || f.Line == 0 || !ok {
			at[k] = len(out)
			out = append(out, f)
			continue
		}
		o := &out[n]
		if o.Layer == f.Layer {
			out = append(out, f) // two rules of a layer stay two
			continue
		}
		look := o.Look || f.Look
		if rank(f.Level) > rank(o.Level) {
			f.Also = append(append(f.Also, o.Layer), o.Also...)
			*o = f
		} else if !slices.Contains(o.Also, f.Layer) {
			o.Also = append(o.Also, f.Layer)
		}
		o.Look = look
	}
	return out
}

// veilCoverage is what skill-veil read of n files, given what it found.
func veilCoverage(fs []Finding, n int) Coverage {
	for _, f := range fs {
		if f.Layer == "skill-veil" && f.Rule == "not-scanned" {
			off := strings.TrimPrefix(f.Text, "not scanned by skill-veil: ")
			if strings.Contains(off, "isn't installed") {
				off = "not installed"
			}
			return Coverage{Layer: "skill-veil", All: n, Off: off}
		}
	}
	return Coverage{Layer: "skill-veil", Files: n, All: n}
}

// claudeCoverage is what Claude read of files, given the other layers'
// findings, Claude's and the model it read with ("" it didn't); ran: it
// was asked to.
func (h *Held) claudeCoverage(files map[string][]byte, earlier, fs []Finding, ai string, ran bool) Coverage {
	switch {
	case !ran:
		return Coverage{Layer: "claude", All: len(files), Off: "not run"}
	case ai == "":
		off := "not read"
		for _, f := range fs {
			if f.Rule == "not-checked" {
				off = strings.TrimPrefix(f.Text, "Not checked by AI: ")
			}
		}
		return Coverage{Layer: "claude", All: len(files), Off: off}
	}
	return planRead(h, files, earlier).coverage()
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
	for _, f := range h.Report.Findings {
		if f.Level == "block" {
			return fmt.Errorf("%s can't be installed as it is: %s (%s)", h.Name, f.Text, f.Next)
		}
	}
	if h.Report.NeedsOK() && typed != h.Name {
		return ErrNeedsOK
	}
	if err := put(files); err != nil {
		return err
	}
	hash := h.Report.Hash
	if h.Kind == "mcp" {
		hash = SpecHash(files["spec"])
	}
	if err := Approve(h.User, Approval{Kind: h.Kind, Name: h.Name, Source: h.Source, Pin: h.Pin, Hash: hash,
		Findings: h.Report.Lines(), OK: h.Report.Verdict() == "ok", By: by,
		Accepted: h.Report.OKs(), Caps: h.Report.CapLines(), AI: h.Report.AI, Read: h.Report.ReadLines()}); err != nil {
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
