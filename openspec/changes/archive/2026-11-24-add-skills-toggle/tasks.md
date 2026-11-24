## 1. Off and on

- [x] 1.1 `internal/steps/skillsoff.go`: list skills in both folders and held (name, places, source, on/off), and turn one or all off or on as the user, keeping each skill's own choice under ALL. A clash leaves both folders and reports the path. Verify: `skillsoff_test.go` covers off and on in one and both places with bytes, modes and marker unchanged, ALL off and on keeping an own choice, a clash, a link refused, and a folder without SKILL.md not listed.
- [x] 1.2 The skills step writes into the held copy for a skill that's off, and into the held place while ALL is set; uninstalling an item removes held copies. Verify: `skills_test.go` covers an update while off, a new skill while ALL is set, and uninstall removing the held copy.

## 2. Command line

- [x] 2.1 `sudo yardo skills [on|off] [skill …]`: list, switch, print states; unknown names list the skills there are; `help.go` text. Verify: `cmd/yardo/skills_test.go` cases for list, one, all, unknown.

## 3. Screen

- [x] 3.1 Skills screen (from Go to): rows with source and state, All skills row, space marks, `a` applies in the background, leaving asks once, dimmed rows when all are off; a skill's screen with Turn it off/on; footer keys. Verify: `internal/ui/skillsmenu_test.go` with the screen model: mark then `a`, leave unsaved, all off dims rows, one-on refused while all off.
- [x] 3.2 The home's Go to lists Skills; moving there and back doesn't re-read the status. Verify: `TestMenuSkills` (the tab, enter, q back to it; the menu reads nothing).
- [x] 3.3 README Skills section: what off does, where held copies go, the CLI. Verify: README reads with it.

## 4. Integration

- [x] 4.1 `task test` and `task lint` pass.
- [x] 4.2 On a VM: turn `tdd` off, check neither skills folder has it (Claude Code isn't logged in on the test VM, so its list wasn't read); run `sudo yardo --yes` and check it's still off; turn it on and check it's back. Verify: done on a throwaway VM: off, an apply (`tdd` still held), all off and on (`tdd` stays off), on (both copies back, the marker kept).
