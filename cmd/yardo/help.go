package main

import (
	"fmt"
	"io"
	"slices"
)

// usage is yardo --help: every command, and the flags that matter.
const usage = `yardo sets up this Linux machine (Ubuntu or Debian) for you and your
coding agents: tick what you want, and it installs and configures it.

Usage:
  sudo yardo                   a short menu: fix, fill in, add or remove tools,
                               who can connect (the checklist on a first run)
  sudo yardo --yes             re-apply everything chosen so far, no questions
  yardo --dry-run              ask, then show what would happen; change nothing
  sudo yardo --profile FILE|URL [--yes]
                               apply a profile (repeatable; later ones win)

Commands:
  status [--json] [--updates]  what's installed, down, partial or waiting for a value
  export [--name N] [--lock] [FILE]
                               write the saved choices as a profile
  connect                      how to reach this machine from a laptop
  gitkey [--host H]            set up the SSH key for git sent on stdin
  version [up [--check]]       the version; upgrade to the latest release
  forget                       remove the keys, logins and secrets yardo set up

Flags:
  -y, --yes          no questions: re-apply the saved choices (or the profiles')
  -n, --dry-run      show what would happen; change nothing
      --verify       check that the saved items are installed; exit 1 if not
      --list         list the catalog's items (id, tab, label), for profiles
      --profile SRC  apply a profile: a file or an https URL (repeatable)
      --user NAME    the account to set up, created if missing
      --ui board     open on the tabbed checklist instead of the short menu
      --state DIR    state directory (default /var/lib/yardo)
      --log FILE     log of every command and its output (default /var/log/yardo.log)
      --version      print the version

Examples:
  sudo yardo export --name team > team.toml
  sudo yardo --profile team.toml --yes --user ubuntu
  ssh me@vm 'sudo yardo gitkey' < ~/.ssh/deploy_key

yardo remembers what you picked (~/.config/yardo/answers.json), and
~/yardo.txt lists how each tool was installed and how to manage it.
More: https://github.com/eugenetaranov/yardo
`

// subUsage is each command's --help.
var subUsage = map[string]string{
	"status": `Usage: sudo yardo status [--json] [--updates]

Checks every item you picked, changing nothing: down, partial, missing,
drifted, values to fill in, then ok. Without sudo it shows the last
check's copy (~/.cache/yardo/status.json).

  --json     print JSON (schema 1) for scripts
  --updates  also check for newer versions (apt and mise; slow)
`,
	"export": `Usage: sudo yardo export [--name NAME] [--lock] [FILE]

Writes the saved choices as a profile (TOML; never secrets), to FILE or
stdout. Apply it elsewhere with: sudo yardo --profile FILE --yes

  --name NAME  name the profile: applying a changed one then drops the
               picks it no longer lists (an unnamed profile only adds)
  --lock       add a [lock] table with the installed mise and apt versions
`,
	"connect": `Usage: sudo yardo connect

Prints how to reach this machine from a laptop: Docker over SSH, the MCP
gateway's URL and token, or an SSH tunnel to it.
`,
	"gitkey": `Usage: ssh me@vm 'sudo yardo gitkey [--host HOST]' < KEYFILE

Sets up the SSH private key on stdin for git: ~/.ssh/yardo_git_key, used
for HOST in ~/.ssh/config.

  --host HOST  github.com (default), bitbucket.org, your GitLab or another
               (repeatable)
`,
	"version": `Usage: yardo version [up [--check] [--yes] [--version vX.Y.Z]]

Prints the version. "up" (or "update", "upgrade") upgrades to the latest
release, checked against its checksums.txt (as root; a Homebrew install
upgrades through brew).

  --check    only say whether a newer release is out
  --yes      upgrade without asking
  --version  install this release instead of the latest
`,
}

// wantsHelp says whether args ask for yardo's help (forget's is the
// bash part's).
func wantsHelp(args []string) bool {
	if len(args) > 0 && args[0] == "help" {
		return true
	}
	for _, a := range args {
		switch a {
		case "--", "forget":
			return false
		case "-h", "--help", "-help":
			return true
		}
	}
	return false
}

// printHelp writes the help for the command in args, or yardo's own.
func printHelp(w io.Writer, args []string) {
	for _, a := range args {
		if u, ok := subUsage[a]; ok {
			fmt.Fprint(w, u)
			return
		}
		if !slices.Contains([]string{"-h", "--help", "-help", "help"}, a) && a != "" && a[0] != '-' {
			break // an argument we don't know: yardo's own help
		}
	}
	fmt.Fprint(w, usage)
}
