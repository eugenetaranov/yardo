# This machine

- A development machine set up by yardo, usually a disposable VM. Breaking it is cheap;
  it can be recreated.
- The user has passwordless sudo.
- Developer tools are installed per user with mise (`mise ls`, `mise use -g
  <tool>`); system packages with apt.
- `~/yardo.txt` lists what yardo installed and how;
  `sudo yardo` adds more.
- Kubernetes config, if any, is in `~/.kube/config`.
