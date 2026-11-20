## Context

Today:
- **Checking.** `cmd/yardo/updates.go` starts the check in a goroutine
  (`steps.Outdated`: `mise outdated --json` and apt's upgradable list). It
  saves `status/<user>.updates.json` (`ByID`: item ID → `"node 22.12 →
  24.11"` lines) and sends `ui.UpdatesMsg` to the menu when it's done.
  Once a day, or on `u`.
- **Showing.** The menu's `updates` screen lists `newer` items. Enter
  returns `MenuUpgrade`, and `options.upgrade` runs `steps.Upgrade` on
  every outdated tool and package of every picked item, then clears the
  whole check.
- **On Tools.** Each `BoardItem` carries `Updates` (shown as an `↑` mark
  and in its details), but Tools has no list of them and no way to act.
- **The queue.** `BoardItem.Pending` is `add`, `uninstall` or `forget`;
  `a` returns `BoardApply` with `Adds`, `Uninstalls` and `Forgets`, and
  `runUser` runs them as one run.
- **Pins.** `Env.Lock` pins versions from a profile, and `steps.Upgrade`
  already skips pinned mise tools.

## Goals / Non-Goals

**Goals:**
- Choosing which tools to update, in the queue Tools already has.
- The check never blocks a screen.
- An update is a run like any other: progress line, toast, Last run.

**Non-Goals:**
- Updating yardo itself here (`yardo version up` stays a command).
- Choosing a version other than the latest.
- Updates for apt packages that no item installed (`sudo apt upgrade` is
  the system's own job; the security updates item covers it).

## Decisions

- **A tab on Tools, not a screen of its own.** Tools already has tabs,
  ticks, the queue, details on enter, search and `a`. An Updates screen
  apart from it would repeat all of that. The tab's ID is a constant like
  `InstalledTab` (`UpdatesTab`), placed right after Installed. Its rows are
  the items with `Updates`, plus one row per extra mise tool with a newer
  version (`mise:<tool>`, as the check already keys them). Alternative
  considered: a Go to entry opening the old screen with checkboxes. It was
  rejected because it would be a second queue, and adds plus updates
  couldn't share one run.
- **One row per item, not per package.** An item updates as a whole: its
  mise tools and its apt packages together (what `ItemOutdated` returns).
  Its row shows every version line, joined with " · " and cut to the
  width; the details (enter) show them all.
- **`update` is a fourth Pending kind.** Space on an Updates row toggles
  `Pending = "update"`, marked `[↑]`, and the queue line says "N to
  update". It's allowed only on a row that's picked and has `Updates`.
  Space on the same item in another tab still means uninstall: the update
  mark is the Updates tab's only, so space never means two things on one
  row. `BoardResult` gains `Updates []string` (item IDs and `mise:`
  IDs). `Reload` keeps a queued update only while the item still has
  `Updates`.
- **Opening from the home queues everything.** The home's row returns a
  new `MenuResult` kind, which the app turns into `Board.Open(UpdatesTab)`
  plus queueing every unpinned update, as `MenuTools` turns into the board
  today. Arriving by tab or by mouse queues nothing, because the user didn't
  ask for that.
- **The check goes to the board too.** `App` hands `UpdatesMsg` to the
  board as well as the menu. The board fills `Updates` from `ByID`, adds or
  removes the tab, and keeps the cursor and queue (`Reload`'s rules).
  While a check runs, the tab shows "Checking for updates…" as its only
  line. `u` on the tab returns the same request the home's `u` does.
- **Updating runs in the apply's run.** `runUser` adds the queued updates
  to the run that installs: uninstalls first, then installs, then one
  update step (`steps.Upgrade` with the queued items' outdated tools and
  packages, read again with `Outdated` just before, as `upgrade` does).
  That step counts in the progress line (`r.More`). `options.upgrade`
  takes the IDs to update, and the home's old "everything" path goes.
- **Afterwards, drop only what was updated.** The saved check keeps its
  other entries. The updated IDs are deleted from `ByID` and the file is
  saved again, so the tab still lists what was left out, with no new
  check.
- **Pinned rows.** The board learns which items are pinned (`HomeInfo`
  gets the profile lock's tool names). Their rows say "pinned by your
  profile", and space shows that in the toast without queueing.

## Risks / Trade-offs

- **apt updates one package, but its dependencies may move too.**
  `apt-get install --only-upgrade <pkgs>` can upgrade shared libraries
  too. That's acceptable, and the same as today.
- **A check finishing during a run.** The run already waits for a running
  check (`upd.wait`), so the check can't overwrite the run's results.
- **The tab appearing and disappearing.** A tab that comes and goes can
  move the user's tab position. `Reload`'s "keep the tab by ID" rule
  covers it, and when the user is on Updates and it goes away, they land
  on Installed.
- **More screens to keep tested.** The `updates` menu screen and its
  tests go. The board's tests cover the tab instead.
