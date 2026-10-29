## 1. No attribution

- [ ] 1.1 `go:agentsafe`: merge `"attribution": {"commit": "", "pr": ""}` into `~/.claude/settings.json`; test that other keys stay
- [ ] 1.2 `agent-notes.md`: "Never add Co-authored-by or Generated-with lines for an agent"
- [ ] 1.3 `go:gitattrib`: on Git ≥ 2.54, `git config --global hook.yardo-no-agent-trailer.event commit-msg` + `.command`; a script that strips only agent trailer lines; tests with the agent and a person's trailer, and idempotence
- [ ] 1.4 Older git: no hook, a note in `~/yardo.txt`; Codex installed: a note on the account setting

## 2. yardo's skills

- [ ] 2.1 Write `git-commits`, `pr-descriptions`, `jira-tickets` (portable frontmatter, < 100 lines, examples of good and bad)
- [ ] 2.2 A step that writes skills to `~/.agents/skills` and `~/.claude/skills` with a `.yardo-skill` marker; updates and removes only matching folders
- [ ] 2.3 Catalog item "Agent skills" (default on, Agents tab)

## 3. Community skills

- [ ] 3.1 `internal/assets/skills/` + `skills.lock.toml`; vendor the opt-in sets with their LICENSE/NOTICE
- [ ] 3.2 `scripts/update-skills <name> <sha>`: fetch, diff, licence and size checks, lint
- [ ] 3.3 A catalog item per set (off by default), its source and licence in the label's details

## 4. Tests and docs

- [ ] 4.1 Skill format test (name/folder, description length, frontmatter keys); lock hash and licence test
- [ ] 4.2 README: what the skills are, where they go, how to turn attribution off in Codex
- [ ] 4.3 Try on a VM: a Claude Code commit and a Codex commit carry no agent trailer; `/skills` (or the agent's list) shows the skills
