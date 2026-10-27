# yardo

**Set up a Linux dev machine for you and your AI agents.** Run `sudo yardo`
on a fresh machine (a VM, a cloud server, a box under your desk), tick what
you want in a tabbed picker, and yardo installs and configures it: dev tools
and languages, cloud, IaC and Kubernetes CLIs, Docker, coding agents (Claude
Code, Codex, Gemini, OpenCode, Goose, …) wired to an MCP gateway, system
hardening, SSH and git keys, your dotfiles. It remembers what you picked and
re-applies it idempotently. Teams share TOML profiles and roll them across
many machines; `yardo verify` reports drift.

yardo is a single static Go binary for Linux (amd64, arm64) with everything
it needs embedded. It needs root (it configures the OS) and Ubuntu or Debian.

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
account is created) and opens the picker, on two screens: first **what to
install**, then **how to configure it**. On each, categories are tabs across
the top (←/→ or 1–9 to switch) above a checklist; space ticks an item.
The first run starts with the usual stack ticked: zsh, Neovim, tmux,
lazygit, the linters, the dev CLIs, the admin and debug tools, Docker and
its tools, Claude Code with OpenSpec, git (name and email, defaults,
GitHub) and the two agent items. Untick what you don't want, or ctrl-x to
untick everything. Later runs start empty: what you picked before stays
installed. Each row is one line; the line under the list shows
how the item under the cursor is installed or applied, e.g. `mise use -g
terraform`.

- **[1/2] Install:** System, Shell & CLI, Dev tools, Languages, Cloud, IaC,
  Containers & Kubernetes, AI. Enter on the last tab goes on to Configure.
- **[2/2] Configure:** System settings, Git & SSH, Agents, Logins & keys, then
  Review, which lists what you ticked on this screen under a line naming what
  you're installing. Esc goes back to Install, with your ticks kept. This
  screen offers only what fits what's installed: registry logins once Docker
  is picked (now, on an earlier run, or already on the machine), the npm
  registry with Node, the Tailscale login with Tailscale, and so on.

The run follows the same order: everything is installed first (the extra
mise tools at the end of that), then configured, with your dotfiles last.
The CA bundle is the exception: it's applied before anything is
downloaded.

yardo is opinionated: what every machine should have isn't offered, it's
always applied (see "Always" below), and related tools come as one item.

Just start typing to fuzzy-search every tab of the screen at once: `ssh`
lists what mentions SSH wherever it lives, each with its category. On the
Install screen the search also covers the ~940 tools in mise's registry,
with their descriptions (`find` turns up `fd`). A tool you tick there is
installed with `mise use -g <tool>` (to pin a version or use a
`backend:tool`, list it under `mise_extra` in a profile). Esc clears the
search; Esc on an empty search quits (on Configure: goes back).

**Always**, on every machine, with nothing to pick. Installing: essentials
(git, curl, jq, gh, build tools). Configuring: automatic security updates
(no auto-reboot) with needrestart restarting services quietly; SSH keys only, no root login
(skipped, with a note, while you have no authorized key; a config sshd
rejects is rolled back); fail2ban for SSH, on a machine with a public IPv4
on an interface (Tailscale addresses are never banned; a cloud VM behind
1:1 NAT only sees its private address, so it gets none); dev limits
(inotify, open files), zram swap, a journal size cap, timezone (asked once)
+ locale. Keeping the clock in sync is left to the machine's launcher.

### [1/2] Install

1. **System (root):** etckeeper, admin and debug tools (dig, mtr, tcpdump,
   iperf3, nmap, strace, htop, btop, ncdu, …), Tailscale (log in under Configure), NFS/SMB
   clients, node_exporter + glances.
2. **Shell and CLI:** one zsh item that brings everything: zsh as the login
   shell, oh-my-zsh with autosuggestions, syntax highlighting, fzf history
   search and completions, fzf-tab (Tab opens an fzf list with directory
   previews), the modern CLI tools (ripgrep, fd, bat, eza, zoxide, fzf,
   delta, direnv) turned on with keys and aliases (z, fzf keys, direnv,
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
3. **Dev tools:** lazygit, dev CLIs (just, Task, watchexec, hyperfine, yq,
   xh, hurl), linters and hooks (shellcheck, shfmt, actionlint, gitleaks,
   pre-commit), database clients (psql, pgcli, usql, mariadb, sqlite3,
   redis-cli), perf/valgrind/bpftrace, act, glab (GitLab's CLI).
4. **Languages** (mise, except PHP and C/C++ from apt): Go + gopls, delve,
   govulncheck, golangci-lint; Python 3.13 + uv + ruff; Node + pnpm +
   TypeScript; Bun; Deno; Java 25 + Maven + Gradle + Kotlin; Rust +
   rust-analyzer + cargo-binstall; Ruby; PHP + Composer; C/C++; Zig; .NET.
5. **Cloud** (mise): AWS, Google Cloud, Azure, DigitalOcean, Hetzner,
   cloudflared, restic + rclone.
6. **IaC** (mise): Terraform with terragrunt, tflint and terraform-docs,
   OpenTofu, trivy + checkov, Packer, Ansible, sops + age, Vault.
7. **Containers & Kubernetes:** Docker (with log rotation and
   10.200.0.0/16 networks), Podman + skopeo + buildah, container tools
   (lazydocker, dive, hadolint, crane, cosign); the Kubernetes CLIs as
   one item (kubectl, kubectx/kubens, Helm 3, k9s, kubelogin, stern,
   kustomize, kubecolor, kubie, kubeconform, helmfile + helm-diff, krew with
   plugins); Argo CD and Flux CLIs; kind, k3d, or k3s as a real cluster on
   the VM; Tilt, Skaffold, ctlptl; talosctl.
8. **AI:** Claude Code, Codex CLI, Gemini CLI, OpenCode, Aider, Goose, omp,
   Ollama, OpenSpec, ECC for Claude Code (agents, rules and commands, no
   hooks), and the MCPJungle gateway (its servers: Configure → Agents).

### [2/2] Configure

1. **System settings:** "sudo asks for your password", weekly tool updates
   (a systemd timer running `mise up` and `krew upgrade` as you). Behind a
   corporate network: an internal CA bundle (a PEM file on the VM, or an
   https URL with its sha256), trusted system-wide and by Node, Python and
   the AWS CLI. It's applied before anything is installed, and can be set
   in a profile's `[system]` (`ca_bundle`, `ca_sha256`).
2. **Git & SSH:** git identity, git defaults (rebase on pull, push
   sets upstream, delta diffs, aliases, a global ignore file), one item per
   git host (GitHub with the gh login, GitLab with gitlab.com or your own
   host, Bitbucket), each asking how this machine reaches it over
   SSH (a forwarded agent, a key made here, or a key you send; GitHub also
   a token), commit signing with SSH, global git ignores in
   `~/.config/git/ignore` (`.claude/`, `.python-version`, `.env`, `.venv/`,
   `node_modules/`, caches, editor files; yardo keeps them as a block at the
   top and leaves your own lines below it alone), and your dotfiles repo:
   cloned to `~/.dotfiles` (pulled on later runs), then its `install.sh`,
   `bootstrap.sh` or `setup.sh`, or else `stow` for each top-level
   directory. It runs after everything else, so your files win (`[dotfiles]
   repo = "…"` in a personal profile).
3. **Agents:** a `CLAUDE.md` / `AGENTS.md` describing the VM, settings for
   agents left to run unattended (Claude Code may not force-push, sudo, or
   read `~/.mcp`, `~/.ssh` or the agent keys; Codex asks first and works in
   a sandbox; both are told never to rewrite pushed history; merged
   into your settings, not replacing them), agent API keys (Anthropic,
   OpenAI, Gemini), and MCP servers on the MCPJungle gateway (GitHub,
   Linear, Postgres, Playwright, Jira, AWS, Jenkins, context7, …), added to
   Claude Code, Codex, Gemini CLI, OpenCode and Goose. The gateway listens
   on this machine only, on the VM's Tailscale address only (when Tailscale
   is up), or on every interface (no auth: trusted networks only).
4. **Logins & keys:** the Tailscale login (an auth key, or `sudo tailscale
   up` later), container registry logins (`docker login`; ghcr.io can use
   your gh token), a private npm registry (scope and token in `~/.npmrc`),
   and your age key for sops (generate or paste).

In a profile, a split item's old id means both halves: `tailscale`,
`sops` and `mcpjungle` are the configure items, and each pulls in its
install item (`tailscale-install`, `sops-install`, `mcpjungle-install`),
which a profile can also list alone. `gitlab` no longer installs glab:
list `gitlab-cli` for that.

Downloads are pinned: vendor apt keys are checked against their
fingerprints and release binaries against their sha256 before they're
installed. The few vendor installers that change in place (Tailscale,
Ollama, k3s, krew, omp, Neovim's plugins) can't be pinned; `~/yardo.txt`
lists the installed ones under "Installed without a pinned checksum", with
why. A `catalog.d` item that downloads must do the same (`sha256`, or
`unverified = "why"`), or yardo refuses to load it.

After the two screens, it asks every typed question in one place: git name
and email, timezone, how to reach GitHub, and the credentials of the items
you picked. Then a review screen offers Install,
Start over or Quit. `~/yardo.txt` lists everything installed so far,
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
`docker login ghcr.io`, …). The run ends with how many are missing.

| Item | File |
|---|---|
| Agent API keys | `~/.config/yardo/agent.env`, exported by interactive shells only (not services or cron) |
| MCP servers | `~/.mcp/<name>/env`, owned by the gateway's user: `mcp-sync edit <name>`, then `mcp-sync` |
| Private npm registry | a yardo block in `~/.npmrc` (your own lines are kept) |
| Registry logins | `~/.docker/config.json`, through `docker login` |
| age key | `~/.config/sops/age/keys.txt`, with `SOPS_AGE_KEY_FILE` set |

Each user's MCPJungle gateway runs as a system user of its own,
`mcp-<you>`, which owns `~/.mcp`: a coding agent running as you can use the
MCP servers through the gateway but can't read their tokens. `mcp-sync` and
`mcp-sync edit <name>` switch to that user through sudo for you. This only
holds while the agent can't get root: with passwordless sudo (the default
for the VM's user) or membership of the docker group, it can. The opt-in
"sudo asks for your password" item (System) removes `NOPASSWD` from your
sudo rules, after setting a password if you have none; tools that run sudo
on the machine over SSH then need that password too.

`sudo yardo --yes` never waits for input: a step that would (the
GitHub browser login, adding a gh scope, `tailscale up` without a key)
leaves a note naming the command to run instead.

**GitHub** asks how the VM should reach it:

- **Forwarded agent** (the default when you connected with
  `ssh -A`): no key on the machine. git over SSH uses your laptop's
  agent, and gh logs in with the browser code, without permission to add
  keys to your account.
- **A key on this VM**: `~/.ssh/id_ed25519` (optionally with a passphrase)
  uploaded to your account. For an organization with SAML SSO, authorize it
  at github.com/settings/keys → Configure SSO.
- **A fine-grained token**: pasted once, used by gh and by git over HTTPS;
  nothing is uploaded.

github.com's host keys come from GitHub's published list, not a first-use
scan. When git has no email yet, you pick one of your verified addresses
(the noreply one only if it's all there is). **Sign commits with SSH**
signs commits and tags with the agent's key or the VM's key, uploads it as
a signing key, and writes `~/.config/git/allowed_signers`.

**`sudo yardo forget`** undoes what yardo created, and only
that: the GitHub keys it uploaded, its registry logins, the values in the
credential files it wrote (the keys stay, so a re-run asks again), its
`~/.npmrc` block, and the gh and Tailscale logins it made. It lists them and
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

**Profiles.** `sudo yardo export > team.toml` writes the choices on
a VM as a profile: a TOML file of picks and plain settings (git identity,
timezone, MCP servers), never secrets. `sudo yardo --profile team.toml
--yes` applies one (`--profile` repeats, later files win, so a team file and
a personal one combine); a profile can also be an https URL. Without `--yes`, `--profile` shows the plan for review and asks for the
secrets it can't hold. A profile with `name = "team"` is a layer: applying
a newer `team` profile replaces the picks the old one made, and lists the
ones it dropped (they stay installed, but aren't re-applied). Picks you
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
