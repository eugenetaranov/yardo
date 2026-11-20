# Tasks

## 1. One program

- [x] 1.1 Add `ui.App` (`internal/ui/app.go`): a `tea.Model` holding `*Menu`, `*Board`, the current view, a toast, and `AppResult` wrapping `MenuResult` or `BoardResult`; `Menu` and `Board` gain `Path()`, `Keys()` and `Body(w, h)` and stop drawing a header or footer; hops (`MenuTools`, board `q` from the menu, `BoardChange "mcp"`) switch the view instead of ending the program. Verify: `internal/ui/app_test.go` drives enter on Tools then `q` and asserts one program, the home body back, and no `BoardQuit` result; existing `menu_test.go` and `board_test.go` pass with `View()` swapped for `Body()`.
- [x] 1.2 `runUser` runs `App` once per return from a run: drop `onBoard`, `reopen`, `fromBoard`, `search`, `flash` and `d.forget()` on a hop; `App.Open(view, screen)` and `App.Toast` replace them; `App.Reload(d)` keeps the tab, cursors, filter and the queue minus what a run applied. Verify: `cmd/yardo/main_test.go` case where the board queues Java, hops home and back, and the queue is still there; `screenData` is called once per run, asserted by a counter in the test's fake.
- [x] 1.3 The home's Tools row note says "N changes queued" with a queue, and `q` on the home with a queue opens the quit dialog (1.6) once. Verify: `app_test.go` view and key tests.
- [x] 1.4 Typing on the home switches to the board with the filter live, each further key narrowing it in the same program. Verify: `app_test.go` types `n` then `e` and asserts the board body filtered to `ne` after each key; the `yardo-uninstall` "Live from the home" scenario.
- [x] 1.5 The shared header (brand · host · user · path) and footer (`view.Keys()` plus `?` and `q`, the apply key in the accent colour with a queue; the toast on the line above, never replacing the footer; `Board.flash` moves there). Verify: `app_test.go` asserts `Home › Tools › Dev` on the Dev tab and that the footer still lists keys while a flash shows.
- [x] 1.6 `Dialog` in `App`, used by the board's apply and uninstall questions, the MCP delete, and quit-with-a-queue; `y`/`n`/enter answer, `q`/esc cancel. Verify: `app_test.go` opens `d` on an MCP server and on a tool and asserts the same frame and footer; `board_test.go`'s confirm tests pass against the dialog.
- [x] 1.7 One cursor style: the board drops its `>` pointer and uses `rowOn` with ` › ` as the menu does. Verify: `board_test.go` view assertions updated; no `>` in any body.

## 2. Keys everyone shares

- [x] 2.1 The key map: `a` applies on MCP servers (Save goes, `s` does nothing), `n` adds there; `h` `l` on the home and `j` `k` everywhere arrows work; ←→ on the home move the cursor to and along Go to; Fix shows every row's details without a key and ←→ do nothing there. Verify: `menu_test.go` key tests for each scenario in `yardo-tui-screens` "One key map" and "Fix shows its details".
- [x] 2.2 Paging: `App` maps PgUp/PgDn/Home/End/`g`/`G` to `MoveMsg` and each list (home, Tools, Settings, Fix, MCP servers, Updates, Last run) clamps and scrolls to keep the cursor in view. Verify: `app_test.go` presses `G` on the Installed tab and asserts the last row is selected and visible.
- [x] 2.3 The keys screen: `?` draws the current view's keys then the shared ones; `q`, esc or `?` close it to the same row; the footer lists `?`. Verify: `app_test.go` presses `?` on Tools and asserts the list and the return.
- [x] 2.4 `d` in Tools' footer when the row is installed. Verify: `board_test.go` footer test; the `yardo-uninstall` "d in the footer" scenario.
- [x] 2.5 Mouse: `tea.WithMouseCellMotion()`; hit areas recorded by each body for tabs, rows, Go to entries, panels, dialog options and footer keys; click selects, a second click within 400 ms acts, wheel sends `MoveMsg{∓3}` to the view under the pointer. Verify: `app_test.go` sends `tea.MouseMsg` clicks on a tab, a row twice, and a footer key, and wheel events over Needs you, asserting each effect.
- [x] 2.6 README: the keys paragraph (one list for every screen), `?`, paging, the mouse, `n` and `a` on MCP servers. Verify: README reads with the new keys and no mention of `s`.

## 3. The home

- [x] 3.1 Machine folds to two lines at every width; Needs you is the full width with notes beside the rows; Services & containers and MCP servers side by side under it. Verify: `menu_test.go` home view at 120 and 80 columns.
- [x] 3.2 Connection: the panel is focusable (tab order Needs you, Connection, Go to), enter on it opens the screen, Go to loses its entry, the panel's title is Connection. Verify: `menu_test.go` tab-then-enter test and the Go to line.
- [x] 3.3 Check for updates: `u` on the home sends `MenuUpdates` with the toast; a Needs you row "Check for updates" while never checked; the Settings row goes. Verify: `menu_test.go` key and view tests; `runUser` handles `MenuUpdates` from the home (reopen to home, not Settings).
- [x] 3.4 Updates in one step: enter on the list sends `MenuUpgrade`; the Update all button and `sure` go; the footer reads `enter update N · q back`. Verify: `menu_test.go` presses enter on "Install 3 updates" then enter and asserts `MenuUpgrade`.
- [x] 3.5 README: the home sample redrawn from the new layout, the Connection panel, `u`, the Updates step. Verify: the sample matches `menu_test.go`'s 120-column view.

## 4. The first run

- [x] 4.1 `runUser`'s first-run branch: `askChoices`, then the choices built without asking (split `fromBoard` into building and asking), then `questions`, then `review`; Install starts the run with no prompt; Quit discards the answers and saves nothing. Verify: `cmd/yardo/main_test.go` with fake prompts asserts the order of prompts and that no prompt follows Install, and that Quit leaves no state file.
- [x] 4.2 `review` prints every answer taken: prompt, timezone, git, gateway address, Docker ports' reach, dotfiles repo, git hosts with a key, secret names only. Verify: `ask_test.go` review output test with a git name and an Anthropic key, asserting the name shows and the key doesn't.
- [x] 4.3 Customize…: the board opens with the picks queued and the answers kept; `a` runs `questions` with the first review's picks as `saved`, so only the added items' questions are asked, then `review`, then the run. Verify: `main_test.go` case adding Java after Customize… asserts no repeated prompt.
- [x] 4.4 shift+tab on the first question goes back to Choices with the ticks kept; `q` on Choices or a question exits saving nothing. Verify: `main_test.go` back-navigation case.
- [x] 4.5 README: the first-run paragraph (one Choices screen, questions, review, Install), and the Choices screen's sentence under its title says the questions come next and Install then needs nothing more. Verify: README and `choices_test.go` view.

## 5. The last run

- [x] 5.1 A recording `run.Progress` wrapping `Live` keeps each result line and failure tail; after a run `runUser` writes `<state>/<user>/.lastrun` (when, ok, lines, notes, hints, failed ids). Verify: `run_test.go` for the recorder; `main_test.go` asserts the file after a run with one failure.
- [x] 5.2 `screenData` reads `.lastrun` into `HomeInfo.LastRun`; the menu adds the Go to entry with when and how it went, the Needs you row "See why the last run failed" on failure, and the `lastrun` screen (result lines, failing output, notes, hints, log path). Verify: `menu_test.go` view tests for both rows and the screen; `yardo-last-run` scenarios.
- [x] 5.3 `ranSays` drops the "q quits and shows the run's output" suffix and names Last run on failure. Verify: `main_test.go` `ranSays` cases.
- [x] 5.4 Keys to add: `.keys` kept across runs (not removed at a run's start), deduped by key on read; Connection shows each under Keys to add with where to add it and the key alone on a line; `d` removes it; `screenData` drops a git host's key when its item reads working; the terminal still prints keys after a non-interactive run. Verify: `connect_test.go` for the section and `d`; `main_test.go` for the kept file and the drop on `ok`; the `--yes` path still prints.
- [x] 5.5 Spinner count: the runner counts the steps it will start and prefixes `Start` with `n of total · `; `Done` keeps the plain title. Verify: `run_test.go` asserts the labels for a plan with an apt batch and two extra mise tools.
- [x] 5.6 README: the toast, Last run, Keys to add on Connection, the progress line. Verify: README reads with them.

## 6. Check

- [x] 6.1 `task test` and `task lint` pass.
- [ ] 6.2 On a fresh VM (pmox): a first run end to end, asserting the order of prompts, that Install starts at once, the GitHub key on Connection afterwards, Last run after a forced failure, a hop home and back with a queue kept, `?`, `G`, a mouse click on a tab over SSH in tmux with mouse on, and `a` on MCP servers. Verify: the steps' output recorded in the PR.
