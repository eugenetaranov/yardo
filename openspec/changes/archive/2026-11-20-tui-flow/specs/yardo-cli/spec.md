# Spec Delta

## MODIFIED Requirements

### Requirement: Picker parity
The Tools board SHALL keep the bash picker's search ranking (`yardo-picker-search`), including that `zsh` lists zsh, zsh config, Powerlevel10k, fuzzy tab completion and shell niceties first. Its tabs, keys and how search is entered and left are `yardo-tui-screens`'; the bash picker's number keys for tabs and Esc to quit are not kept.

#### Scenario: Search zsh
- **WHEN** the user types `zsh`
- **THEN** the first five results are the items zsh, zshcfg, p10k, fzftab and shellcfg, with zsh first

#### Scenario: Number keys
- **WHEN** the user presses `2` on Tools with no filter typed
- **THEN** the board filters to `2`, and the tab doesn't change
