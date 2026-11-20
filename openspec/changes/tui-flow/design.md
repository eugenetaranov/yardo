# Design

## Context

See proposal.md for why. What shapes the approach:

- `cmd/yardo/main.go`'s `runUser` is a loop: build the screen's data
  (`screenData`, which checks every item), run one Bubble Tea program
  (`ui.Menu.Run` for the home and its screens, or `ui.Board.Run` for
  Tools), act on its result (a run, a prompt, a hop to the other
  program), repeat. Each `Run` is its own alt screen. State that must
  survive a hop is threaded by hand (`reopen`, `toast`, `fromBoard`,
  `search`, `flash`, `done`), and the board's queue is dropped on
  `BoardQuit` (`d.forget()`).
- `ui.Menu` (menu.go, home.go, mcpmenu.go) and `ui.Board` (board.go) are
  each a `tea.Model` with their own key handling, header and footer.
  Their tests (`menu_test.go`, `board_test.go`) drive `Update` and
  `View` directly.
- A first run is `askChoices` (one Bubble Tea screen), `review` (plain
  text and a `huh` choice), then `fromBoard` → `questions` (`huh`
  prompts), then the run. Later runs' questions come from the same
  `questions`.
- The run is line output: `run.Live` (a spinner per step) and plain
  result lines. After it, `printKeys`, notes and hints go to stdout, and
  the next program's alt screen covers them.
- Prompts (`huh`) and the run need the terminal in line mode; they can't
  live inside the alt-screen program without a rewrite of every prompt.

## Goals / Non-Goals

**Goals:**

- One Bubble Tea program for every screen between prompts and runs, with
  the existing `Menu` and `Board` models kept as its views, so their
  tests keep working.
- One key map, one header, one footer, one dialog, one cursor, one way to
  page, and the mouse, implemented once and used by every view.
- The first run's questions before its review.
- The run's output kept and shown inside the program.

**Non-Goals:**

- Moving `huh` prompts or the run's progress into the alt screen.
- Changing what the catalog asks or installs.
- A help text beyond the keys screen.
- Number keys for tabs.

## Decisions

### One program: `ui.App` composes `Menu` and `Board`

`ui.App` is the `tea.Model` the loop runs. It holds `*Menu`, `*Board`,
`view` (`menu` or `board`) and the shared chrome. `Update` forwards keys
and mouse events to the current view after handling the shared ones
(`?`, ctrl+c, paging, mouse hits on the chrome); `View` draws the
header, the view's body, the toast line and the footer. `Menu` and
`Board` stop drawing a header and footer of their own: each exposes
`Path() string`, `Keys() []Key` and `Body(w, h int) string`.

A hop between views that today ends the program (`MenuTools`,
`BoardQuit` with `fromMenu`, `BoardChange "mcp"`) becomes `app.view =
…`. The results that need the terminal (`MenuFix`, `MenuEdit`,
`MenuChange`, `MenuApply`, `MenuUpgrade`, `MenuUpdates`, `MenuMCP`,
`BoardFix`, `BoardEdit`, `BoardChange`, `BoardApply`, quit) end the
program as now, as one `AppResult` that wraps either kind. `runUser`'s
`switch` bodies stay; its `onBoard`/`reopen`/`fromBoard`/`search`/`flash`
plumbing goes, replaced by `App.Open(view, screen)` and `App.Toast`.

Why not merge `Menu` and `Board` into one struct: 2,000 lines of key
handling and 1,600 of tests would move for no behaviour. Why not keep two
programs and hide the flicker: Bubble Tea leaves the alt screen on
`Program.Run` returning; there is no way to hand it over.

State across runs: when the program ends for a run, `runUser` keeps the
`App` and gives it fresh data afterwards (`App.Reload(d)`), which keeps
the tab, the cursors and the filter, and keeps the queue minus what the
run applied. `screenData` is read once per return from a run, not per
hop.

### The shared chrome

- Header: `brand · host · user` left, the path right, from
  `view.Path()` (`Home`, `Home › Tools › Dev`, `Home › Settings ›
  Look`, `Home › MCP servers › jira-work`).
- Footer: `view.Keys()` rendered once, in `App`. A `Key` is `{Key, What
  string; On bool}`; `On` draws it in the accent colour (the apply key
  with a queue). `?` and `q` are appended by `App`. The toast or flash is
  the line above; `Board.flash` stops replacing the footer.
- Dialog: `Board`'s `panel` moves to `App` as `Dialog{Title, Lines,
  Opts, OnPick}` and is used by the board's apply and uninstall
  questions, the MCP delete and the quit-with-a-queue question (now a
  dialog, not a flash). The menu's `sure` state goes.
- Cursor: `rowOn` with ` › `, as the menu draws it; the board's `>`
  pointer column goes.
- Paging: `App` turns PgUp/PgDn/Home/End/`g`/`G` into `MoveMsg{N int}`
  (±screenful, ∓∞) the views already know how to clamp (`move`,
  `normalize`).
- Keys screen: `App` draws `view.Keys()` plus the shared list; `?`
  toggles it, `q`/esc close it.

### Mouse

The program starts with `tea.WithMouseCellMotion()`. Each view's `Body`
records hit areas while drawing (`[]Hit{Y0, Y1, X0, X1 int; Do
func()}`) for tabs, rows, Go to entries, panels and dialog options; the
footer's keys get hits in `App`. A click runs the hit's `Do` (select the
row, open the tab); a second click on the same row within 400 ms is
enter. Wheel up/down is `MoveMsg{∓3}` to the view. Nothing depends on
the mouse: every hit has a key.

### First-run order

`runUser`'s first-run branch becomes: `askChoices` → build `choices`
from the picks (the part of `fromBoard` that doesn't ask) → `questions`
→ `review` → Install starts the run, Customize… opens the board with the
picks queued and `k.promptSet`/the answers kept, Quit returns with
nothing saved (the answers struct is discarded; `st.Save` isn't
called). `a` on the board after Customize… runs `questions` for the
added items only (it already asks only what's new since `saved`; pass the
first review's picks as `saved`), then `review`, then the run.

`review` prints the answers it has: the prompt, timezone, git, gateway
address, Docker ports' reach, dotfiles repo, git hosts with a key, and
the secrets' names from `creds.Secrets` (names only).

### Last run

A recording `run.Progress` wraps `Live`: it keeps each `Done` line and,
for a failure, the tail the runner prints. After the run, `runUser`
writes `<state>/<user>/.lastrun` (JSON: when, ok, the lines, the notes,
the hints, the failed ids) next to `.notes`. `screenData` reads it into
`HomeInfo.LastRun`; the menu adds the Go to entry, the Needs you row on
failure, and the `lastrun` screen. The toast comes from `ranSays`
without the "q quits" suffix.

### Keys to add

`.keys` is no longer removed at the start of a run; a run appends, and
`printKeys`' dedupe by public key applies on read. `connect.go` adds a
Keys to add section to the Connection screen from the same file. `d` on
a key removes its lines; `screenData` drops a git host's key when that
host's item reads `ok`. The terminal still prints the keys after a
non-interactive run (`--yes`, a profile), where no screen follows.

### Spinner count

`run.Runner` counts the steps it will start (items to apply, the apt
batch as one, each extra mise tool, `primeZshCompletions`) and calls
`Progress.Start` with the title prefixed `n of total · `; `Done` is
called with the plain title, as now, so result lines don't carry the
count. `Plain` prints both unchanged.

### Home layout

`homeView` uses `machineShort` at every width; Needs you is one box the
width of the screen, its rows' notes beside them (the three reserved
lines under the rows stay for the selected row's full note); Services &
containers and MCP servers stay side by side under it; the Connection
panel is a focusable panel (tab order: Needs you, Connection, Go to) and
Go to loses its Connection entry. `u` sends `MenuUpdates`; the Settings
`check` row goes; a "Check for updates" Needs you row shows while
`!checked`.

### Updates

The `updates` screen's `sure` state and the Update all button go: enter
sends `MenuUpgrade`, the footer says `enter update 3 · q back`, and the
line under the list says what the update does.

## Risks / Trade-offs

- [The `App` refactor touches every screen's tests] → `Menu` and
  `Board` keep `Update` and `View` (the latter now `Body`); the tests
  swap `View()` for `Body(w, h)` and the header/footer assertions move
  to `app_test.go`. No test changes meaning.
- [Mouse reporting over SSH, tmux and zellij] → tmux and zellij pass the
  mouse through when their own mouse mode is on; without it, the wheel
  scrolls the terminal and clicks do nothing. Every action keeps its key.
- [Double-click timing] → 400 ms, by the message's time; a slow link
  turns a double-click into two selects, which is harmless.
- [`.keys` kept across runs grows] → deduped by public key on read and
  removed on dismiss or when the host reads working.
- [Esc over SSH arrives late or not at all] → unchanged: `q` is the key
  the footer shows; esc is kept for leaving a field.
- [A first run's questions take time before the review] → the review
  then shows what they set, and Install needs nothing more; the choices
  screen says so in its sentence under the title.

## Migration Plan

No data migrates. `.lastrun` appears after the first run of the new
version; `.keys` keeps its format. Older state without `.lastrun` means
no Last run entry until a run happens. README updates ship with the
change.

## Open Questions

- Whether Last run should keep more than the last run (a list) can be
  decided later without changing the specs: `.lastrun` can become a
  directory.
