---
name: pr-descriptions
description: Writes pull request titles and descriptions as a short note to the reviewer - what changed, why, and how it was tested - with no agent footer. Use when opening or updating a pull request or merge request.
license: MIT
metadata:
  source: yardo
---

# Pull request descriptions

Write for the person who will review it, in a few minutes, not for a
report.

## Title

Like a good commit subject: what the change does, in under about 70
characters. Follow the repo's convention (`feat: …`, a ticket key).

## Body

At most three short parts, each a few sentences or bullets:

1. **What and why.** The problem, and what this changes. Link the issue.
2. **How to review.** Only if it helps: where to start, what's risky,
   what was deliberately left out.
3. **Testing.** What you ran and saw (`go test ./...`, tried it on a VM).
   Say plainly what wasn't tested.

## Never

- Never add a "Generated with …" footer or an agent `Co-authored-by`.
- No file-by-file walkthrough: the diff shows that.
- No "Summary/Overview/Changes/Conclusion" headings on a small change,
  no emojis, no marketing ("robust", "seamless", "comprehensive").
- Don't claim tests passed that you didn't run.
- Don't open, update or merge a pull request unless the user asked.

## Example

    Retry the gateway health check before giving up

    The gateway takes up to a minute to start on a cold VM, and yardo gave
    up after 10 seconds, so the first run often failed. It now waits up to
    60 seconds, checking every second.

    Tested: a fresh VM with `task push`; the first run passed.
    Not tested: arm64.
