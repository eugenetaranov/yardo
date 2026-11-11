## 1. Removal model

- [x] 1.1 Go-step undo (`Undoer`, `catalog.GoUndone`): skills, gitattrib; `git_config` and `cred` undo; an item with no steps is removable; `protected`
- [x] 1.2 `part_of` replaces `remove_with` (validation; netbird, tailscale, mcpjungle); login remove scripts (log out)
- [x] 1.3 Catalog: keep_say only for ECC, GitHub, commit signing, dotfiles, agent settings, Network access; zsh `protected`; shorter keep_say strings
- [x] 1.4 Dependents uninstalled first (ordered), unremovable ones forgotten; planner struct
- [x] 1.5 `Answers.Declined`; `Pick` clears it; `Profile.Apply` skips with a warning
- [x] 1.6 Tests: skills/gitattrib/git_config undo, dependents order, declined

## 2. Board

- [x] 2.1 Space uninstalls an installed tool; can't: forget y/n; protected: says why; `d` queues and asks; `forget` replaces `remove` (panel only)
- [x] 2.2 Typing filters; enter/↓ into the rows; esc clears
- [x] 2.3 Marks and words: `[−]` will uninstall, will forget, no health mark when pending, queue in words, footer, strings
- [x] 2.4 Question: title by action, short names, Say/Stays/goes with/forgets/back, "List everything", indents kept, y/n
- [x] 2.5 Result panel with Retry; parts folded into the parent row (health, values, changes, Log in/out); fix includes parts
- [x] 2.6 Tests

## 3. CLI, docs

- [x] 3.1 `yardo remove`: names (tool before parts), dependents, same plan; tests
- [x] 3.2 Home note, README, help
