# yardo-cli Specification

## Purpose
yardo's command line: the commands and flags, what they print, and how a run is invoked, interactively or with --yes.

## Requirements

### Requirement: Single static binary
yardo SHALL be a static Go binary for linux amd64 and arm64, with its catalog, config files, templates, MCP catalog, mcp-sync and mise registry snapshot embedded. It SHALL NOT need python3, gum or bash scripts at run time.

#### Scenario: Minimal image
- **WHEN** yardo runs on Ubuntu cloud image without python3
- **THEN** the picker opens and a run with `cli` picked completes

### Requirement: Compatible usage
`sudo yardo` SHALL open the picker, `--yes` SHALL re-apply saved answers without questions, and `--dry-run` SHALL ask and then show the plan, as the bash version did. It SHALL fail with the same messages when there is no terminal and no `--yes`, or no saved answers with `--yes`.

#### Scenario: No answers with --yes
- **WHEN** `sudo yardo --yes` runs on a VM with no saved answers
- **THEN** it exits non-zero saying there are no saved choices yet

### Requirement: Picker parity
The Tools board SHALL keep the bash picker's search ranking (`yardo-picker-search`), including that `zsh` lists zsh, zsh config, Powerlevel10k, fuzzy tab completion and shell niceties first. Its tabs, keys and how search is entered and left are `yardo-tui-screens`'; the bash picker's number keys for tabs and Esc to quit are not kept.

#### Scenario: Search zsh
- **WHEN** the user types `zsh`
- **THEN** the first five results are the items zsh, zshcfg, p10k, fzftab and shellcfg, with zsh first

#### Scenario: Number keys
- **WHEN** the user presses `2` on Tools with no filter typed
- **THEN** the board filters to `2`, and the tab doesn't change
