## REMOVED Requirements

### Requirement: GitHub access modes
**Reason**: yardo no longer asks `gh_mode` (agent / key / token); git over SSH to GitHub uses the key chosen by `yardo-git-ssh`'s "Adding a git host asks for its key", as for GitLab and Bitbucket, and logging gh in is the user's (`gh auth login`).
**Migration**: Saved answers keep working: a GitHub host set up in any earlier mode keeps its key (or none), per `yardo-git-ssh`'s "Machines set up before keep their keys". A saved `token` mode is read as No key here.
