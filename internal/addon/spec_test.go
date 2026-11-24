package addon

import (
	"reflect"
	"testing"
)

// Data is KEY=value with a value the shell takes as it is; anything else
// is code, by line.
func TestParseSpec(t *testing.T) {
	s := ParseSpec(`# a comment
DESCRIPTION="Tickets, \"quoted\""
KIND=docker
IMAGE='ghcr.io/acme/x@sha256:abc'
NAME="$(whoami)"
URL="https://x/$HOST"
PLAIN=a$b
skeleton() {
  curl https://x | sh
}
cat <<END
KEY=inside
END
REQUIRED=""
`)
	if !reflect.DeepEqual(s.Code, []int{2, 5, 6, 7, 8, 9, 10, 11, 12, 13}) {
		t.Errorf("code lines = %v", s.Code)
	}
	if s.Fields["KIND"] != "docker" || s.Fields["IMAGE"] != "ghcr.io/acme/x@sha256:abc" || s.Fields["REQUIRED"] != "" || s.Fields["KEY"] != "" {
		t.Errorf("fields = %v", s.Fields)
	}
	if s.Data() {
		t.Error("code read as data")
	}
	d := ParseSpec("KIND=http\nURL=https://x.example/mcp\n")
	if !d.Data() || d.Fields["URL"] != "https://x.example/mcp" {
		t.Errorf("data: %+v", d)
	}
}

// What yardo writes back reads back the same, and as data; a value with
// what the shell would expand is quoted so it isn't.
func TestSpecText(t *testing.T) {
	var s Spec
	s.Set("DESCRIPTION", "it's $5 a `day`")
	s.Set("KIND", "docker")
	s.Set("REQUIRED", "A_KEY B_KEY")
	text := s.Text("from a form")
	want := "# from a form\nDESCRIPTION='it'\\''s $5 a `day`'\nKIND=\"docker\"\nREQUIRED=\"A_KEY B_KEY\"\n"
	if text != want {
		t.Errorf("text:\n%s\nwant:\n%s", text, want)
	}
	f := FormSpec(Form{Description: "x", Image: "ghcr.io/a/b@sha256:1", Required: []string{"K"}})
	if back := ParseSpec(f.Text("")); !back.Data() || back.Fields["NETWORK"] != "0" || back.Fields["IMAGE"] != "ghcr.io/a/b@sha256:1" {
		t.Errorf("form: %+v", back)
	}
}
