## Why

Uninstalling is built (v0.8.4) but feels weird and cumbersome. A design
review (interaction, words and symbols, mental model) found:

- **Space, the untick key, doesn't uninstall.** On an installed row it
  queues "stop managing": the tool stays. The real uninstall is `d`.
  "Remove" means stop managing in the code and uninstall in the README
  and `yardo remove`.
- **The way in the home suggests can't remove.** "Just start typing"
  opens a search where `d` and `a` are typed into the query; the home's
  "(d uninstalls)" searches for "d".
- **✗ means three things**: broken, queued to uninstall, failed. A row
  can read `[✗] ✓ AWS`.
- **The question is hard to read**: full labels, what goes said twice
  (in words, then as raw ids like `pipx:git+https://…`), what stays split
  in two, indents lost (`wrap` drops them), no y/n.
- **The result vanishes**: one footer line, cleared by any key, cut at
  the edge; failures too.
- **Too many concepts**: a VPN client and its login are two rows; what
  needs a removed tool is left installed but unchecked (and for a VPN
  login, "stays installed" is false: its client is gone); 13 items are
  refused; a team profile puts back what you uninstalled.

## What Changes

- **Untick uninstalls.** Space on an installed tool queues uninstalling
  it (`[−]`, "will uninstall"); `a` asks, then does it. `d` uninstalls the
  row now (one question). "Stop managing" becomes **Forget** (stays
  installed, yardo stops checking it), in the details panel only, and
  offered when yardo can't uninstall a tool.
- **Typing filters.** Typed text filters the list; enter or ↓ goes into
  it keeping the filter, so space, `d`, `a` work; esc clears it.
- **Symbols**: ✗ only means broken or failed. A pending row shows no
  health mark. The queue line uses words.
- **The question**: "Uninstall 2 tools?" with short names; per tool a
  line of what goes, a `Stays:` line, and what goes with it; the full
  list behind "List everything (N)"; indents kept; y/n.
- **The result stays**: a panel ("Done: 1 uninstalled, 1 failed") until
  dismissed; a failed tool offers Retry.
- **One row per tool.** A login is part of its client's row (`part_of`,
  replacing `remove_with`): the row shows its health and values; its
  details offer "Log in" or "Log out (NetBird stays)". Same for Tailscale
  and the MCP gateway's servers.
- **What needs it goes too.** Uninstalling Node also uninstalls Gemini
  CLI, said first in the question; what yardo can't uninstall is
  forgotten, and said.
- **An uninstall sticks.** A tool you uninstalled is skipped when a team
  profile is applied again, and the run says so; adding it on Tools
  clears that.
- **Fewer refusals.** Agent skills (the folders you didn't edit), the
  commit-message hook, git settings (the values still yardo's), the agent
  keys item (its file of keys stays, said) and bundles like Bitbucket
  become removable. Refused: zsh (`protected`), ECC, GitHub, commit
  signing, dotfiles, agent settings.
- **Words**: uninstall (it goes) and forget (it stays) everywhere.

## Capabilities

### Modified Capabilities
- `yardo-uninstall`: the keys, the question, the result, one row per
  tool, dependents, sticky uninstalls, fewer refusals.

## Impact

`internal/ui/board.go` (keys, filter, marks, question, result),
`internal/ui/menu.go`, `cmd/yardo/remove.go`, `cmd/yardo/main.go`,
`cmd/yardo/status.go`, `cmd/yardo/ask.go`, `internal/steps/remove.go`
(Go-step and git_config/cred undo), `skills.go`, `gitattrib.go`,
`internal/catalog` (`part_of`, `protected`), `internal/answers`
(`Declined`), `internal/profile`, catalog data, README, help.
