## ADDED Requirements

### Requirement: The user's .zshrc is theirs
yardo's zsh setup SHALL live in `~/.config/yardo/zshrc`, and what this spec's other requirements call the rendered `.zshrc` SHALL be that file. In `~/.zshrc`, yardo SHALL keep one marked block at the top that loads it, and SHALL NOT add, change or remove any other line. A re-run SHALL leave every line outside the block as it was. Lines after the block SHALL take effect after yardo's setup, so the user's settings win. `~/.zshrc.local`, when it exists, SHALL still be loaded.

#### Scenario: An alias survives
- **WHEN** the user adds `alias k=kubectl` to `~/.zshrc` and `sudo yardo` runs again
- **THEN** `~/.zshrc` still has the alias, and a new shell has it

#### Scenario: The user's setting wins
- **WHEN** `~/.zshrc` has `export EDITOR=hx` below the block
- **THEN** a new shell's `EDITOR` is `hx`

#### Scenario: No .zshrc
- **WHEN** the zsh config is installed for a user with no `~/.zshrc`
- **THEN** `~/.zshrc` is created holding only the block

#### Scenario: The block is gone
- **WHEN** the user deletes the block and yardo runs
- **THEN** the block is back at the top, and the other lines are unchanged

### Requirement: An existing .zshrc is kept
Installing the zsh config for a user with a `.zshrc` of their own SHALL keep the file, adding only the block on top. When that file loads oh-my-zsh itself, the run SHALL note that it now loads twice and what to take out.

#### Scenario: Own .zshrc
- **WHEN** the user has their own 40-line `.zshrc` and the zsh config is installed
- **THEN** the file is those 40 lines under the block, and there is no `.zshrc.orig`

### Requirement: Machines set up before are moved over
On the first run after this change, a `.zshrc` that yardo wrote SHALL be saved once as `~/.zshrc.before-yardo-split` and replaced by the block, and the run SHALL note where the old file is. The lines yardo added to `.zshrc` for its other shell files SHALL be removed from it. Later runs SHALL NOT save or note again.

#### Scenario: A yardo-written .zshrc
- **WHEN** a machine whose `.zshrc` came from yardo's template runs the new yardo
- **THEN** `.zshrc` is the block, the old file is `~/.zshrc.before-yardo-split`, a new shell works as before, and the run's notes name the old file

#### Scenario: Second run
- **WHEN** yardo runs again
- **THEN** nothing is saved or noted, and `.zshrc` is unchanged

### Requirement: Uninstalling leaves the user's lines
Uninstalling the zsh config SHALL remove the block and `~/.config/yardo/zshrc` and SHALL leave every other line of `~/.zshrc`.

#### Scenario: Uninstall
- **WHEN** the zsh config is uninstalled from a machine whose `.zshrc` has the block and an alias
- **THEN** `.zshrc` has the alias and no block

## MODIFIED Requirements

### Requirement: Shell drop-in activates installed tools
A `shellcfg` item, on the Shell & CLI tab, SHALL install `~/.config/yardo/shell.sh`. The drop-in SHALL, for both zsh and bash and only for tools that are present on PATH:
- initialise zoxide
- hook direnv
- load fzf key bindings and completion from `/usr/share/doc/fzf/examples/`
- define the aliases `ls`/`ll`/`la` → eza and `cat` → `bat --paging=never --style=plain`
- set `EDITOR`/`VISUAL` to `nvim` if it is present, else `vim`
- when `terraform` or `tofu` is present, export `TF_PLUGIN_CACHE_DIR=$HOME/.terraform.d/plugin-cache` and create that directory
- when `kubecolor` is present, alias `kubectl` to `kubecolor`; in zsh, also `compdef kubecolor=kubectl`

For bash, a single guarded `source` line SHALL be added to the end of `.bashrc`, at most once. For zsh, yardo's own zsh file SHALL source the drop-in when the zsh config is installed; without the zsh config, the line SHALL be in yardo's block in `.zshrc`. No source line SHALL be added to `.zshrc` outside that block.

#### Scenario: Tools activated
- **WHEN** `cli` and `shellcfg` are picked and the user opens a new zsh
- **THEN** `z`, Ctrl-R fzf history and direnv `.envrc` loading all work

#### Scenario: Tool absent
- **WHEN** `shellcfg` is picked without `cli`
- **THEN** a new shell starts with no errors and none of the missing tools' hooks

#### Scenario: Idempotent
- **WHEN** yardo runs `shellcfg` twice
- **THEN** `.bashrc` contains the source line exactly once, and `.zshrc` has one yardo block

#### Scenario: zsh config rewrites zshrc
- **WHEN** `zshcfg` and `shellcfg` are both in the plan
- **THEN** yardo's zsh file sources the drop-in, and `.zshrc` has only the block from yardo

#### Scenario: Terraform providers cached per VM
- **WHEN** `terraform` and `shellcfg` are installed and the user runs `terraform init` in two directories using the same provider
- **THEN** the provider is downloaded once, into `~/.terraform.d/plugin-cache`
