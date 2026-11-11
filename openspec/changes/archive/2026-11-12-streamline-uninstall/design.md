## Decisions

- **Pending kinds**: `add`, `uninstall`, `forget` (was `remove`).
  `BoardResult.Removes` becomes `Forgets`. Apply order: uninstalls,
  forgets, adds.
- **Space on an installed tool yardo can't uninstall** opens a y/n:
  "yardo can't uninstall X: why. Forget it instead?" A `protected` item
  (zsh) only says why.
- **`d`** queues the row's uninstall if it isn't, then opens the
  question for every queued uninstall. No separate "now" path: one
  question, one run.
- **Filter**: `Board.search` stays the query; a new `typing` flag says
  keys go to it. Enter/↓ end typing; `/` starts a new query; esc clears
  the filter (then esc leaves as before).
- **Planner** returns a struct per id: `Say`, `Off` (grouped later by
  the UI), `Stays`, `Also` (dependents uninstalled), `Forgets`
  (dependents yardo can't uninstall), `Back` (a queued add that needs it).
- **Result panel**: the board gets `ShowResult(title, lines, retry)`;
  cmd passes it across the loop like the flash it replaces.
- **`part_of`** (catalog): the item must need its parent; the board
  doesn't list it; screenData folds its state, inputs and changes into
  the parent's row, and the parent's details get Log in / Log out.
  Fixing the parent fixes its picked parts too. Removing the parent
  removes its parts (they need it).
- **Dependents**: `leaving` returns what needs each id; the run
  uninstalls them first (one that needs another goes before it), then
  the id. A dependent that isn't removable is unpicked, and said.
- **Declined**: `Answers.Declined`, set when an uninstalled id is on a
  named profile's layer; `Pick` clears it; `Profile.Apply` skips declined
  ids with a warning.
- **Go-step undo**: a Go step that implements `Undoer` plans its own
  undo; `RegisterGo` marks it in `catalog.GoUndone`, which `Removable`
  reads. Skills: delete a skill folder whose mark matches its contents.
  gitattrib: the hook file if unchanged, its two config values if still
  yardo's. `git_config`: unset each value still yardo's. `cred`: stays,
  said ("holds your keys"). An item with no steps is removable.
- **Not done**: progress shown inside the screen while uninstalling
  (the run still prints its lines in the terminal, as installs do);
  dimmed "uninstalled" rows (the result panel says it).
