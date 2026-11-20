# yardo

**Set up a Linux dev machine for you and your AI agents.** Run `sudo yardo`
on a fresh machine (a VM, a cloud server, a box under your desk), tick what
you want in a tabbed picker, and yardo installs and configures it: dev tools
and languages, cloud, IaC and Kubernetes CLIs, Docker, coding agents (Claude
Code, Codex, Gemini, OpenCode, Goose, …) wired to an MCP gateway, system
hardening, SSH and git keys, your dotfiles. It remembers what you picked and
re-applies it idempotently. Teams share TOML profiles and roll them across
many machines; `yardo status` shows what's installed and what isn't right.

yardo is a single static Go binary for Linux (amd64, arm64) with everything
it needs embedded. It needs root (it configures the OS) and Ubuntu (22.04 or
later) or Debian (12 or later), or a derivative of one; on anything else
`install.sh` and yardo stop before changing anything.

## Install

On the machine:

```sh
curl -fsSL https://github.com/eugenetaranov/yardo/releases/latest/download/install.sh | sudo sh
sudo yardo
```

`install.sh` downloads the release for the machine's architecture and checks
it against the release's `checksums.txt`; `--version v0.1.0` (or
`YARDO_VERSION`) picks a release. Homebrew on Linux: `brew install
eugenetaranov/tap/yardo`. From source: `go install
github.com/eugenetaranov/yardo/cmd/yardo@latest`.

`yardo --help` lists every command and flag (`yardo status --help` and so
on for one command's). `--dry-run` shows what a run would do, changing
nothing; every command and its output goes to `/var/log/yardo.log`.

To upgrade later, run `yardo version up` (`--check` only says whether a newer
release is out). It downloads the release, checks it against
`checksums.txt` and that it runs, and replaces the binary (as root); a
Homebrew install upgrades through `brew`.

In cloud-init, to set a machine up unattended at first boot:

```yaml
runcmd:
  - curl -fsSL https://github.com/eugenetaranov/yardo/releases/latest/download/install.sh | sh
  - yardo --profile https://example.com/team.toml --yes --user ubuntu
```

[pmox](https://github.com/eugenetaranov/pmox) (Proxmox VMs for developers)
installs yardo on every VM it launches and drives it across VMs (`pmox yardo
apply|verify|gitkey`).

## Using it

`sudo yardo` sets up the user who ran it (`--user ada` for another; a missing
account is created). Once something is set up, it opens on the home:

```
 yardo  ·  vm  ·  ada                                                                          Home
 ▲ 3 things need you  ·  everything else is working                     Ubuntu 24.04  ·  100.64.0.7

┌─ Needs you ───────────────────────────────────────────────────────────────────────────────── 3 ┐
│  ○ Fill in jira                                                                                │
│  ▲ Check opensearch  didn't register: check its settings                                       │
│  ↑ Install 1 update  node 22.12 → 24.11, gh 2.80 → 2.83                                        │
│    JIRA_API_TOKEN                                                                              │
│                                                                                                │
│                                                                                                │
│ ✓ 4 of 4 tools working                                                                         │
│   1 update · checked 09:12                                                                     │
└────────────────────────────────────────────────────────────────────────────────────────────────┘
┌─ Machine ──────────────────────────────────────────────────────────────────────────────────────┐
│ CPU   3%   ▁▂▁▁▃▁  now  ·  0.12 0.08 0.05 on 4 cores                                           │
│ Mem   ▰▰▱▱▱▱▱▱▱▱ 23%   1.8 GB of 7.8 GB  ·  Disk  ▰▰▰▱▱▱▱▱▱▱ 31%   of 78 GB  ·  Up    3d 4h     │
└────────────────────────────────────────────────────────────────────────────────────────────────┘

┌─ Services & containers ──────────────────────┐  ┌─ MCP servers ────────────────────────────────┐
│ ● ssh          22/tcp         anyone         │  │ Gateway ● up  this machine only              │
│ ● fail2ban     up                            │  │                                              │
│ ● tailscale    41641/udp      anyone         │  │ ● github            ● jira-work              │
│ ● docker       8080/tcp       VPN            │  │ ○ jira             needs JIRA_API_TOKEN      │
│ ─ containers ─────────────                   │  │ ▲ opensearch       didn't register: check i… │
│ ● postgres:16    myapp-db-1     :5432        │  │ + 1 more in ~/.mcp                           │
└──────────────────────────────────────────────┘  └──────────────────────────────────────────────┘

┌─ Connection ──────────────────────────────────────────────────────────────────── from a laptop ┐
│ SSH      ssh vm  ·  ada@vm.tail1234.ts.net                                                     │
│ MCP      http://vm.tail1234.ts.net:8081/mcp  ·  token: sudo cat ~/.mcp/clients/laptop.token    │
│ Enter: every step, ready to paste, and the networks: start, stop, add, remove                  │
└────────────────────────────────────────────────────────────────────────────────────────────────┘

┌─ Go to ────────────────────────────────────────────────────────────────────────────────────────┐
│  Tools 4     MCP servers 4     Connection     Settings     Last run                            │
│ or just start typing to search the tools                                                       │
└────────────────────────────────────────────────────────────────────────────────────────────────┘

 ↑↓←→ choose   tab next   enter open   u updates   type search   ? keys   q quit
```

**Needs you**, the width of the screen, has a row for each thing that
needs doing, named by what it does: **Start** something stopped,
**Restore** a file edited by hand (putting back yardo's version) or
**Update** what an older yardo set up (only what yardo set up: an item
picked but never installed is on Tools, not a problem; three or more are
one **Fix 3 things** row, a checklist with each one's details under it,
all ticked: space unticks, enter fixes the ticked in one run), **Fill
in** a value (opens its file in your editor), **Install 3 updates**
(enter opens Tools on the Updates tab with all three queued: **a**
updates them, space on one leaves it out, q goes home with the queue
kept), **Check for updates** (while it was never
done; **u** does it from anywhere on the home), and **See why the last
run failed**. Under it, the **Machine** in two lines, **Services &
containers** (what's up or failed, the ports it listens on beyond this
machine and who can reach them; the containers by image) and the **MCP
servers**; then **Connection**, a panel tab reaches: enter opens the
Connection screen (below), the same as Go to's Connection. **Go to** has
what never moves: **Tools** (the full
screen, below; with changes queued there, it says how many), **MCP
servers** (the gateway's list: **space** marks one, or All servers, off
or on, **a** applies what's marked, **n** adds one, or another instance
of one, **e** edits its settings, **d** deletes one, asking first),
**Connection**, **Settings** (**Network access**: the MCP gateway and containers'
published ports, each a row of choices with a sentence saying what it
means; **Accept new hosts' SSH keys**; **Session on login**; the
**Look**, theme and accent
colour; your **Git name and email**, each git host's **SSH key**, the
**Shell prompt** and the **Dotfiles repo**, asked again, then applied)
and **Last run**. Typing on the home searches the tools, live on the
Tools screen.

yardo looks for newer versions (apt and mise) in the background while
the home is up, once a day; a run waits for it, as both use apt.

**One program, one key map.** The home, Tools, Settings, Fix, Updates,
Connection, Last run and the MCP servers are one screen that changes,
without a flicker: Tools keeps its queue, its tab, each tab's row and
its filter while you go home and back (q on the home with changes queued
asks once). Only a question that needs typing (a name, a key) and a run
leave the screen. Every screen has the same top line (who and where,
then the path: `Home › Tools › Dev`) and the same bottom line, listing
every key that screen takes (zellij's status bar, Midnight Commander's
key bar); the line above it says what just happened. The keys mean the
same everywhere:

| Key | Does |
|---|---|
| ↑↓ (`j` `k`) | move |
| ←→ (`h` `l`) | switch tabs on Tools; pick a choice on a setting; on the home, to and along Go to |
| tab | the next panel (home) or tab (Tools) |
| enter | open the row, or do what it says |
| space | tick: queue a tool, tick a fix, turn a server off or on |
| `a` | apply what's queued or changed (Tools, Network access, Look, MCP servers) |
| `n` | add (an MCP server) |
| `d` | delete or uninstall, asked first |
| `e` | edit (an MCP server's settings) |
| `/` | search |
| `u` | check for updates (the home) |
| PgUp PgDn, Home End, `g` `G` | a screenful up or down, the first or last row |
| `?` | every key of the screen, then the ones every screen shares (as in tmux) |
| `q` | back; on the home, quit |
| ctrl+c | quit, from anywhere |

Every question (apply, uninstall, delete, quit with a queue) is the same
framed box over the screen: enter, `y` or `n` answers it, `q` leaves it.
The mouse works too, over SSH and in tmux or zellij with their mouse on:
a click selects a row, a double-click opens it, a click on a tab, a Go
to entry or a key in the bottom line does what it says, and the wheel
scrolls. Nothing needs the mouse. Esc goes back too, but over SSH, in
tmux or zellij it can arrive late or not at all, so it's kept for leaving
a field you're typing in, where q types a q.

**After a run** yardo comes back to the home, saying how it went on the
line above the keys: `✓ Done: Docker Engine`, or `✗ Docker Engine failed ·
Last run shows what happened`. **Last run** (Go to, and a Needs you row
while it failed) keeps what the run printed, from one session to the
next: each step's result, the last lines of a failing step's output, the
notes and hints, and the log's path. While a run is on, its line counts
the steps: `12 of 43 · Docker Engine (1m20s)`.

**The first run** asks everything before it installs anything: the
choices (one screen: the zsh prompt, Powerlevel10k or Starship, then):

| Choice | What it installs | Starts |
|---|---|---|
| Languages | any of Python, Node.js, Go, each on its own | none |
| Docker | Docker and its tools, published-port limits, the MCP gateway | on |
| Kubernetes tools | kubectl and its aliases, kubectx, helm, k9s, the extras | off |
| Terraform tools | Terraform, OpenTofu, Packer, Ansible, tack, Vault | off |
| Cloud providers | any of AWS, DigitalOcean, Hetzner, cloudflared, Google Cloud, Azure | none |
| Coding agents | any of Claude Code, Codex, Gemini, OpenCode, Aider, Goose, omp, API keys | Claude Code |
| Git hosts | any of GitHub, GitLab, Bitbucket (each logs in) | GitHub |
| VPN or mesh | one of Tailscale, NetBird, the OpenVPN or WireGuard client | none |

Everything else in the base comes without asking: zsh, Neovim, tmux,
zellij, mosh, shell helpers, dev CLIs and linters, DB clients, admin and
debug tools, etckeeper, OpenSpec, agent setup and
skills, git name and email; and what every machine gets (security
updates, SSH keys only, fail2ban, zram, …). Then every question the
choices need, a page each: git name and email, timezone, who can reach
the MCP gateway, Docker's published ports, wiring the agents, the
dotfiles repo, API keys, a key per git host (shift+tab on the first goes
back to the choices, ticks kept; esc on any quits, nothing saved). Then
the review: the whole list, item by item, and every answer (keys by name,
never shown), and **Install** (it starts at once: nothing more to ask),
**Customize…** (the full screen below, with it all queued; **a** there
asks only what the added items need, then the review again) or **Quit**
(nothing is saved; the next `sudo yardo` starts over). The rest (Java, Rust, Podman, NFS/SMB clients, Ollama, jiractl,
restic, your dotfiles, commit signing, more agent add-ons) is on Tools.
A machine set up before the base had all it has installs what's new in
it on its next run, once, before the home opens.

**Tools** opens the full screen: tabs by area
(**Installed** first, once anything is: every tool installed, from all
the tabs, to fix or uninstall one; **Updates** next, while a check runs
or has found newer versions; then **Dev**, **DevOps**, **AI**, **Shell & CLI**, **System**, **Git &
SSH**) above a checklist, where a ticked box means installed and a mark
beside it how it is, said in a word beside it: `✓` working, `✗` stopped
or not installed, `~` half set up, edited by hand, or set up by an older
yardo (this one ships newer files; Fix applies them), `○` needs a value,
`↑` a newer version is out.

- **space** ticks a tool to install (`[+]`), or unticks an installed one
  to uninstall it (`[−]`); space again takes it off the queue. The queue
  shows on a line above the keys, in words.
- **Updates** lists each tool with a newer version, one row each: the
  version it has and the one that's out (`node 22.12 → 24.11`), and
  whether it comes from mise or apt. **space** queues one to update
  (`[↑]`), in the same queue as adding and uninstalling, and **a**
  applies it all in one run: only the queued tools are updated, each to
  its latest version, and the others stay listed until the next check.
  A tool a team profile pins says "pinned by your profile" and isn't
  queued. yardo looks for newer versions once a day, in the background;
  **u** there (or on the home) looks again, and every key keeps working
  meanwhile. Updating yardo itself is `sudo yardo version up`.
- **d** asks to uninstall the row now (with anything else queued to
  uninstall).
- **enter** acts on the row: fix what's broken (now), fill in a missing
  value (in your editor), change an answer (git name and email, the
  prompt, the dotfiles repo, the MCP servers), log in or out (a VPN), or
  show an item's details, with Reinstall (after `yardo version up`, to
  take a newer yardo's files; tools installed from a git repo, such as
  rds-mysql, are fetched again too), Uninstall, and Forget it (it stays
  installed, and yardo stops checking it). On a row not installed, enter
  queues adding it, as space does.
- **d** is in the bottom line on an installed tool.
- **a** applies the queue: it asks first (see [Removing
  tools](#removing-tools)), uninstalls, then asks what the new items
  need (git name, prompt, MCP servers, keys) and installs.
- **Typing**, here or on the home, filters every tab as you type;
  **enter** goes into the list with the filter kept, **esc** clears it
  while you type, **q** after. For a tool the catalog
  doesn't have, ctrl-a adds it as an extra mise tool.
- **AI** has Agent setup (agent notes, safety settings, skills), under
  its own heading. A VPN's login is part of its row: enter on Tailscale
  or NetBird logs in or out.

**Team profiles.** On a machine set up from a named team profile, the
home says how many tools came from it, the Tools screen notes each, and
unticking one warns that applying the profile again adds it back.

**Theme.** Settings → Look → Theme, or `--theme` for one run:
**Classic** (the default: calm, in your accent colour), **Bridge** (a
starship console: navy, cyan and amber, ● ✕ ◧ marks, titles in
capitals), **Covert** (a night-ops dossier: phosphor green with signal
orange, ■ ! ▣ marks) or **Tactical** (a shooter HUD: graphite, white
and orange). A theme is yardo's own screens only; it's kept at once,
without a run. Your terminal's background and font stay yours, so it
works over SSH anywhere.

**Accent colour.** Settings → Look → Accent colour: Coral (the default),
Lagoon (aqua blue), Pistachio, Lavender, Amber or Rose. yardo's screen
takes it in the Classic theme, and so does your shell, so each machine's terminals can be
told apart at a glance: the prompt (Powerlevel10k's OS icon, directory
and prompt character, or Starship's), tmux's status bar and active pane
border, and `$YARDO_ACCENT` for your own scripts. yardo only rewrites a
prompt config it wrote (its first line says so); after `p10k configure`,
or once you take that line off your `starship.toml`, the file is yours.

The first run comes with the usual stack queued: zsh, Neovim, tmux,
zellij, the shell helpers (the git and kubectl pickers, …), the linters, the dev CLIs, the database clients, the admin and
debug tools, Docker and its tools, Claude Code with OpenSpec, the
MCPJungle gateway, git (name and email, defaults, lazygit, GitHub) and
the two agent items. Space takes off what you don't want; a installs.
Later runs show what's installed ticked and checked, and ask only about
what's new. `sudo yardo --ui board` opens on this screen instead of the
home (q goes to the home, the queue kept).

The run follows the same order: everything is installed first (the extra
mise tools at the end of that), then configured, with your dotfiles last.

yardo is opinionated: what every machine should have isn't offered, it's
always applied (see "Always" below), and related tools come as one item.

Just start typing to fuzzy-search every tab of the screen at once: `ssh`
lists what mentions SSH wherever it lives, each with its category. For a
tool the catalog doesn't have, search for it and press ctrl-a on Install:
it's added, ticked, as an extra mise tool (`mise use -g <tool>`; a
version or a `backend:tool` such as `pipx:pytest` or
`github:owner/repo` works too), under an Extra tools tab. A profile lists
them under `mise_extra`. yardo keeps them on re-runs until you untick
one. Extra tools aren't pinned: mise fetches whatever the name resolves
to. Esc clears the search; Esc on an empty search quits (on Configure:
goes back).

**Always**, on every machine, with nothing to pick. Installing: essentials
(git, curl, jq, yq, gh, build tools). Configuring: automatic security updates
(no auto-reboot) with needrestart restarting services quietly; weekly
updates of your tools (a systemd timer running `mise up` and `krew upgrade`
as you); SSH keys only, no root login
(skipped, with a note, while you have no authorized key; a config sshd
rejects is rolled back); fail2ban for SSH, on a machine with a public IPv4
on an interface (Tailscale addresses are never banned; a cloud VM behind
1:1 NAT only sees its private address, so it gets none); dev limits
(inotify, open files), zram swap, a journal size cap, timezone (asked once)
+ locale. Keeping the clock in sync is left to the machine's launcher.

### Removing tools

On Tools, untick a tool (**space**) and press **a**, or press **d** on it;
or from a shell:

```sh
sudo yardo remove aws netbird --dry-run   # what would go, and what stays
sudo yardo remove aws netbird             # asks, then uninstalls
```

An item goes by its name or id (`yardo --list` lists them); `--yes`
doesn't ask. Before it changes anything, yardo asks, saying for each tool:

- **what its removal does**, in a line: "Disconnects from NetBird,
  uninstalls it and deletes its keys". **List everything that goes**
  shows every package, mise tool, file and line it takes off: its
  packages, apt repo and mise tools, and what yardo set up for it (its
  files, the lines it added to `.bashrc`, yardo's block in `.zshrc` (the
  rest of `.zshrc` stays), its config block,
  links, clones, services, sysctl and git settings, agent skills).
- **what stays, and why**: what another installed tool uses ("fzf
  (Shell helpers uses it)"), a file or git setting you changed ("edited
  by hand"), a clone with local changes, the file with your agent keys.
  Your own data never goes: `~/.aws`, `~/.kube`, `~/.ssh`, shell history,
  Docker's volumes.
- **what goes with it**: the installed tools that need it are
  uninstalled too, first (uninstalling Node takes Gemini CLI with it);
  one yardo can't uninstall is forgotten (it stays installed).

A VPN's login is part of its row: uninstalling NetBird logs out, uninstalls
it and deletes its keys; the peer stays in your NetBird dashboard (or the
machine in your Tailscale admin console) until you delete it there. To
log out and keep NetBird, enter on its row → Log out.

After, a box says how it went, a line per tool, until you close it. A
tool that fails stays installed (it shows as half set up); the box offers
**Retry**, which picks up where it stopped, and the others still go.
`yardo remove` exits 1 then, and 2, removing nothing, when a tool isn't
installed or can't be removed.

A tool you uninstall that your team profile lists stays off: applying the
profile again skips it and says so. Adding it on Tools clears that.

A few tools yardo won't uninstall, and says why: ECC, your GitHub login,
commit signing, your dotfiles and the agents' settings (they sit among
your own), and zsh (the shell you log in with). For all but zsh it offers
to forget them instead.

When apt has dependencies left that nothing needs, yardo says so; it
doesn't run `apt autoremove` itself, which could take packages you
installed by hand.

### The tabs: what to install

1. **Dev → Languages** (mise): Go + gopls, delve, govulncheck,
   golangci-lint; Python 3.13 + uv + ruff; Node + pnpm + TypeScript;
   Java 25 + Maven + Gradle + Kotlin; Rust + rust-analyzer +
   cargo-binstall. Any other language is a search and ctrl-a away (an
   extra mise tool: `bun`, `deno`, `zig`, `dotnet`, …).
2. **Dev → Dev tools:** dev CLIs and linters as one item (just, Task,
   watchexec, hyperfine, xh, hurl; shellcheck, shfmt, actionlint,
   gitleaks, pre-commit), database clients (psql, pgcli, usql, mariadb,
   sqlite3, redis-cli), jiractl (Jira from the terminal), restic + rclone
   (backups, cloud storage).
3. **DevOps → Containers & Kubernetes:** Docker (with log rotation and
   10.200.0.0/16 networks) and its tools (lazydocker, dive, hadolint, crane,
   cosign), Podman + skopeo + buildah; Kubernetes as one item (kubectl,
   with oh-my-zsh's aliases such as `kgp`, `kdp`, `kl` under zsh;
   kubectx/kubens, Helm 3, k9s, stern, k3d for local clusters in Docker);
   Kubernetes extras as another (kustomize, helmfile + helm-diff,
   kubeconform, kubelogin, kubie, kubecolor, krew with plugins, Argo CD,
   Flux, Tilt, Skaffold, ctlptl, kind, talosctl).
4. **DevOps → IaC** (mise): Terraform with terragrunt, tflint,
   terraform-docs and checkov, OpenTofu, Packer, Ansible, tack,
   Vault.
5. **DevOps → Cloud** (mise): AWS, Google Cloud, Azure, DigitalOcean, Hetzner,
   cloudflared. AWS is one item with what goes with the
   CLI: AWS's session-manager-plugin (pinned) and aws-ssm-connect (a
   shell on an EC2 instance, picked with fzf, through Session Manager),
   ssmsearch (search SSM Parameter Store) and
   [rds-mysql](https://github.com/eugenetaranov/rds-mysql) (pick an RDS
   MySQL cluster, writer or reader, and connect; the password from an SSM
   parameter it remembers once it logs in, or typed in), and
   `aws_profile` (`aws-profile` too): a menu of your profiles that
   switches this shell's AWS profile, through oh-my-zsh's `asp` when
   it's there (`aws_profile NAME` switches straight to one; oh-my-zsh's
   `aws_profiles` still lists them), in `~/.config/yardo/aws.sh`.
6. **AI → Coding agents:** Claude Code, Codex CLI, Gemini CLI, OpenCode,
   Aider, Goose, omp.
7. **AI → Add-ons & models:** OpenSpec, ECC for Claude Code (agents, rules
   and commands, no hooks), the MCPJungle gateway (runs in Docker; its
   servers: MCP servers, on the menu), Ollama (local models).
8. **Shell & CLI:** one zsh item that brings everything: zsh as the login
   shell, oh-my-zsh with autosuggestions, syntax highlighting, fzf history
   search and completions, fzf-tab (Tab opens an fzf list with directory
   previews), the modern CLI tools (ripgrep, ag, fd, bat, eza, zoxide, fzf,
   delta, direnv, glow) turned on with keys and aliases (z, fzf keys, direnv
   with a quiet `direnv.toml` that loads `.env` files too,
   eza/bat aliases, a shared Terraform/OpenTofu provider cache, kubecolor;
   oh-my-zsh's `gcm` checks out the main branch and pulls it too)
   through `~/.config/yardo/shell.sh` (sourced from yardo's zsh setup and
   `.bashrc` too), and man pages + tldr. It asks for the prompt: Powerlevel10k (ready
   in a one-line lean style with the OS icon, time and Nerd Font icons;
   needs a Nerd Font in your terminal; `p10k configure` restyles it, and an
   existing `~/.p10k.zsh` is kept) or Starship (one line;
   `~/.config/starship.toml`, kept if you have one). zsh's plugins follow
   what's installed: git, wd, sudo, extract, fzf history search and the
   terraform and opentofu aliases (`tfp`, `tfa`, … even before a project's
   `.tool-versions` brings the tool in) always, and aws, kubectl, kubectx,
   helm, docker, gh, mise, rust and golang whenever the tool is on PATH, so
   a tool added later completes in the next shell. Tools without an
   oh-my-zsh plugin (k9s, stern, just, task, uv, terragrunt, vault,
   packer, …) get Tab completion from their own scripts, cached in
   `~/.cache/yardo/zsh-completions` and refreshed only when the tool is
   upgraded. Also: Neovim (latest, from mise) with a curated
   `~/.config/nvim/init.lua` and plugins (fuzzy finding, a file browser,
   git signs, language servers through `:Mason`, completion; press space
   and wait to see the keys; `sudo nvim FILE` edits through sudoedit, with
   your config), tmux (with `~/.tmux.conf`: mouse, splits, 50k history),
   zellij, mosh. Over SSH, `y` in Neovim and a mouse drag in tmux copy to
   your laptop's clipboard (OSC 52); iTerm2 lets that through once you
   allow it, on the laptop:
   `defaults write com.googlecode.iterm2 AllowClipboardAccess -bool true`,
   then quit and reopen iTerm2 (Ghostty, kitty, WezTerm and Alacritty
   need nothing; Terminal.app can't). `.zshrc` stays yours; the
   Neovim and tmux files replace yours, keeping the old ones as `.orig`; keep your
   own lines in the local files instead (see [Your own
   settings](#your-own-settings)).

   **ssh in, and you're in your session.** Over ssh, the shell attaches to
   your running tmux or zellij session, or starts one, so a dropped
   connection loses nothing: ssh in again and you're back where you were.
   The first time, with both installed, one line asks: `t` tmux, `z`
   zellij, `n` never (any other key: not now). Detaching leaves you in the
   plain shell, still connected; if tmux or zellij can't start, you get
   the plain shell and a line saying so. Nothing starts inside tmux or
   zellij, for `ssh vm command`, scp, rsync or git, on the console, or in
   an editor's terminal (VS Code, Cursor, JetBrains). Skip it once with
   `ssh -t vm YARDO_SESSION=off zsh`; for good, or to change your answer,
   Settings → **Session on login** (tmux, zellij, ask, off), or
   `sessions = "off"` in a profile. **Shell helpers**, in
   `~/.config/yardo/helpers.sh` (zsh and bash): `git-make-branch
   <ticket>`, `recent_branches`, `fzf-git-checkout`,
   `default_branch_git`; `kev`, `kgn`, `kdn`, `kgpres`, `sternf`, `keit`,
   `kgpf`, `kdebug` (a throwaway netshoot pod), `kgnami`,
   `k8s_config_switch` (a menu of the kubeconfigs in `~/.kube`, then
   the picked one's contexts), `k8s_pods_restarted_pods_stats`,
   `k8s_node_terminate` and `k8s_delete_ns` (both ask first); `tfreset`,
   `tf-format`, `tfl`; `aws-list-rds`, `aws-ec2-list`,
   `ec2ctl`; `openssl_connect`, `openssl_info_text`.
9. **System → System (root):** etckeeper, admin and debug tools (dig, mtr, tcpdump,
   iperf3, nmap, strace, htop, btop, ncdu, …), NFS/SMB clients.
10. **System → Connectivity (root):** Tailscale and NetBird (log in from
   their details: enter on the row), and the OpenVPN and WireGuard
   clients, connecting this machine to a VPN elsewhere. yardo no longer
   sets up VPN servers, the Docker API over TLS or k3s: a machine that
   has one keeps it running, and yardo says once how to stop it.

### Configuring it

1. **Network access** (Settings): who can reach the MCP gateway, containers'
   published ports (everywhere, Docker's way; this machine and your VPNs;
   or this machine only), each a row of choices.
2. **Git & SSH:** git name, email, your SSH key and defaults as one item,
   with lazygit (rebase on pull, push sets upstream, delta diffs, aliases,
   a global ignore file); one item per git host (GitHub with act to run
   Actions locally, GitLab with glab, for gitlab.com or your own host,
   Bitbucket), all reached over your SSH key; commit signing with SSH,
   global git ignores in
   `~/.config/git/ignore` (`.claude/`, `.python-version`, `.env` and
   `.env.*` but examples, `.envrc`, `.venv/`, `.terraform*` and local state,
   `node_modules/`, Go coverage and `go.work`, caches, editor files; yardo
   keeps them as a block at the
   top and leaves your own lines below it alone), and your dotfiles repo:
   cloned to `~/.dotfiles` (pulled on later runs), then its `install.sh`,
   `bootstrap.sh` or `setup.sh`, or else `stow` for each top-level
   directory. It runs after everything else, so your files win (`[dotfiles]
   repo = "…"` in a personal profile).
3. **Agents:** one item for a `CLAUDE.md` / `AGENTS.md` describing the VM and settings for
   agents left to run unattended (Claude Code may not force-push, sudo, or
   read `~/.mcp`, `~/.ssh` or the agent keys; Codex asks first and works in
   a sandbox; both are told never to rewrite pushed history; merged
   into your settings, not replacing them), agent API keys (Anthropic,
   OpenAI, Gemini), and MCP servers on the MCPJungle gateway (GitHub,
   Jira, Confluence, Bitbucket, Linear, Slack, Figma; Postgres, MySQL,
   Supabase, OpenSearch; AWS (AWS's managed AWS MCP Server, through its
   SigV4 proxy, signing with a profile of the AWS CLI's: adding or editing
   it lists your `~/.aws` profiles, checks the one you pick (an SSO login
   run out is renewed there and then, `aws sso login --use-device-code`)
   and asks whether the agent may call AWS APIs (yes, the default: what
   the profile's role allows, said when it's an admin one; no is
   read-only, AWS's docs and guides only); the server's screen says
   which; it reads `~/.aws` read-only, as
   you, so a later `aws sso login` reaches it without a sync; it replaces
   the retired AWS API server, `aws-api`, which a
   machine set up before shows as retired, to delete), Azure, Kubernetes,
   Docker, Grafana, Jenkins;
   Playwright, context7, …; read-only where the server allows it (Postgres
   in restricted mode, OpenSearch with writes off, Slack's read tools only,
   Grafana, Azure, MySQL and Kubernetes read-only; each server's settings
   say how to let it write), no usage reports, and
   Docker is root on the machine), added to
   Claude Code, Codex, Gemini CLI, OpenCode and Goose. The gateway can be
   reached from this machine only; from this machine and your VPNs
   (Tailscale, NetBird, WireGuard, OpenVPN: it listens everywhere, and
   yardo's firewall lets its port in on those interfaces only, so a VPN
   that comes up later needs nothing redone); from the internet, at a
   domain name (Caddy, from its own apt repository, serves it over HTTPS
   with an automatic certificate and passes requests to the gateway on
   loopback; fail2ban bans addresses that keep failing its token; point
   the name's DNS here and open TCP 80 and 443); or from everywhere, plain
   HTTP. Beyond this machine it needs a token
   (MCPJungle's enterprise mode): yardo makes the admin token (kept in
   `~/.mcp`, the gateway user's) and one for the agents on the VM, wired
   into them, and one for your laptop, in `~/.mcp/clients/client.token`
   (read it with `sudo cat`; the home's Connection says where). More
   laptops: Connection's **n** (or `mcp-sync client <name>`), sent as
   `Authorization: Bearer <token>`; **d** there revokes one. Neither shows a token: it goes to
   `~/.mcp/clients/<name>.token`. The
   laptop only ever talks to the gateway: the servers' own tokens
   (GitHub, Jira…) stay on this machine. On this machine only, there
   are no tokens: anything on the VM can use the gateway.
   **The servers** are the menu's **MCP servers**: each one you have, by
   name and type (`jira-work` · jira), with how it is (ready, needs
   `JIRA_API_TOKEN`, didn't register), and the ones found in `~/.mcp` that
   yardo didn't make. **Space turns one off** (or on): it stays
   registered with its settings, but the gateway hides its tools from
   every client, so they're not in your agents' context or within their
   reach; **All servers**, above them, turns them all off and back on,
   each as you left it. A server that's off doesn't ask for anything on
   the home. New agent sessions see the change. It keeps tools out of
   everyday sessions; it isn't a lock (an agent with sudo can turn one
   back on). Enter on one edits its settings (its
   `~/.mcp/<name>/env` in your editor, then it registers), renames it (its
   settings move with it) or deletes it (off the gateway, with its
   settings). **Add a server** shows the catalog; then its name, the
   server's own (`jira`) unless you give another (`work` makes it
   `jira-work`: a second Jira, another AWS account), and once the run has
   made its env, that opens in your editor: its keys, the options
   commented out and how to get each value. Adding tools doesn't ask about
   MCP servers. Whatever you change through yardo registers right after,
   and it says how that went (or why not, on the server's screen, with Try
   again). yardo keeps watch: opening it registers servers whose settings
   changed since. From the shell: `sudo yardo mcp` (the screen),
   `sudo yardo mcp edit <name>`, `sudo yardo mcp sync`,
   `sudo yardo mcp status`, `sudo yardo mcp off|on [<name>…]` (no name:
   all of them). A profile lists another instance as
   `"grafana:prod"` in `[mcp] servers`.
   **Logins you already have** fill in what's empty, so a token isn't
   typed twice: GitHub (github.com) gets gh's token after `gh auth login`,
   and `jira` and `confluence` get jiractl's server, user and token after
   `jiractl configure` (Confluence's URL only on Atlassian Cloud;
   `jira-work` and other instances are other accounts, so not those).
   yardo marks each key it filled (`# yardo fills KEY from gh`) and keeps
   it current on every run; delete that line to keep a value as it is. A
   token you put in yourself is never replaced: gh's token reaches all you
   can, so put a fine-grained one there to narrow it.
   **Agent skills** (on by default): yardo's `git-commits`,
   `git-branches`, `pr-descriptions` and `jira-tickets` skills, in `~/.agents/skills`
   (Codex, Gemini CLI, OpenCode, Goose, pi, omp) and `~/.claude/skills`
   (Claude Code): short commit messages that say why, branches named
   `DEVOPS-142-short-description` (the Jira key first, when there's one;
   a project's own convention wins: its docs, its recent branches, or a
   prefix kept with `git config yardo.branchPrefix tech/`, per repo or
   `--global`; `git-make-branch` names them the same), a PR description
   that's a note to the reviewer, and tickets a teammate would write: a
   title, one to three plain sentences, and three to five acceptance
   criteria, each checkable (required), under a hundred words. yardo
   updates only the copies it wrote; one you edit stays yours. **No
   agent's name on commits:** Claude Code's `attribution` setting is off,
   the agents' notes say so, and on Git 2.54+ a global commit-msg hook
   (config-based, so your repos' own hooks still run) drops an agent's
   `Co-authored-by` and "Generated with" lines, never a person's (older
   git on Ubuntu is upgraded from the git-core PPA first). Codex
   takes it from your ChatGPT account: turn commit attribution off there.
   **Community skills** (off by default), shipped inside yardo at pinned
   commits with their licences: superpowers' `systematic-debugging` and
   `verification-before-completion`, Matt Pocock's `tdd` and `grill-me`,
   Sentry's `find-bugs`, `humanizer`, and Anthropic's `skill-creator` and
   `webapp-testing`. Nothing is fetched on the VM; `scripts/update-skills`
   re-vendors them (`internal/assets/skills.lock.toml`).
4. **VPN logins** (in Tailscale's and NetBird's rows): the Tailscale login (an auth key, or `sudo tailscale
   up` later), the NetBird login (a setup key, on NetBird's cloud or your
   own management URL, or `sudo netbird up` later).

In a profile, a split item's old id means both halves: `tailscale` and
`mcpjungle` are the configure items, and each pulls in its install item
(`tailscale-install`, `mcpjungle-install`), which a profile can also list
alone. sops + age is no longer an item (the ones installed stay). Merged items' old ids still work in
saved choices: `ctools` is `docker`, `lint` is `devcli`, `gitcfg` is
`gitid`, `lazygit` is `gitid`, `agentsafe` is `agentcfg` and `k3d` is `kind`.

Downloads are pinned: vendor apt keys are checked against their
fingerprints and release binaries against their sha256 before they're
installed. The few vendor installers that change in place (Tailscale,
Ollama, krew, omp, Neovim's plugins) can't be pinned; `~/yardo.txt`
lists the installed ones under "Installed without a pinned checksum", with
why. A `catalog.d` item that downloads must do the same (`sha256`, or
`unverified = "why"`), or yardo refuses to load it.

When you press a and confirm, it asks every typed question in one place:
git name and email, timezone, and the credentials of
the items you added, then installs. To change an answer later (git name
and email, the zsh prompt, the dotfiles repo, the MCP servers), press enter
on the item and choose Change: yardo asks again and applies just that item. `~/yardo.txt` lists everything installed so far,
grouped by how it was installed, with how to update or remove it.

### Your own settings

`~/.zshrc` is yours. yardo keeps one small marked block at its top,
which loads yardo's zsh setup (`~/.config/yardo/zshrc`: oh-my-zsh, the
plugins, the prompt), and never touches another line. Add aliases,
exports and tools' install lines below the block: they stay through every
run, and they load after yardo's setup, so yours win. If you delete the
block, the next run puts it back on top. On a machine set up by an older
yardo, the first run saves the `.zshrc` it wrote as
`~/.zshrc.before-yardo-split` and says so: copy any lines you'd added
there into `.zshrc`.

yardo rewrites `~/.config/nvim/init.lua`, `.tmux.conf` and
`~/.config/yardo/shell.sh` on a re-run, so lines you add to them are
lost (your first version of each is kept once, as `.orig`). Put your own
settings in these files instead. yardo never creates or changes
them, and the files it writes source them last, so yours win:

| File | Read by |
| --- | --- |
| `~/.zshrc.local` | zsh (end of yardo's zsh setup, still read) |
| `~/.config/yardo/local.sh` | zsh and bash (end of `shell.sh`) |
| `~/.tmux.local.conf` | tmux (end of `.tmux.conf`) |
| `~/.config/nvim/local.lua` | Neovim (end of `init.lua`) |

For example, `echo 'export KUBECONFIG=~/.kube/work' >> ~/.config/yardo/local.sh`
survives `sudo yardo --yes`.

### Credentials

yardo never stores or ships a credential. An item that needs one
(agent API keys, MCP server keys, a GitHub token) asks for it once and writes it straight into that tool's own
config file, readable only by you (0600). The agent keys are optional:
Claude Code, Codex and Gemini can each log in by themselves instead. Leave a value empty, or run with
`--yes`, and the file is still written, with each key left empty and a
comment saying where to get it. Either way, `~/yardo.txt` ends with
**Secrets to fill in**: one line per value still missing, with its file,
the key, where to get it, and the command that applies it (`mcp-sync`,
…). The run ends with how many are missing.

| Item | File |
|---|---|
| Agent API keys | `~/.config/yardo/agent.env`, exported by interactive shells only (not services or cron) |
| MCP servers | `~/.mcp/<name>/env`, owned by the gateway's user: `sudo yardo mcp edit <name>` (it registers it after) |

Each user's MCPJungle gateway runs as a system user of its own,
`mcp-<you>`, which owns `~/.mcp`: a coding agent running as you can use the
MCP servers through the gateway but can't read their tokens. `mcp-sync` and
`mcp-sync edit <name>` switch to that user through sudo for you. This only
holds while the agent can't get root: with passwordless sudo (the default
for the VM's user) or membership of the docker group, it can; the
"Set up agents for this VM" item denies Claude Code sudo.

`sudo yardo --yes` never waits for input: a step that would (`tailscale
up` without a key) leaves a note naming the command to run instead.

**Git and SSH.** The Git item asks your name and email and sets them
globally. Adding GitHub, GitLab or Bitbucket asks for its SSH key, on one
page: **Make a new key** (ed25519, at `~/.ssh/identities/<github|gitlab|bitbucket>`,
shown with where to add it), **Use a key on this machine** (one of your
private keys under `~/.ssh`, left where it is), **Paste a key** (saved to
`~/.ssh/identities/<host>`), or **No key here** (a forwarded agent, or
HTTPS). A key in `~/.ssh/identities` is never replaced without asking: its
type, fingerprint and comment are shown, with Use it or Replace it (the old
one is moved to `<host>.old-<date>`). Change a host's key from its settings.
`--yes` and profiles ask nothing: a new host gets `~/.ssh/identities/<host>`,
made when missing. A machine set up by an earlier yardo keeps the key it
uses (`~/.ssh/id_ed25519`, `~/.ssh/<host>`, a sent key, or an agent).

yardo's SSH settings are in a file of their own, `~/.ssh/config.d/yardo-git`,
which one `Include` line at the top of `~/.ssh/config` brings in; nothing
else in your config changes (an earlier yardo's block there is moved out).
A git host your own `~/.ssh/config` sets (`Host github.com`, say) is left to
you, and yardo says so. Bitbucket is reached at `ssh.bitbucket.org` (remotes
naming `bitbucket.org` keep working), and GitHub's and Bitbucket's host keys
come from their published lists, not a first-use scan. **Accept new hosts'
SSH keys** (Settings, off by default; `ssh_accept_new = true` in a profile)
has ssh trust a host it hasn't seen before without the fingerprint prompt,
still refusing a changed key; it's in `~/.ssh/config.d/yardo-defaults`,
included at the end of `~/.ssh/config`, so a `StrictHostKeyChecking` of
yours wins. Nothing is uploaded and nothing logs in: run `gh auth login`
yourself when you want gh to. **Sign commits with SSH**
signs commits and tags with the agent's key or the VM's key, uploads it as
a signing key, and writes `~/.config/git/allowed_signers`.

**`sudo yardo forget`** undoes what yardo created, and only
that: the GitHub keys it uploaded, the values in the credential files it
wrote (the keys stay, so a re-run asks again), the gh and Tailscale
logins it made, and the registry logins and `~/.npmrc` block that older
versions made. It lists them and
asks first (`--yes` doesn't), then prints where to revoke the tokens it
can't revoke itself. Run it before you throw a machine away.

Run it again any time to add more. Choices accumulate, never with
secrets, and are only read as data, never run:

- machine-wide items (the System tab, Ollama), the timezone, the users
  set up and their MCPJungle ports in root-only
  `/var/lib/yardo/system.json`;
- each user's own items and settings in their `~/.config/yardo/answers.json`.
  yardo reads that file as the user's input: it won't follow a symlink
  there, and a machine-wide item listed in it is ignored with a warning.

`sudo yardo --yes` re-applies everything for every user set up;
`--yes --user ada` only ada's. Several users can share a VM: each gets their
own MCPJungle gateway (`mcpjungle@<user>`, on port 8080, 8081, …), wired into
their own agents.

**An SSH key for git you send.** Send a private key (a deploy key, or one
on your GitHub, GitLab or Bitbucket account) from your machine:

```sh
ssh ubuntu@web1 'sudo yardo gitkey' < ~/.ssh/deploy_key
ssh ubuntu@web1 'sudo yardo gitkey bitbucket' < ~/.ssh/bitbucket_key
ssh ubuntu@web1 'sudo yardo gitkey --host git.example.com' < ~/.ssh/id_ed25519
```

GitHub's (the default), GitLab's and Bitbucket's key goes to
`~/.ssh/identities/<host>`, as a pasted one does; a key already there is
kept unless you add `--replace` (the old one is moved aside). Another
host's goes to `~/.ssh/yardo_git_key`. `~/.ssh/config.d/yardo-git` points
the host at it, so it works for any repo the key may reach: a deploy key's
one repo, or all of yours for an account key. yardo saves only the hosts,
never the key.

**Your team's own items and MCP servers.** yardo reads extra items from
`/etc/yardo/catalog.d/*.toml` (the catalog's format; see
`internal/catalog/data`) and extra MCP server specs from
`/usr/local/share/mcp-catalog/*.spec`; specs named in that directory's
`.team-files` win over yardo's own of the same name. A download in an item
must be pinned (`sha256`) or say why it can't be (`unverified = "…"`), or
yardo refuses to load it.

**Away from home, and back.** For a VM on your home network (Proxmox,
say), run a mesh VPN on the VM itself (Tailscale or NetBird, on Tools →
System → VPN logins) and on the laptop, and always use the VM's VPN
address or name (`payzoff.<tailnet>.ts.net`, `payzoff.netbird.cloud`), at
home too. At home Tailscale finds the
direct path over your LAN, so it's as fast as the LAN; away it goes
through the tunnel. The name and address don't change when the laptop
changes network, so ssh, known_hosts and VS Code Remote-SSH keep one host.
Connection's SSH row (and `yardo connect`) has the name and a ready
`~/.ssh/config` block (one `Host`, `HostKeyAlias`, keep-alives), **c**
copies it; tmux on the VM keeps sessions across sleep. Things to avoid:
- The VM's LAN address: VPN-only services (the MCP gateway, container
  ports) aren't open on it, and it's unreachable away from home.
- A subnet router for the whole home LAN (192.168.0.0/24): traffic arrives
  on the VM's LAN side, where those services are closed, and cafés and
  hotels use the same range. If you need the Proxmox UI, advertise just its
  /32 from a small LXC.
- Port forwards on your router for ssh or the gateway.
- Ephemeral auth keys for VMs (the node is removed once offline). Use a
  tagged auth key: tagged nodes never expire, and a grant like
  `{"src": ["autogroup:member"], "dst": ["tag:devvm"], "ip": ["tcp:22", "tcp:8081"]}`
  limits who reaches them. An untagged VM's key expires after 180 days;
  Connection's Tailscale row (and `yardo connect`) says when.
- NetBird: the same with a reusable setup key (not ephemeral) whose
  auto-group, `devvm` say, is in a policy like *laptops → devvm, TCP 22
  and 8081*; a peer that needs approval waits for it in the dashboard
  (yardo says so). Run a self-hosted NetBird on a VPS, not on the home
  network it's meant to reach.
- Two mesh VPNs on the laptop at once (Tailscale and NetBird share
  100.64.0.0/10).

yardo sets Tailscale to accept no subnet routes on the VM and makes you
its operator (`tailscale serve` without sudo), names the NetBird peer
after the machine, and logs in to neither again once the VM is on it;
`sudo yardo forget` logs the VM out of both (a deleted VM stays in your
tailnet or NetBird network otherwise).

**A VPN client on the VM** (the WireGuard or OpenVPN client item, to reach
another network from it) doesn't take the way in: even with a profile that
sends all traffic into the tunnel, replies from the VM's local address and
traffic to the local network go out the way they did before (a routing
table of yardo's, set up each time the tunnel comes up), so ssh over the
LAN still works, and Tailscale and NetBird keep their own routes.

**Reaching this machine from a laptop: the Connection screen.** The
home's **Connection** panel has the short version (`ssh vm`,
`docker context use vm`, the gateway's URL and where its token is);
enter on it, or Go to → Connection, opens the screen:

- **Networks**: Tailscale and NetBird when installed, each WireGuard and
  OpenVPN client profile (`/etc/wireguard/<name>.conf`,
  `/etc/openvpn/client/<name>.conf`), and the local network, each with
  its state, address and name, and the one your session came in by
  marked *← this session*. **space** stops a network or starts it again
  (`tailscale down`/`up`, `netbird down`/`up`, `systemctl stop`/`start`
  of the profile's unit), in the background, the screen kept; it never
  logs out or deletes anything. A failure says its last line on the row.
  **b** turns a profile's start at boot on or off.
- **The warning.** Stopping or removing the network your ssh session came
  in by (yardo reads `SSH_CONNECTION`) asks first, no by default, and
  names the way back: another address of the machine, or its console.
  Any other network stops without a question. When yardo can't tell
  which network a session came by, every stop warns.
- **Setting up.** Enter on Tailscale or NetBird not set up asks for its
  key and logs in (the login item's run); Tailscale with no key shows its
  login link on the row (**c** copies it) and turns connected once you've
  logged in in a browser. **n** adds a profile: WireGuard or OpenVPN, a
  name (it names the interface: lowercase, 15 at most), and a file on
  the machine or the profile pasted. yardo checks it's a profile of that
  kind (`[Interface]` with a `PrivateKey` and a `[Peer]`; a `remote`
  line) or refuses it, storing nothing; stores it root's only (0600);
  starts it at boot and now; and says whether it connected. One that
  sends all traffic through it says so first. The client is installed
  first when it isn't.
- **Removing.** **d** asks, saying what goes and what stays: Tailscale is
  logged out (the machine stays in the tailnet's admin console until you
  delete it there), NetBird is disconnected and logged out, a profile is
  stopped, taken off boot and its file deleted. `yardo forget` then has
  nothing left to log out.
- **From your laptop**: SSH (the Host block), Docker (a context over SSH,
  which opens no port and needs no certificate; bind mounts are this
  machine's paths, `-p` ports open here), and with the MCP gateway
  Claude Code (`claude mcp add`), Codex (`~/.codex/config.toml`) and
  other clients (the URL and the header), each with this machine's
  address (its VPN's first), the exact lines and how you sign in. **c**
  copies the row's lines to the laptop's clipboard through the terminal
  (OSC 52; through tmux too); with a token, it's read from its file as
  it's copied, never drawn. iTerm2 needs it allowed once (the screen
  says how), Terminal.app can't. When the gateway is this machine's
  only, the rows have the ssh tunnel to use instead.
- **Gateway tokens**: a token per client, by name and file, never the
  value. **n** makes one (`mcp-sync client <name>`), **d** revokes one,
  asked first: the gateway refuses it from then on and its file goes
  (`mcp-sync client-rm <name>`).
- **All of it, one script**: `ssh you@vm sudo yardo connect --script >
  yardo-vm.sh`, read it, then `sh yardo-vm.sh` on the laptop. It adds the
  Host block, the Docker context and, with claude there, the gateway to
  Claude Code, each once (run it again: it adds nothing). It holds no
  token: it reads it over ssh when it runs.
- **Keys to add**: a key a run made (a git host's, with where to add it
  and the key alone on a line), until the host reads working on Tools or
  you dismiss it with **d**.

Tools' VPN details say the same: Log in and Log out there run the same
login and logout. `sudo yardo connect` prints the laptop's side as text.

Containers' published ports are
reachable from everywhere, as Docker does it, unless Settings → Network
access → "Container ports" keeps them to the VPNs or this machine.
yardo's firewall rules are nftables tables of its own in
`/etc/yardo/firewall` (loaded at boot by `yardo-firewall.service`): they
only keep these ports out, and leave Docker's, the VPNs' and anyone
else's rules alone.

**Status.** `sudo yardo status` checks every item you picked, changing
nothing, and lists the ones that aren't right first: `down` (set up, but
its service isn't running), `partial` (some of its steps done),
`missing`, `drifted` (applied by yardo before, not right now), then
values to fill in (a credential left empty: which file,
which key), then `ok`. `--json` prints it for scripts (`"schema": 1`, every
user set up), and each run keeps the latest in
`/var/lib/yardo/status/<user>.json`. `--updates` also asks apt and mise
what's newer (slow: it refreshes apt's lists) and lists it per item; it
never upgrades (`mise up`, `sudo apt upgrade` do; the home looks once a day,
in the background, and its Install updates row opens Tools › Updates,
everything queued).
Each run leaves a copy for you in
`~/.cache/yardo/status.json`: `yardo status` without sudo shows that one,
and how old it is. `sudo yardo` shows the same marks on its screen. `sudo yardo --verify` is the same
check, failing when something is down, partial or missing.

**Profiles.** `sudo yardo export --name team > team.toml` writes the choices on
a VM as a profile: a TOML file of picks and plain settings (git identity,
timezone, MCP servers), never secrets. `sudo yardo --profile team.toml
--yes` applies one (`--profile` repeats, later files win, so a team file and
a personal one combine); a profile can also be an https URL. Without `--yes`, `--profile` shows the plan for review and asks for the
secrets it can't hold; `--dry-run` shows what it would do, changing
nothing. A profile with `name = "team"` (what `export --name team` writes)
is a layer: applying a newer `team` profile replaces the picks the old one
made, and lists the ones it dropped (they stay installed, but aren't
re-applied). An unnamed profile only ever adds picks. `yardo --list` lists
every item's id, for writing `picks` by hand. Picks you
made yourself are never dropped. `user = "ada"` in a profile, or
`--user ada` (which wins), sets up that account, creating it like the VM's
first user (passwordless sudo, same groups) if it doesn't exist.
`yardo export --lock` adds a `[lock]` table with the installed
versions of the picked items' mise tools and apt packages. Applying a
locked profile installs exactly those mise versions, and those apt
versions while the archive still has them (otherwise the current one, with
a note naming the package), so a machine built next month matches.

**Automatic security updates.** Some launchers' cloud-init turns
unattended-upgrades and the apt-daily timers off at boot, since on first boot
they hold the dpkg lock and make installs fail. The "Automatic security
updates" item (System) turns them back on and leaves
`/etc/yardo/auto-upgrades`, which such a cloud-init can check to stand down.

## Development

```sh
task build        # bin/yardo for this machine
task build:linux  # bin/yardo-linux-amd64, -arm64
task test         # go test -race ./...
task lint
HOST=ubuntu@10.0.0.5 task e2e   # every item on a throwaway machine over SSH
task mcp-audit    # each MCP server's pin next to its latest release
```

Before a minor release, `task mcp-audit` and each server's changelog:
pin the newest release that's settled (a few days old, not a new major
without reading what broke), drop a server its maker retired, and check
that a description saying read-only still is. Every image carries a tag
and every `npx`/`uvx` package an exact version, or the spec test fails.

The bash part (`internal/assets/yardo.sh`, `picker.py`) still runs the GitHub
login and commit-signing items, and is the fallback on other architectures;
it goes once those are ported to Go (see `openspec/`).

## License

MIT
