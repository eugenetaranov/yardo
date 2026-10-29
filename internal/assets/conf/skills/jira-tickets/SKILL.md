---
name: jira-tickets
description: Writes Jira (or Linear, GitHub) issues, comments and updates the way a teammate would - a clear title, a few plain sentences and short acceptance criteria, never walls of text. Use when creating, editing or commenting on a ticket or issue.
license: MIT
metadata:
  source: yardo
---

# Tickets

A ticket is read by busy people between other things. Make it short
enough to read in 30 seconds, and complete enough to act on.

## Title

What needs to happen, as a short sentence a teammate would say:
`Gateway health check gives up too early on a cold VM`. No prefixes like
"[BUG]" unless the project uses them; no full stop.

## Description

- 2–5 plain sentences: what's wrong or wanted, who it affects, and why it
  matters now. Include the one detail that saves the reader a question (an
  error message, a link, where it happens).
- Then, if it helps, **Acceptance criteria**: at most 5 short bullets, each
  something you can check.
- That's all. Steps to reproduce only for a bug, as a short numbered list.

## Never

- No headings like Overview, Background, Context, Goals, Summary,
  Proposed Solution or Next Steps, no bold labels on every line, no
  emojis, no tables for two facts.
- No restating the title, no filler ("This ticket aims to…", "It is
  important to note…"), no pasted logs beyond the few lines that matter
  (attach the rest).
- Don't invent details: write what you know, and say what's unknown.
- Comments: one or two sentences with the news (what changed, what's
  blocked, what you need). Not a report.
- Don't create, edit, assign or transition tickets unless the user asked.

## Example

Bad: a "## Overview" restating the title, a "## Background" of three
paragraphs, "## Proposed Solution", "## Acceptance Criteria" with twelve
bullets and emojis.

Good:

    Gateway health check gives up too early on a cold VM

    On a fresh VM the MCP gateway takes up to a minute to start, but the
    first yardo run gives up after 10 seconds and reports the gateway as
    down. Every new VM needs a second run.

    Acceptance criteria:
    - The first run on a fresh VM passes
    - A gateway that really doesn't start still fails, within 2 minutes
