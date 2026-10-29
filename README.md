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
account is created). Once something is set up, it opens on a short menu:
how the machine is, in three lines, then what you can do, in plain
words, changing with the machine: **Fix 2 problems** (one card each:
fix it, or see how it's set up), **Fill in 1 secret** (opens its file in
your editor), **Add or remove tools**, **MCP servers** (the gateway's
list: add one, or another instance of one), **Who can connect** (the MCP
gateway, containers' published ports and the Docker API, each a row of
choices with a sentence saying what it means), **Connect from a laptop**
and **Check for updates**. Esc always goes back. **Add or remove
tools**, and the first run, open the full screen: tabs by area (**Dev**, **DevOps**,
**AI**, **Shell & CLI**, **System**, then **This VM**) above a checklist,
where a ticked box means installed and a mark beside it how it is: `✓`
ready, `✗` stopped or not installed, `~` half set up or changed by hand,
`?` needs a value, `↑` a newer version is out.

- **space** queues adding (`[+]`) or removing (`[-]`) an item; removing one
  stops yardo looking after it, and it stays installed.
- **enter** acts on the row: fix what's broken (apply it again, now), fill
  in a missing value (in your editor), change a setting or an answer (git
  name and email, the prompt, the dotfiles repo, the MCP servers), or show
  an item's details.
- **a** applies the queue: it asks what the new items need (git name,
  prompt, MCP servers, keys), then installs. **Review**, the last tab,
  lists the queue.
- **/** searches every tab; for a tool the catalog doesn't have, ctrl-a
  adds it as an extra mise tool.
- **Needs you** comes first while something does: what's broken, what
  needs a value, what has updates. **This VM** holds git, agents, logins
  and system settings, who can reach the MCP gateway, containers'
  published ports and the Docker API, the accent colour, how to connect
  from a laptop, and checking for updates or checking again.

**Accent colour.** This VM → Look → Accent colour: Coral (the default),
Lagoon (aqua blue), Pistachio, Lavender, Amber or Rose. yardo's screen
takes it, and so does your shell, so each machine's terminals can be
told apart at a glance: the prompt (Powerlevel10k's OS icon, directory
and prompt character, or Starship's), tmux's status bar and active pane
border, and `$YARDO_ACCENT` for your own scripts. yardo only rewrites a
prompt config it wrote (its first line says so); after `p10k configure`,
or once you take that line off your `starship.toml`, the file is yours.

The first run comes with the usual stack queued: zsh, Neovim, tmux,
zellij, the linters, the dev CLIs, the database clients, the admin and
debug tools, Docker and its tools, Claude Code with OpenSpec, the
MCPJungle gateway, git (name and email, defaults, lazygit, GitHub) and
the two agent items. Space takes off what you don't want; a installs.
Later runs show what's installed ticked and checked, and ask only about
what's new. `sudo yardo --ui board` opens on this screen instead of the
menu.

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
(git, curl, jq, gh, build tools). Configuring: automatic security updates
(no auto-reboot) with needrestart restarting services quietly; weekly
updates of your tools (a systemd timer running `mise up` and `krew upgrade`
as you); SSH keys only, no root login
(skipped, with a note, while you have no authorized key; a config sshd
rejects is rolled back); fail2ban for SSH, on a machine with a public IPv4
on an interface (Tailscale addresses are never banned; a cloud VM behind
1:1 NAT only sees its private address, so it gets none); dev limits
(inotify, open files), zram swap, a journal size cap, timezone (asked once)
+ locale. Keeping the clock in sync is left to the machine's launcher.

### The tabs: what to install

1. **Dev → Languages** (mise): Go + gopls, delve, govulncheck,
   golangci-lint; Python 3.13 + uv + ruff; Node + pnpm + TypeScript;
   Java 25 + Maven + Gradle + Kotlin; Rust + rust-analyzer +
   cargo-binstall. Any other language is a search and ctrl-a away (an
   extra mise tool: `bun`, `deno`, `zig`, `dotnet`, …).
2. **Dev → Dev tools:** dev CLIs and linters as one item (just, Task,
   watchexec, hyperfine, yq, xh, hurl; shellcheck, shfmt, actionlint,
   gitleaks, pre-commit), database clients (psql, pgcli, usql, mariadb,
   sqlite3, redis-cli).
3. **DevOps → Containers & Kubernetes:** Docker (with log rotation and
   10.200.0.0/16 networks) and its tools (lazydocker, dive, hadolint, crane,
   cosign), Podman + skopeo + buildah; Kubernetes as one item (kubectl,
   with oh-my-zsh's aliases such as `kgp`, `kdp`, `kl` under zsh;
   kubectx/kubens, Helm 3, k9s, stern, k3d for local clusters in Docker);
   Kubernetes extras as another (kustomize, helmfile + helm-diff,
   kubeconform, kubelogin, kubie, kubecolor, krew with plugins, Argo CD,
   Flux, Tilt, Skaffold, ctlptl, kind, talosctl); or k3s as a real cluster
   on the VM.
4. **DevOps → IaC** (mise): Terraform with terragrunt, tflint,
   terraform-docs and checkov, OpenTofu, Packer, Ansible, tack, sops + age,
   Vault.
5. **DevOps → Cloud** (mise): AWS, Google Cloud, Azure, DigitalOcean, Hetzner,
   cloudflared, restic + rclone.
6. **AI:** Claude Code, Codex CLI, Gemini CLI, OpenCode, Aider, Goose, omp,
   Ollama, OpenSpec, ECC for Claude Code (agents, rules and commands, no
   hooks), and the MCPJungle gateway (its servers: MCP servers, on the menu).
7. **Shell and CLI:** one zsh item that brings everything: zsh as the login
   shell, oh-my-zsh with autosuggestions, syntax highlighting, fzf history
   search and completions, fzf-tab (Tab opens an fzf list with directory
   previews), the modern CLI tools (ripgrep, fd, bat, eza, zoxide, fzf,
   delta, direnv) turned on with keys and aliases (z, fzf keys, direnv
   with a quiet `direnv.toml` that loads `.env` files too,
   eza/bat aliases, a shared Terraform/OpenTofu provider cache, kubecolor)
   through `~/.config/yardo/shell.sh` (sourced from `.zshrc` and `.bashrc`
   too), and man pages + tldr. It asks for the prompt: Powerlevel10k (ready
   in a one-line lean style with the OS icon, time and Nerd Font icons;
   needs a Nerd Font in your terminal; `p10k configure` restyles it, and an
   existing `~/.p10k.zsh` is kept) or Starship (one line;
   `~/.config/starship.toml`, kept if you have one). zsh's plugins follow
   what's installed: git, wd, sudo, extract and fzf history search always,
   and aws, terraform, opentofu, kubectl, kubectx, helm, docker, gh, mise,
   rust and golang whenever the tool is on PATH, so a tool added later
   completes in the next shell. Tools without an oh-my-zsh plugin (k9s,
   stern, just, task, uv, terragrunt, vault, packer, …) get Tab completion
   from their own scripts, cached in `~/.cache/yardo/zsh-completions` and
   refreshed only when the tool is upgraded. Also: Neovim (latest, from
   mise) with a curated `~/.config/nvim/init.lua` and plugins (fuzzy
   finding, a file browser, git signs, language servers through `:Mason`,
   completion; press space and wait to see the keys; `sudo nvim FILE`
   edits through sudoedit, with your config), tmux (with
   `~/.tmux.conf`: mouse, splits, 50k history), zellij, mosh. The zsh,
   Neovim and tmux files replace yours, keeping the old ones as `.orig`; keep your
   own lines in the local files instead (see [Your own
   settings](#your-own-settings)).
8. **System (root):** etckeeper, admin and debug tools (dig, mtr, tcpdump,
   iperf3, nmap, strace, htop, btop, ncdu, …), Tailscale and NetBird (log
   in under Configure), NFS/SMB clients, the OpenVPN client, and an OpenVPN server:
   its own CA (easy-rsa), UDP 1194, clients on 10.8.0.0/24 with all their
   traffic sent through the machine (NAT, through Docker's DOCKER-USER
   chain when Docker is there), and a profile for you in
   `/srv/openvpn/<you>.ovpn`, root only (change its `remote` if clients
   reach the machine by another address, and open UDP 1194 in the cloud
   firewall). WireGuard the same way: UDP 51820, clients on
   10.9.0.0/24, configs in `/srv/wireguard/<name>.conf`. Users, for
   either: `sudo yardo-vpn openvpn|wireguard client|list|revoke <name>`
   (`wireguard qr <name>` shows a config as a QR code for the phone app).
   The OpenVPN and WireGuard clients are for the other way round:
   connecting this machine to a VPN elsewhere.

### This VM: configuring it

1. **Git & SSH:** git name, email and defaults as one item, with lazygit (rebase on pull, push
   sets upstream, delta diffs, aliases, a global ignore file), one item per
   git host (GitHub with the gh login and act to run Actions locally,
   GitLab with glab, for gitlab.com or your own host, Bitbucket), each asking how this machine reaches it over
   SSH (a forwarded agent, a key made here, or a key you send; GitHub also
   a token), commit signing with SSH, global git ignores in
   `~/.config/git/ignore` (`.claude/`, `.python-version`, `.env` and
   `.env.*` but examples, `.envrc`, `.venv/`, `.terraform*` and local state,
   `node_modules/`, Go coverage and `go.work`, caches, editor files; yardo
   keeps them as a block at the
   top and leaves your own lines below it alone), and your dotfiles repo:
   cloned to `~/.dotfiles` (pulled on later runs), then its `install.sh`,
   `bootstrap.sh` or `setup.sh`, or else `stow` for each top-level
   directory. It runs after everything else, so your files win (`[dotfiles]
   repo = "…"` in a personal profile).
2. **Agents:** one item for a `CLAUDE.md` / `AGENTS.md` describing the VM and settings for
   agents left to run unattended (Claude Code may not force-push, sudo, or
   read `~/.mcp`, `~/.ssh` or the agent keys; Codex asks first and works in
   a sandbox; both are told never to rewrite pushed history; merged
   into your settings, not replacing them), agent API keys (Anthropic,
   OpenAI, Gemini), and MCP servers on the MCPJungle gateway (GitHub,
   Jira and Confluence, Linear, Slack, Figma; Postgres, MySQL,
   Supabase, OpenSearch; AWS (AWS's managed AWS MCP Server, through its
   SigV4 proxy; it replaces the retired AWS API server, `aws-api`, which a
   machine set up before shows as retired, to delete), Azure, Kubernetes,
   Docker, Grafana, Jenkins;
   Playwright, context7, …; read-only where the server allows it, and
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
   into them; `mcp-sync client <name>` makes one for a laptop's agent,
   sent as `Authorization: Bearer <token>`. On this machine only, there
   are no tokens: anything on the VM can use the gateway.
   **The servers** are the menu's **MCP servers**: each one you have, by
   name and type (`jira-work` · jira), with how it is (ready, needs
   `JIRA_API_TOKEN`, didn't register), and the ones found in `~/.mcp` that
   yardo didn't make. Enter on one edits its settings (its
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
   `sudo yardo mcp status`. A profile lists another instance as
   `"grafana:prod"` in `[mcp] servers`.
   **Agent skills** (on by default): yardo's `git-commits`,
   `pr-descriptions` and `jira-tickets` skills, in `~/.agents/skills`
   (Codex, Gemini CLI, OpenCode, Goose, pi, omp) and `~/.claude/skills`
   (Claude Code): short commit messages that say why, a PR description
   that's a note to the reviewer, and tickets a teammate would write: a
   title, a few plain sentences, at most five acceptance criteria. yardo
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
3. **Logins & keys:** the Tailscale login (an auth key, or `sudo tailscale
   up` later), the NetBird login (a setup key, on NetBird's cloud or your
   own management URL, or `sudo netbird up` later) and your age key for sops (generate or paste).

In a profile, a split item's old id means both halves: `tailscale`,
`sops` and `mcpjungle` are the configure items, and each pulls in its
install item (`tailscale-install`, `sops-install`, `mcpjungle-install`),
which a profile can also list alone. Merged items' old ids still work in
saved choices: `ctools` is `docker`, `lint` is `devcli`, `gitcfg` is
`gitid`, `lazygit` is `gitid`, `agentsafe` is `agentcfg` and `k3d` is `kind`.

Downloads are pinned: vendor apt keys are checked against their
fingerprints and release binaries against their sha256 before they're
installed. The few vendor installers that change in place (Tailscale,
Ollama, k3s, krew, omp, Neovim's plugins) can't be pinned; `~/yardo.txt`
lists the installed ones under "Installed without a pinned checksum", with
why. A `catalog.d` item that downloads must do the same (`sha256`, or
`unverified = "why"`), or yardo refuses to load it.

When you press a and confirm, it asks every typed question in one place:
git name and email, timezone, how to reach GitHub, and the credentials of
the items you added, then installs. To change an answer later (git name
and email, the zsh prompt, the dotfiles repo, the MCP servers), press enter
on the item and choose Change: yardo asks again and applies just that item. `~/yardo.txt` lists everything installed so far,
grouped by how it was installed, with how to update or remove it.

### Your own settings

yardo rewrites `.zshrc`, `~/.config/nvim/init.lua`, `.tmux.conf` and
`~/.config/yardo/shell.sh` on a re-run, so lines you add to them are
lost (your first version of each is kept once, as `.orig`). Put your own
settings in these files instead. yardo never creates or changes
them, and the files it writes source them last, so yours win:

| File | Read by |
| --- | --- |
| `~/.zshrc.local` | zsh (end of `.zshrc`) |
| `~/.config/yardo/local.sh` | zsh and bash (end of `shell.sh`) |
| `~/.tmux.local.conf` | tmux (end of `.tmux.conf`) |
| `~/.config/nvim/local.lua` | Neovim (end of `init.lua`) |

For example, `echo 'export KUBECONFIG=~/.kube/work' >> ~/.config/yardo/local.sh`
survives `sudo yardo --yes`.

### Credentials

yardo never stores or ships a credential. An item that needs one
(agent API keys, MCP server keys, an npm or registry token, an age key, a
GitHub token) asks for it once and writes it straight into that tool's own
config file, readable only by you (0600). Leave a value empty, or run with
`--yes`, and the file is still written, with each key left empty and a
comment saying where to get it. Either way, `~/yardo.txt` ends with
**Secrets to fill in**: one line per value still missing, with its file,
the key, where to get it, and the command that applies it (`mcp-sync`,
…). The run ends with how many are missing.

| Item | File |
|---|---|
| Agent API keys | `~/.config/yardo/agent.env`, exported by interactive shells only (not services or cron) |
| MCP servers | `~/.mcp/<name>/env`, owned by the gateway's user: `sudo yardo mcp edit <name>` (it registers it after) |
| age key | `~/.config/sops/age/keys.txt`, with `SOPS_AGE_KEY_FILE` set |

Each user's MCPJungle gateway runs as a system user of its own,
`mcp-<you>`, which owns `~/.mcp`: a coding agent running as you can use the
MCP servers through the gateway but can't read their tokens. `mcp-sync` and
`mcp-sync edit <name>` switch to that user through sudo for you. This only
holds while the agent can't get root: with passwordless sudo (the default
for the VM's user) or membership of the docker group, it can; the
"Set up agents for this VM" item denies Claude Code sudo.

`sudo yardo --yes` never waits for input: a step that would (the
GitHub browser login, adding a gh scope, `tailscale up` without a key)
leaves a note naming the command to run instead.

**GitHub** asks how the VM should reach it:

- **Forwarded agent** (the default when you connected with
  `ssh -A`): no key on the machine. git over SSH uses your laptop's
  agent, and gh logs in with the browser code, without permission to add
  keys to your account.
- **A key on this VM**: `~/.ssh/github` (optionally with a passphrase)
  uploaded to your account, and `~/.ssh/config` points github.com at it.
  GitLab's and Bitbucket's are `~/.ssh/gitlab` and `~/.ssh/bitbucket`. A
  machine already using `~/.ssh/id_ed25519` (an earlier yardo's) keeps it.
  For an organization with SAML SSO, authorize it at
  github.com/settings/keys → Configure SSO.
- **A fine-grained token**: pasted once, used by gh and by git over HTTPS;
  nothing is uploaded.

github.com's host keys come from GitHub's published list, not a first-use
scan. When git has no email yet, you pick one of your verified addresses
(the noreply one only if it's all there is). **Sign commits with SSH**
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

- machine-wide items (the System tab, k3s, Ollama), the timezone, the users
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
ssh ubuntu@web1 'sudo yardo gitkey --host gitlab.example.com' < ~/.ssh/id_ed25519
```

yardo puts it in `~/.ssh/yardo_git_key` (0600), switches the host's item
(github.com by default; GitLab for your GitLab host, Bitbucket for
bitbucket.org) to "a key you send", makes every connection to the host use
it in `~/.ssh/config`, and adds GitHub's host keys (other hosts are trusted
on first use). It then works for any repo the key may reach: a deploy key's
one repo, or all of yours for an account key. yardo saves only the hosts;
send it again to replace it. Choosing "a key you send" in `sudo yardo`
before sending one makes one on the machine, and the closing notes show its
public key and where to add it.

**Your team's own items and MCP servers.** yardo reads extra items from
`/etc/yardo/catalog.d/*.toml` (the catalog's format; see
`internal/catalog/data`) and extra MCP server specs from
`/usr/local/share/mcp-catalog/*.spec`; specs named in that directory's
`.team-files` win over yardo's own of the same name. A download in an item
must be pinned (`sha256`) or say why it can't be (`unverified = "…"`), or
yardo refuses to load it.

**Reaching this machine from a laptop.** `sudo yardo connect` (or This
VM → Connect from a laptop, on the screen) prints it all, with this machine's addresses (VPNs
first): Docker over SSH, which opens no port and needs no certificate
(`docker context create vm --docker "host=ssh://you@vm"`; bind mounts are
this machine's paths, `-p` ports open here); and the MCP gateway's URL,
how to make a laptop's token (`mcp-sync client laptop`) and the
`claude mcp add` and Codex lines that use it, or, when the gateway is for
this machine only, an SSH tunnel to it. For tools that can't use SSH,
This VM → Who can reach it → "Docker API" opens port 2376,
client certificates only, from the local network (10/8, 172.16/12,
192.168/16, Tailscale's 100.64/10) and the VPNs, or the VPNs only; never
the internet. yardo makes a CA (`/etc/docker/tls`) and Docker gets the
port from a systemd drop-in, with `--live-restore`, so its restart leaves
the containers running. `sudo yardo-docker client laptop` makes a
laptop's certificate and says how to use it; Docker can't revoke one, so
`sudo yardo-docker revoke-all` makes a new CA instead. Containers' published ports are
reachable from everywhere, as Docker does it, unless This VM → Who can
reach it → "Container ports" keeps them to the VPNs or this machine.
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
never upgrades (`mise up`, `sudo apt upgrade` do; on the screen: This VM →
Check for updates).
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
```

The bash part (`internal/assets/yardo.sh`, `picker.py`) still runs the GitHub
login and commit-signing items, and is the fallback on other architectures;
it goes once those are ported to Go (see `openspec/`).

## License

MIT
