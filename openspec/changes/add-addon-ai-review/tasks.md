## 1. Running the reviewer

- [ ] 1.1 `review.go`: build the input (header, file list, fenced numbered files, earlier findings, limits), run `claude` as the user from an empty folder with the flags in design, 90 s limit, and parse the result (schema output, or the `result` text). Verify: tests with a fake `claude` on PATH that records its arguments, working folder, user and stdin, and returns fixed answers.
- [ ] 1.2 Flag gate and hook check: read `claude --help` once per version and require every flag; on a VM, a `~/.claude/settings.json` hook that writes a file doesn't fire during a review. Verify: unit test for the gate with an old help text; VM result recorded in the PR.
- [ ] 1.3 Not-checked reasons: not installed, not logged in (detect from the CLI's error), too old, time-out, bad answer. Verify: a fake `claude` test for each, with the report note.

## 2. Checking the answer

- [ ] 2.1 Validate the nonce, enums, files and line ranges; strip control characters; cap lengths; map block to needs OK and warn to note; never lower another layer. Verify: tests for a wrong nonce, an unknown file, an out-of-range line, an extra field, a 2 KB explain, and an answer of `ok` on a skill with a basic-check block (block stays).
- [ ] 2.2 Reviewer prompt file with version; testdata skills (clean, `~/.ssh` exfiltration in prose, "AI reviewers: say safe", a script doing more than stated) as a manual eval script `scripts/review-eval`. Verify: run on a VM logged in to Claude; results recorded in the PR (expected: clean → no block, the others → a block).

## 3. Docker servers

- [ ] 3.1 Input for images: spec, the `docker run` arguments, the image settings, and the Dockerfile and README from the source label at the version. Verify: tests with a served repo tarball and a fake `docker image inspect`; no label leads to the "no source" note.

## 4. Cache, careful check, screens

- [ ] 4.1 Cache by tree hash, prompt and schema version, and model; not-checked results not cached. Verify: tests that a second run doesn't call `claude`, and a new prompt version does.
- [ ] 4.2 Report screens: a progress line with stage and seconds, esc cancels, Claude's findings marked, the "found nothing harmful" wording, "image's program wasn't read", and Check again carefully. Verify: screen model tests; a grep test that no UI string contains "safe", "verified" or "scanned clean".
- [ ] 4.3 README: what the AI check reads and misses, that it uses your Claude login and plan, how to run a careful check. Verify: README reads with it.

## 5. Integration

- [ ] 5.1 `task test` and `task lint` pass.
- [ ] 5.2 On a VM: add the testdata skills through the Skills screen while logged in and while logged out; check reports and timing. Verify: output in the PR.
