---
name: git-commits
description: Writes git commit messages - a short Conventional Commits subject, a body that says why, and no agent attribution. Use when committing, amending or squashing, or when asked for a commit message.
license: MIT
metadata:
  source: yardo
---

# Git commits

## The subject

- `type: what it does now`, lowercase, imperative, at most about 72
  characters, no full stop. Types: `feat`, `fix`, `docs`, `refactor`,
  `test`, `chore`, `perf`, `build`, `ci`. A scope only when the repo uses
  them: `fix(api): …`.
- Say what the change does for someone reading `git log`, not how:
  `fix: login with an older gh`, not `fix: add check for --skip-ssh-key flag`.

## The body

- Leave it out when the subject says it all.
- Otherwise a few plain sentences: why the change was needed, what it does
  now, anything surprising. Wrap at about 72 columns.
- No lists of files changed, no "This commit…", no headings, no emojis.

## Never

- Never add `Co-authored-by` for an agent (yourself included), and never
  a "Generated with …" footer. Keep a person's `Co-authored-by` when the
  user asks for it.
- Never commit secrets, generated noise, or unrelated changes: one logical
  change per commit. Stage only the files that belong to it.
- Don't commit, amend or push unless the user asked.

## Follow the repo

Read `git log --oneline -15` first. If the repo has its own style (scopes,
capitalised subjects, ticket keys like `ABC-123: …`), follow it over these
rules, but still never add an agent trailer.

## Example

Bad:

    Updated files to fix the bug with login 🎉

    - Changed auth.go
    - Changed auth_test.go

    Co-Authored-By: Claude <noreply@anthropic.com>

Good:

    fix: GitHub login with an older gh

    gh auth login --skip-ssh-key is newer than the gh Ubuntu ships, which
    stopped at "unknown flag". Pass it only when gh has it.
