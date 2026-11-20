---
name: git-branches
description: Names git branches - the project's own convention when it has one (a prefix like tech/ from git config yardo.branchPrefix, the repo's docs or its recent branches), else the Jira key first (PROJ-123-short-description). Use when creating or renaming a branch.
license: MIT
metadata:
  source: yardo
---

# Git branches

    [prefix]<KEY>-<short-description>

## The project's convention first

Use the first of these that gives one:

1. The repo says how: AGENTS.md, CLAUDE.md, CONTRIBUTING.md or the PR
   template. Follow it exactly.
2. `git config --get yardo.branchPrefix` (set per repo, or `--global`):
   put it in front as it is, e.g. `tech/` → `tech/DEVOPS-142-cold-start-timeout`.
3. The recent branches (`git branch -r --sort=-committerdate | head -20`)
   mostly share a prefix (`tech/`, `feature/`, `fix/`) or a case: do the
   same. When they disagree, use no prefix.

When you followed 2 or 3, say which in one line. To keep a prefix for a
project, the user runs `git config yardo.branchPrefix tech/` there.

## The name

- With a ticket: its key exactly as Jira shows it, then a hyphen:
  `DEVOPS-142-gateway-cold-start-timeout`. Find the key in what the user
  gave you (a URL, the ticket's text); don't invent one.
- Without a ticket: the description alone: `gateway-cold-start-timeout`.
  Ask for the key if the user's team works from tickets and none was given.
- The description: 2–5 lowercase words, joined with hyphens, saying what
  changes, from the ticket's title. Only `a-z`, `0-9` and `-`; at most
  about 50 characters with the key.
- No dates, no `wip`, no your name, and no prefix the project doesn't use.
- Create or rename a branch only when asked; never rename one that's
  pushed without saying so.

Examples:

    https://acme.atlassian.net/browse/DEVOPS-142 "Gateway health check gives up too early on a cold VM"
      → DEVOPS-142-gateway-cold-start-timeout
      → tech/DEVOPS-142-gateway-cold-start-timeout   (yardo.branchPrefix = tech/)
    no ticket, "fix the login with older gh"
      → login-older-gh
