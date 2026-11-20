## 1. The shell

- [x] 1.1 `conf/session.zsh`: the guard, reading `~/.config/yardo/session`, attach-else-start for tmux and zellij, the one-key question (saving the answer), failure falling through with a message, no `exec`. Verify: a zsh test harness with fake `tmux` and `zellij` on PATH covering: attach when running, start when not, inside `TMUX`/`ZELLIJ`, no `SSH_CONNECTION`, `TERM_PROGRAM=vscode`, `YARDO_SESSION=off`, only one installed, neither installed, the multiplexer exiting non-zero.
- [x] 1.2 The question: `t`, `z`, `n` save and act; another key saves nothing. Verify: the harness feeding each key through a pty and asserting the file and what ran.
- [x] 1.3 `zshrc.tmpl` sources it first, before the instant prompt; the zsh config item installs the file. Verify: catalog test; the template test asserts the order.

## 2. The setting

- [x] 2.1 `answers.Session`, a Settings row "Session on login" (tmux, zellij, ask, off) whose value comes from the file; applying writes or removes `~/.config/yardo/session` as the user. Verify: `menu_test.go` row and apply; a `steps` test for the file's content and owner.
- [x] 2.2 Profiles: `sessions = "tmux" | "zellij" | "off"`, validated. Verify: `profile` test, good and bad values.

## 3. Docs and checks

- [x] 3.1 README: the shell section (what happens on ssh, the question, detach, how to skip it once and for good) and Settings. Verify: README reads with it.
- [x] 3.2 `task test` and `task lint` pass.
- [ ] 3.3 On a VM: first ssh asks; reconnect after `kill` of the ssh client lands in the same session with a running command intact; `ssh vm uptime`, scp and VS Code Remote-SSH unaffected; a new tmux pane starts nothing. Verify: the steps' output recorded in the PR.
