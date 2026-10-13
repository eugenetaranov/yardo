// Package assets holds what yardo ships besides its Go code: the config
// files its steps install (conf/), the MCP server catalog and mcp-sync,
// mise's registry snapshot, and the bash installer (yardo.sh, picker.py)
// that still runs a few items until they are ported. The Go code reads
// them from here; Extract writes them to ShareDir for the bash part.
//
// mcp-catalog/ and mcp-sync are copies from github.com/tackhq/tack-roles
// (see SOURCE), so yardo and the tack roles install the same MCP servers.
package assets

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed conf mcp-catalog mcp-sync mise-registry.tsv yardo.sh picker.py
var files embed.FS

// ShareDir is where Extract puts the files on the machine.
const ShareDir = "/usr/local/share/yardo"

// LegacyScript is the bash installer's name in ShareDir.
const LegacyScript = "yardo.sh"

// FS is every shipped file.
func FS() fs.FS { return files }

// Conf is yardo's own config files, for file steps.
func Conf() fs.FS {
	s, err := fs.Sub(files, "conf")
	if err != nil {
		panic(err) // embedded; cannot fail
	}
	return s
}

// MiseRegistry is the snapshot of mise's registry (name, aliases,
// description per line), for the picker's search.
func MiseRegistry() []byte {
	b, _ := files.ReadFile("mise-registry.tsv")
	return b
}

// stamp names the version the files in a share dir came from.
const stamp = ".version"

// Extract writes the files to dir (0755 directories; yardo.sh and mcp-sync
// executable), unless dir already holds this version's. Files dropped
// since are removed with the old tree.
func Extract(dir, version string) error {
	if b, err := os.ReadFile(filepath.Join(dir, stamp)); err == nil && bytes.Equal(bytes.TrimSpace(b), []byte(version)) && version != "dev" {
		return nil
	}
	tmp := dir + ".new"
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	err := fs.WalkDir(files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(tmp, p)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if p == LegacyScript || p == "mcp-sync" {
			mode = 0o755
		}
		return os.WriteFile(dst, b, mode)
	})
	if err == nil {
		err = os.WriteFile(filepath.Join(tmp, stamp), []byte(version+"\n"), 0o644)
	}
	if err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("extract yardo's files to %s: %w", dir, err)
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.Rename(tmp, dir)
}
