## Why

Work on a remote machine should survive a dropped connection, and the way
to get that is tmux or zellij. yardo installs both, but you have to
remember to start one, and to reattach after every reconnect. Forget once,
the Wi-Fi drops, and the build or the agent that was running is gone.

## What Changes

- **ssh in, and you're in your session.** When you connect over ssh to a
  machine with yardo's zsh setup:
  - a tmux or zellij session of yours is already running: the shell
    attaches to it;
  - none is running: it starts one.
- **It asks which, once.** The first time there's nothing to attach to and
  both are installed, one line asks: tmux, zellij, or never. One key
  answers, and the answer is remembered. With only one of them installed
  there's no question.
- **Never one inside the other.** Inside tmux or zellij (a new pane, a
  new tab, a nested ssh back to the same machine) nothing is started or
  attached.
- **Only for a person at a terminal over ssh.** Not for `ssh vm command`,
  scp, rsync or git; not on the console; not in an editor's remote
  terminal (VS Code, Cursor, JetBrains); not for a shell an agent starts.
- **Detaching leaves you in the plain shell** on the machine, still
  connected. If tmux or zellij can't start, you get the plain shell and a
  line saying why, never a closed connection.
- **A setting:** Settings → "Session on login": tmux, zellij, ask, or
  off. It shows and changes the same answer the question saved.
  `sessions = "tmux" | "zellij" | "off"` in a profile sets it for a team.
- Machines set up before get it on their next yardo run (the zsh config
  is rewritten then); the first ssh login after that asks.

Not in this change: bash (yardo's bash setup stays as it is), naming or
choosing between several sessions (it attaches to the last one used),
and mosh-specific handling (mosh logs in through ssh, so it works the
same).

## Capabilities

### New Capabilities

### Modified Capabilities
- `yardo-shell-config`: the zsh setup attaches to, or starts, a tmux or
  zellij session on an ssh login.

## Impact

- `internal/assets/conf/session.zsh` (new), sourced first in yardo's zsh
  setup (`zshrc.tmpl`), before Powerlevel10k's instant prompt. Lands
  either side of `zsh-own-config`.
- `internal/catalog/data/20-shell.toml`: the zsh config item installs it.
- `internal/answers`: `Session`; `internal/profile`: `sessions`;
  Settings row in `cmd/yardo/status.go` and `internal/ui/menu.go`; the
  apply writes `~/.config/yardo/session`.
- README: the shell section and Settings.
