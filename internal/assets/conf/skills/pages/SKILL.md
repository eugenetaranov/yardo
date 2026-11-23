---
name: pages
description: Shows the user an HTML page made on this machine (a diagram, a report, an archify explainer) by saving it in ~/pages, which yardo serves, and giving them the link from pages-url. Use whenever you produce an HTML file for the user to look at.
license: MIT
metadata:
  source: yardo
---

# Pages the user can open

This machine is remote: the user can't open a file path here in their
browser. yardo serves `~/pages` over HTTP for them.

1. Save the page in `~/pages`, under a short, descriptive name:
   `~/pages/checkout-architecture.html` (a subfolder per project is fine:
   `~/pages/myrepo/flow.html`). With archify, deliver to that path.
2. Run `pages-url <name>` (the path inside `~/pages`, e.g.
   `pages-url myrepo/flow.html`). It prints the link as the user's laptop
   reaches this machine, and any note on stderr (a password, an SSH tunnel
   to open first).
3. End your answer with that link, and the note if there was one.

The page must be self-contained (inline CSS and JS, or files next to it in
`~/pages`): nothing else on this machine is served. `pages-url` with no
argument links the list of pages.

If `pages-url` isn't there, ~/pages isn't served yet: say so and give the
file's path (`sudo yardo`, Agents, adds Web pages).
