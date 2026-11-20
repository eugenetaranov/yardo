## Why

yardo already looks for newer versions (apt and mise) in the background,
once a day, and the home shows **Install N updates**. But that's all or
nothing: the Updates screen lists them and enter updates every one. You
can't leave Node on 22 while taking the rest, or update just the one tool
you need today. Updates also have no place of their own: they show up only
on the home, or as an `↑` mark buried in Tools' other tabs.

## What Changes

- **An Updates tab on Tools.** It sits beside Installed and lists every
  tool with a newer version, one row each: its name, the version it has,
  the version that's out (`node 22.12 → 24.11`), and whether it's a mise
  tool or an apt package. The tab is there once a check has found something
  or while a check is running. With nothing to update, it's gone (and a
  toast says everything is current).
- **Pick, then apply.** Space queues a tool to update (`[↑]`), using the
  same queue as adding and uninstalling, and `a` applies it all in one run.
  Enter on the home's **Install N updates** row opens the tab with every
  update queued, so two keys still update everything (enter, then `a`).
  Arriving any other way, nothing is queued.
- **The check stays in the background.** While it runs, the tab says
  "Checking for updates…" and fills in when it's done. `u` on the tab (as
  on the home) checks again. The screen never waits for it, and nothing
  else stops while it runs.
- **Only what you picked is updated.** The run updates just the queued
  tools (`mise up <tools>`, and apt for just those packages). It uses the
  run's progress line, its toast and Last run. Afterwards only the updated
  tools drop off the list; the rest stay until the next check.
- **Pinned tools say so.** A tool whose version a team profile pins is
  listed with "pinned by your profile" and can't be queued.
- **BREAKING (screen):** the home's separate Updates screen goes. Its row
  opens the Tools tab instead.

Not in this change: updating yardo itself from the tab (`yardo version
up` stays a command), choosing a version other than the latest, and
changing the weekly tool updates timer.

## Capabilities

### New Capabilities
- `yardo-updates`: how yardo finds newer versions of what it installed
  (in the background, kept between runs), shows them on the Updates tab,
  and updates the ones you pick.

### Modified Capabilities
- `yardo-tui-screens`: "Updates in one step" (the Updates screen) is
  replaced by "Install updates opens the Updates tab", with everything
  queued.

## Impact

- `internal/ui/board.go`: the Updates tab, the `update` queue kind and its
  mark, `u` there. `internal/ui/menu.go`: the `updates` screen goes; its
  row opens the tab. `internal/ui/app.go`: update results go to the board
  as well as the menu.
- `cmd/yardo/updates.go`, `status.go` (`upgrade`): update the IDs given,
  not everything; keep the check's other entries afterwards.
  `cmd/yardo/main.go`: an apply with updates queued runs them in the same
  run as adds and uninstalls.
- `internal/steps/decl.go` (`Upgrade`): unchanged in what it takes (tool
  and package lists); it gets the picked ones.
- README: the Tools section and the home's Install updates row.
