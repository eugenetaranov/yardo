## ADDED Requirements

### Requirement: A session on ssh login
On an interactive ssh login, yardo's zsh setup SHALL attach to the user's running tmux or zellij session, or start one when none is running. It SHALL do nothing when the shell is already inside tmux or zellij, when the login isn't interactive (a command, scp, rsync, git), on the console, in an editor's remote terminal, or when the setting is off. When both tmux and zellij have a running session, it SHALL attach to the one the setting names, and with no setting, ask.

#### Scenario: Reconnect
- **WHEN** the user's ssh drops while a tmux session is running, and they ssh in again
- **THEN** they are back in that tmux session, with what was running in it

#### Scenario: Nothing running
- **WHEN** the user sshes in, the setting is zellij, and no session is running
- **THEN** a zellij session starts

#### Scenario: Inside a session
- **WHEN** a new pane opens inside tmux
- **THEN** its shell starts neither tmux nor zellij

#### Scenario: A command over ssh
- **WHEN** `ssh vm uptime` runs
- **THEN** it prints the uptime and exits, with no session started

#### Scenario: Editor terminal
- **WHEN** VS Code Remote-SSH opens a terminal on the machine
- **THEN** it is a plain shell

### Requirement: Which one is asked once
The first time there is no session to attach to, both tmux and zellij are installed and nothing is set, the shell SHALL ask in one line which to use: tmux, zellij, or never, answered by one key, and SHALL remember the answer for later logins. With only one of them installed it SHALL use that one without asking. With neither installed it SHALL do nothing.

#### Scenario: First login
- **WHEN** the user sshes in for the first time, with both installed and nothing set, and presses `z`
- **THEN** zellij starts, and later logins use zellij without asking

#### Scenario: Never
- **WHEN** the user answers never
- **THEN** this login and later ones get the plain shell, until the setting is changed

#### Scenario: Only tmux
- **WHEN** only tmux is installed and nothing is set
- **THEN** tmux starts, with no question

### Requirement: The shell is never lost to it
Detaching from the session, or ending it, SHALL leave the user in the plain shell, still connected. When tmux or zellij fails to start or attach, the user SHALL get the plain shell and a line saying what failed.

#### Scenario: Detach
- **WHEN** the user detaches from tmux
- **THEN** they are at the machine's zsh prompt, and the session keeps running

#### Scenario: zellij broken
- **WHEN** zellij exits with an error at login
- **THEN** the shell says zellij didn't start and gives the prompt

### Requirement: Session on login is a setting
Settings SHALL have "Session on login" with tmux, zellij, ask and off, showing the answer the login question saved, and changing it SHALL take effect at the next login. A profile's `sessions` (`tmux`, `zellij` or `off`) SHALL set it.

#### Scenario: Turn it off
- **WHEN** the user sets Session on login to off and applies
- **THEN** the next ssh login is a plain shell

#### Scenario: The question's answer shows
- **WHEN** the user answered zellij at login, then opens Settings
- **THEN** Session on login reads zellij
