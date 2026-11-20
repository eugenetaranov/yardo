## Context

- `zshrc.tmpl` is rewritten by yardo on each run; users' own lines live
  in `~/.zshrc.local`. Its first lines source Powerlevel10k's instant
  prompt, which complains about anything printed or read after it.
- tmux is an apt package and zellij a mise tool; both are on by default.
  tmux sets `TMUX` in its panes, zellij sets `ZELLIJ`.
- Settings rows (`BoardSetting`) hold a value and choices and are applied
  by a run (Network access, Look).

## Goals / Non-Goals

**Goals:**
- A dropped connection never loses work, with nothing to remember.
- Never in the way of scripts, editors, agents or nested shells.

**Non-Goals:**
- Session naming, layouts, or picking among several sessions.
- bash.

## Decisions

- **One file, sourced first.** `~/.config/yardo/session.zsh`, sourced at
  the very top of `.zshrc`, before the instant prompt, since it may ask a
  question and hand the terminal to tmux. It defines no functions that
  stay and sets no options.
- **The guard, in this order:** `[[ -o interactive ]]`, a tty on stdin
  and stdout, `SSH_CONNECTION` set, `TMUX` and `ZELLIJ` unset,
  `TERM_PROGRAM` not `vscode`, no `VSCODE_*`/`CURSOR_*`/`JETBRAINS_*`
  injection variables, `TERM` not `dumb`, and `YARDO_SESSION != off`
  (an escape hatch for one-off use from `~/.zshenv` or `ssh -t vm
  YARDO_SESSION=off zsh`). `ssh vm cmd` is non-interactive, so it never
  gets here.
- **The answer lives in a file the shell and yardo share:**
  `~/.config/yardo/session` holding `tmux`, `zellij` or `off` (missing:
  ask). The login question writes it; the Settings row reads it for its
  value and an apply writes it. Alternative considered: only in
  `answers.json`. Rejected: it's root's, so the login shell couldn't save
  the answer.
- **The question is one line and one key**, read with `read -k1` from the
  tty: `Session: [t]mux  [z]ellij  [n]ever (change in sudo yardo ›
  Settings)`. Any other key means "not now": plain shell this time, asked
  again next login.
- **Attach, else start:**
  - tmux: `tmux attach` (the last session used) when `tmux ls` has one,
    else `tmux new -s main`.
  - zellij: the first session `zellij list-sessions --short` gives that
    isn't exited, with `zellij attach <name>`; else `zellij attach -c
    main`.
  - Both running, nothing set: the question, worded "attach to".
- **No `exec`.** The multiplexer runs as a child, so detaching returns to
  zsh and a failure falls through to the prompt with a message.
  Trade-off: leaving takes a detach and then an exit. Alternative: `exec`,
  where detach logs out. Rejected as the default because a broken tmux or
  zellij config would lock the user out of ssh.
- **A second ssh while attached elsewhere** attaches the same session
  (tmux and zellij both mirror). That's the point on reconnect, and the
  simple rule otherwise.
- **The setting:** `answers.Session`, a `BoardSetting` with tmux, zellij,
  ask, off; `screenData` reads the file so the login's answer shows; the
  apply writes or removes the file as the user. Profiles: `sessions`.
- **Existing machines:** the file arrives with the next run's zsh
  config. The first login after asks, and `n` ends it for good.

## Risks / Trade-offs

- **A question at login surprises.** It's one line, one key, once, and
  "never" is one of the keys.
- **Tools that open an interactive ssh shell and type into it** (some
  deploy scripts, `expect`). → The guard needs a real tty; `YARDO_SESSION=off`
  and the off setting cover the rest; the README says so.
- **Editors change their variables.** → The check is a short list in one
  place; an editor that slips through gets the question, and "never" or
  off fixes it.
- **zellij's list output differs between versions.** → Fall back to
  `zellij attach -c main` when it can't be read.
