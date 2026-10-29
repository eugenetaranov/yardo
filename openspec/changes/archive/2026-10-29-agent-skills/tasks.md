## 1. No attribution

- [x] 1.1 `go:agentsafe`: merge `"attribution": {"commit": "", "pr": ""}` into `~/.claude/settings.json`; test that other keys stay
- [x] 1.2 `agent-notes.md`: "Never add Co-authored-by or Generated-with lines for an agent"
- [x] 1.3 `go:gitattrib`: on Git ≥ 2.54, `git config --global hook.yardo-no-agent-trailer.event commit-msg` + `.command`; a script that strips only agent trailer lines; tests with the agent and a person's trailer, and idempotence
- [x] 1.4 Older git: on Ubuntu, git from the git-core PPA first; else no hook, a note in `~/yardo.txt`; Codex installed: a note on the account setting

## 2. yardo's skills

- [x] 2.1 Write `git-commits`, `pr-descriptions`, `jira-tickets` (portable frontmatter, < 100 lines, examples of good and bad)
- [x] 2.2 A step that writes skills to `~/.agents/skills` and `~/.claude/skills` with a `.yardo-skill` marker; updates only matching folders (unticked: stays, as every item)
- [x] 2.3 Catalog item "Agent skills" (default on, Agents tab)

## 3. Community skills

- [x] 3.1 `internal/assets/conf/skills/` + `internal/assets/skills.lock.toml`; vendor the opt-in skills with their LICENSE
- [x] 3.2 `scripts/update-skills [--latest] [NAME...]`: fetch at the lock's commit, licence and size checks, rewrite the lock's commit and sha256
- [x] 3.3 One catalog item, "Community skills" (off by default), sources and licences in its details

## 4. Tests and docs

- [x] 4.1 Skill format test (name/folder, description length, frontmatter keys)
- [x] 4.1b Lock hash and licence test (with the vendored skills, the embedded copy too)
- [x] 4.2 README: what the skills are, where they go, how to turn attribution off in Codex
- [ ] 4.3 Try on a VM: a Claude Code commit and a Codex commit carry no agent trailer; `/skills` (or the agent's list) shows the skills
