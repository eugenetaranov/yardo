## 1. The Updates tab

- [x] 1.1 `UpdatesTab` on the board: shown after Installed while a check runs or `Updates`/extra mise tools have newer versions; one row per item (its version lines joined, cut to the width; details on enter); "Checking for updates…" while a check runs. Verify: `board_test.go` view with three items with updates, with none, and while checking.
- [x] 1.2 `Pending = "update"` (`[↑]`), only on the Updates tab, on a picked row with updates; the queue line counts it ("N to update"); `BoardResult.Updates`; `Reload` keeps a queued update only while the item still has updates. Verify: `board_test.go` space/`a` test asserting `Updates` and no `Uninstalls`; `app_test.go` Reload case.
- [x] 1.3 Pinned rows: `HomeInfo` gets the profile lock's tools; their rows say "pinned by your profile" and space only says why. Verify: `board_test.go` pinned row.
- [x] 1.4 `UpdatesMsg` reaches the board through `App`; the tab fills, appears or goes (landing on Installed when it goes while open); `u` on the tab asks for a check as the home's does. Verify: `app_test.go` sends `UpdatesMsg` while on the tab and asserts the rows, then an empty one and asserts Installed.

## 2. The home

- [x] 2.1 The home's "Install N updates" row opens Tools on the Updates tab with every unpinned update queued; the `updates` menu screen, its keys and `MenuUpgrade` go. Verify: `menu_test.go`/`app_test.go`: enter on the row, then `a`, returns a `BoardApply` with all three in `Updates`; `q` goes home with the queue kept.
- [x] 2.2 With nothing newer after a check, the toast says everything is current. Verify: `menu_test.go` toast test.

## 3. The run

- [x] 3.1 `runUser`: queued updates join the apply's run, after uninstalls and installs, as one counted step (`r.More`, `r.Step`); `options.upgrade` takes the IDs (items' outdated tools and packages read again just before, `mise:` IDs as tools). Verify: `main_test.go` for the IDs → tools/packages split, pinned tools left out.
- [x] 3.2 After the run, drop the updated IDs from the saved check and save it; the rest stay listed. Verify: `main_test.go` saves a check of three, updates one, asserts two left.
- [x] 3.3 The toast and Last run name what was updated or what failed. Verify: `ranSays` case with updates.

## 4. Docs and checks

- [x] 4.1 README: the Tools section (the Updates tab, space, `a`, pinned) and the home's Install updates row (opens the tab, everything queued). Verify: README reads with no Updates screen.
- [x] 4.2 `task test` and `task lint` pass.
- [ ] 4.3 On a machine with a stale tool: `u`, the tab filling in while keys still work, update one of several, check the others stay listed and Last run names the one updated. Verify: the steps' output recorded in the PR.
