# Proposal

## Why

yardo's screens each read well, but the flow between them has seams: the
first run keeps asking questions after you said Install; a run's output,
and the new SSH key you must paste into GitHub, end up hidden behind the
home; the same key means a different thing on each screen; updating takes
three enters; and every screen is its own program, so each hop flickers
and drops the Tools queue. A revisit of the whole flow fixes these
together, so the fixes agree on one key map and one place for output.

## What Changes

- **The first run asks everything before "Ready?".** Choices, then the
  questions those choices need (git name and email, timezone, where the
  gateway listens, Docker's ports, wiring agents, the dotfiles repo, API
  keys, a key per git host), then the review, which now shows those
  answers, and **Install** starts the run at once. Customize… opens the
  board with it all queued, and `a` there goes straight to the review.
- **The last run stays reachable.** A run's output no longer needs `q` to
  be seen: the home gets a **Last run** row when the run failed (opening
  what it printed, from the log), and a run's new public keys are on the
  Connection screen, under **Keys to add**, until the host has them (a
  git host) or you dismiss them. The toast after a run stops saying to
  quit to see the output.
- **One key map across the screens.** `a` applies everywhere (Tools,
  Network access, Look, MCP servers; adding an MCP server moves to `n`);
  `s` is gone. ←→ switch tabs on Tools and pick a choice on a settings
  row; on the home they walk the Go to strip only, and on Fix the details
  open with enter on the row's note. `d` shows in Tools' footer. `h` `j`
  `k` `l` work wherever arrows do.
- **Updates in one step.** "Install 3 updates" opens the list, and enter
  there updates; `q` backs out. The "Update all" button and the second
  question go.
- **One program.** The home, the Tools board, Settings, Fix, Updates,
  Connection and the MCP servers screens are views of one program with
  one alt screen: no flicker between them, the Tools queue and each tab's
  cursor kept as you go home and back, search typed on the home live on
  the board. Prompts that ask a question (a name, a key, a choice) and the
  run itself still leave the screen, as today.
- **At home for tmux, Midnight Commander and zellij users.** Every screen
  has the same two fixed lines: a header with the path (`Home › Tools ›
  Dev`) and a footer listing every key the screen takes, like zellij's
  status bar and MC's function-key bar. `?` opens a keys screen, as `?`
  does in tmux. Every list pages the same way: PgUp/PgDn, Home/End, `g`
  and `G`. Every question is one framed dialog answered with `y`/`n` or
  enter, cancelled with `q` or esc. The cursor row is one style
  everywhere (the board's `>` pointer goes). The mouse works: click a row
  to select it, double-click to act, click a tab or a Go to entry to go
  there, click a key in the footer to press it, and the wheel scrolls.
- **Smaller:** `Connection` is one name, on Go to and on its panel, and
  enter on the panel opens it. **Check for updates** moves to the home
  (`u`, and a row in Needs you while updates haven't been checked); it
  leaves Settings. The run's spinner says where it is: `12 of 43 ·
  Docker Engine (1m20s)`. The Machine panel folds to its two-line form at
  every width, so Needs you has the full row. README's home sample and
  the first-run text are brought up to date.

## Capabilities

### New Capabilities

- `yardo-first-run`: what the first run asks, in what order, what the
  review shows, and what Install and Customize… do.
- `yardo-tui-screens`: the one program: its screens, how they're reached
  and left, what each keeps when you leave it, the key map shared by all
  of them, the header, footer, dialog and cursor every screen has, the
  keys screen, paging and the mouse, and the home's layout (Needs you,
  Machine, Connection, Go to, Check for updates).
- `yardo-last-run`: how a run's result reaches you after the screen comes
  back: the Last run row, the Keys to add list, the spinner's count.

### Modified Capabilities

- `yardo-uninstall`: "The board queues uninstalls" gains `d` in the
  footer and keeps `a` as the apply key, stated as the shared one;
  "Typing filters the board" says the filter typed on the home is live
  on the board without the screen restarting.
- `yardo-runner`: "Log and summary" adds the step count in the progress
  line and that the terminal's post-run text is also reachable from the
  home.
- `yardo-cli`: "Picker parity" is replaced: the picker it describes
  (number keys for tabs, Esc quits) is the bash picker's, which the Tools
  board no longer mirrors; the requirement now points at
  `yardo-tui-screens` for the keys and keeps the search ranking.

## Impact

- `cmd/yardo/main.go` (`runUser`): the first-run order (questions before
  review); the loop over screens becomes one program's result handling;
  the post-run toast; reading the run's output back for Last run.
- `cmd/yardo/ask.go`, `cmd/yardo/choices.go`: `review` shows the
  answers; `questions` runs before it on a first run.
- `internal/ui/menu.go`, `internal/ui/board.go`, `internal/ui/mcpmenu.go`,
  `internal/ui/home.go`: one `tea.Model` holding the menu's and the
  board's state as views; the key map; the keys screen; the shared
  header, footer, dialog and cursor row; paging; mouse events
  (`tea.WithMouseCellMotion`, hit areas for tabs, rows, Go to and the
  footer); the Updates screen; `u`; the Last run screen; the Machine
  fold; the Connection panel.
- `internal/run/live.go`, `internal/run/run.go`: the step count in the
  spinner label.
- `cmd/yardo/connect.go`: Keys to add on the Connection screen, read from
  the state's `.keys` file; cleared per host once its key is known to be
  added, or on dismiss.
- `cmd/yardo/status.go`: `screenData` keeps the board's queue and cursors
  across a hop to the home.
- `README.md`: the home sample, the first-run paragraph, the keys.
- Tests in `internal/ui/*_test.go`, `cmd/yardo/*_test.go`.
