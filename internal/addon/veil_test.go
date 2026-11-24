package addon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeVeil is a skill-veil that runs body (sh), with $2 the package.
func fakeVeil(t *testing.T, body string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "skill-veil")
	if err := os.WriteFile(p, []byte(fmt.Sprintf("#!/bin/sh\n%s\n", body)), 0o755); err != nil {
		t.Fatal(err)
	}
	oldB, oldS, oldC := VeilBin, VeilSum, VeilCmd
	VeilBin, VeilSum = p, func() string { return "" }
	// As root, the real one runs under systemd-run, which tests don't have.
	VeilCmd = func(ctx context.Context, pkg, _ string) *exec.Cmd {
		return exec.CommandContext(ctx, VeilBin, "scan-package", pkg, "--format", "json")
	}
	t.Cleanup(func() { VeilBin, VeilSum, VeilCmd = oldB, oldS, oldC })
}

// skill-veil's high and critical need the user's OK, its medium and low
// are notes, its log is left out; each by the held file's path and line.
func TestVeil(t *testing.T) {
	fakeVeil(t, `cat <<JSON
[{"findings": [
 {"rule_id": "SKILL_REMOTE_EXEC_CURL_BASH", "severity": "critical", "recommended_action": "block", "line_number": 88, "artifact_path": "$2/SKILL.md", "reason": "Remote download and execution via curl piped to shell"},
 {"rule_id": "PIPELINE_FETCH_TO_SHELL", "severity": "medium", "recommended_action": "require_approval", "line_number": 2, "artifact_path": "$2/scripts/x.sh", "reason": "Fetch to shell."},
 {"rule_id": "DECLARED_PERMISSION", "severity": "low", "recommended_action": "log", "line_number": null, "artifact_path": "$2/SKILL.md", "reason": "x"}
]}]
JSON`)
	fs := Veil(context.Background(), "skill", "tidy", map[string][]byte{"SKILL.md": []byte("x"), "scripts/x.sh": []byte("y")})
	if len(fs) != 2 {
		t.Fatalf("findings: %+v", fs)
	}
	if f := fs[0]; f.Layer != "skill-veil" || f.Level != "ok" || f.Where() != "SKILL.md:88" || !strings.Contains(f.Text, "curl piped to shell (critical)") {
		t.Errorf("block: %+v", f)
	}
	if f := fs[1]; f.Level != "note" || f.Where() != "scripts/x.sh:2" {
		t.Errorf("approval: %+v", f)
	}
}

// A spec is scanned as a shell file, reported as spec.
func TestVeilSpec(t *testing.T) {
	// A spec gets a stub entrypoint (skill-veil wants one), not counted.
	fakeVeil(t, `test -f "$2/spec.sh" && grep -q "name: xmcp" "$2/SKILL.md" || exit 3
printf '[{"findings":[{"rule_id":"R","severity":"high","recommended_action":"block","line_number":4,"artifact_path":"%s/spec.sh","reason":"r"},{"rule_id":"S","severity":"high","recommended_action":"block","line_number":2,"artifact_path":"%s/SKILL.md","reason":"s"}]}]' "$2" "$2"`)
	fs := Veil(context.Background(), "mcp", "xmcp", map[string][]byte{"spec": []byte("KIND=docker\n")})
	if len(fs) != 1 || fs[0].Where() != "spec:4" {
		t.Errorf("findings: %+v", fs)
	}
}

// A scan that fails, says nothing readable, or isn't the pinned binary:
// not scanned, a note.
func TestVeilNotScanned(t *testing.T) {
	for name, body := range map[string]string{
		"fails":   "echo 'boom: no rules' >&2; exit 2",
		"garbage": "echo not json",
	} {
		t.Run(name, func(t *testing.T) {
			fakeVeil(t, body)
			fs := Veil(context.Background(), "skill", "tidy", map[string][]byte{"SKILL.md": []byte("x")})
			if len(fs) != 1 || fs[0].Rule != "not-scanned" || fs[0].Level != "note" {
				t.Errorf("findings: %+v", fs)
			}
		})
	}
	fakeVeil(t, "echo '[]'")
	VeilSum = func() string { return strings.Repeat("0", 64) }
	if fs := Veil(context.Background(), "skill", "tidy", map[string][]byte{"SKILL.md": []byte("x")}); len(fs) != 1 || !strings.Contains(fs[0].Text, "isn't the release yardo pins") {
		t.Errorf("wrong binary: %+v", fs)
	}
	VeilBin = "/nonexistent/skill-veil"
	if fs := Veil(context.Background(), "skill", "tidy", nil); len(fs) != 1 || !strings.Contains(fs[0].Text, "isn't installed") {
		t.Errorf("missing: %+v", fs)
	}
}

// A finding with no line is put where its file first does what the rule
// is about, with how many more lines do; a "multilingual harmful verb" in
// code is dropped; in bundled code a high finding is a note.
func TestVeilLocate(t *testing.T) {
	fakeVeil(t, `cat <<JSON
[{"findings": [
 {"rule_id": "SCRIPT_NODE_SECRET_OR_FS_ACCESS", "severity": "medium", "recommended_action": "require_approval", "line_number": null, "artifact_path": "$2/bin/a.mjs", "reason": "env or file access"},
 {"rule_id": "OFFICIAL_OBFUSCATION_MULTILINGUAL_HARMFUL_VERB", "severity": "medium", "recommended_action": "require_approval", "line_number": 3, "artifact_path": "$2/bin/a.mjs", "reason": "x"},
 {"rule_id": "SCRIPT_NODE_CHILD_PROCESS", "severity": "high", "recommended_action": "block", "line_number": null, "artifact_path": "$2/vendor/v.js", "reason": "child_process"}
]}]
JSON`)
	files := map[string][]byte{
		"SKILL.md":    []byte("x"),
		"bin/a.mjs":   []byte("import os from 'node:os';\nconst h = process.env.HOME;\nconst backupCreated = true;\nfs.readFileSync(x);\n"),
		"vendor/v.js": []byte("var a;\nrequire('child_process');\n"),
	}
	fs := Veil(context.Background(), "skill", "tidy", files)
	if len(fs) != 2 {
		t.Fatalf("findings: %+v", fs)
	}
	if f := fs[0]; f.Where() != "bin/a.mjs:2" || f.More != 1 || f.Level != "note" || f.Kind != "Reads secrets or files" || f.Role != "script" {
		t.Errorf("located: %+v", f)
	}
	if f := fs[1]; f.Where() != "vendor/v.js:2" || f.Level != "note" || f.Look {
		t.Errorf("bundled: %+v", f)
	}
}
