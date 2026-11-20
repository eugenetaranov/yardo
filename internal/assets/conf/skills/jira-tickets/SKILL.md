---
name: jira-tickets
description: Writes Jira (or Linear, GitHub) issues, comments and updates the way a teammate would - a short title, two or three plain sentences and checkable acceptance criteria, nothing more. Use when creating, editing or commenting on a ticket or issue.
license: MIT
metadata:
  source: yardo
---

# Tickets

Nobody reads a long ticket. Readable in 20 seconds, under ~100 words.

## Shape

    <Title: what needs to happen, a sentence a teammate would say, no full stop>

    <1–3 plain sentences: what's wrong or wanted, and why now. The one
    detail that saves a question: the error, a link, where it happens.>

    Acceptance criteria:
    - <3–5 bullets, each one observable and pass/fail>

- **Acceptance criteria are required** on every story, task and bug. Each
  is a result someone can check ("A fresh VM passes the first run"), not
  a step of the work ("Increase the timeout") and not vague ("Works
  better").
- A bug adds **Steps to reproduce**, numbered, 3 lines at most.
- Nothing else. No Overview, Background, Context, Goals, Proposed
  solution or Next steps; no bold labels, emojis or tables; no restating
  the title; no filler ("This ticket aims to…").
- Don't invent details: say what's unknown in one line.
- Comments: one or two sentences with the news (what changed, what's
  blocked, what you need).
- Create, edit, assign or transition a ticket only when asked.

## Example

    Gateway health check gives up too early on a cold VM

    On a fresh VM the MCP gateway takes up to a minute to start, but the
    first run gives up after 10 seconds, so every new VM needs a second run.

    Acceptance criteria:
    - The first run on a fresh VM passes
    - A gateway that really doesn't start still fails, within 2 minutes
