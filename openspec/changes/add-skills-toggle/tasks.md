## 1. Off and on

- [ ] 1.1 `internal/steps/skillsoff.go`: list skills in both folders and held (name, places, source, on/off), and turn one or all off or on as the user, keeping each skill's own choice under ALL. A clash leaves both folders and reports the path. Verify: `skillsoff_test.go` covers off and on in one and both places with bytes, modes and marker unchanged, ALL off and on keeping an own choice, a clash, a link refused, and a folder without SKILL.md not listed.
- [ ] 1.2 The skills step writes into the held copy for a skill that's off, and into the held place while ALL is set; uninstalling an item removes held copies. Verify: `skills_test.go` covers an update while off, a new skill while ALL is set, and uninstall removing the held copy.

## 2. Command line

- [ ] 2.1 `sudo yardo skills [on|off] [skill …]`: list, switch, print states; unknown names list the skills there are; `help.go` text. Verify: `main_test.go` cases for list, one, all, unknown.

## 3. Screen

- [ ] 3.1 Skills screen (from Go to): rows with source and state, All skills row, space marks, `a` applies in the background, leaving asks once, dimmed rows when all are off; a skill's screen with Turn it off/on; footer keys. Verify: `skills_test.go` in `cmd/yardo` with the screen model: mark then `a`, leave unsaved, all off dims rows, one-on refused while all off.
- [ ] 3.2 The home's Go to lists Skills; moving there and back doesn't re-read the status. Verify: `dashboard_test.go` case.
- [ ] 3.3 README Skills section: what off does, where held copies go, the CLI. Verify: README reads with it.

## 4. Integration

- [ ] 4.1 `task test` and `task lint` pass.
- [ ] 4.2 On a VM: turn `tdd` off, start `claude` and check it isn't listed; run `sudo yardo --yes` and check it's still off; turn it on and check it's back. Verify: output recorded in the PR.
