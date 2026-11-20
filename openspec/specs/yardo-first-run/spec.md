# yardo-first-run Specification

## Purpose
What a first `sudo yardo` run asks, in what order, what its review shows, and what Install, Customize… and Quit do, so that saying Install starts the run.

## Requirements

### Requirement: Questions come before the review
A first run SHALL ask in this order: the Choices screen (the prompt and the catalog's choices, one screen); then every question the chosen items need (git name and email, timezone, where the gateway listens and its domain, Docker's published ports, wiring the agents, the dotfiles repo, API keys, a key per git host); then the review. No question SHALL be asked after the review's answer.

#### Scenario: Install starts the run
- **WHEN** the user ticks the defaults on Choices, answers the questions, and picks Install on the review
- **THEN** the run starts at once, with no further prompt before its first step

#### Scenario: Back from a question
- **WHEN** the user presses shift+tab on the first question after Choices
- **THEN** the Choices screen opens again with the ticks kept

### Requirement: The review shows the answers
The review SHALL list what the run installs and configures, what every machine always gets, and every answer the questions took, each on a line of its own: the user, the prompt, the git name and email, the timezone, the gateway's address and port, the Docker ports' reach, the dotfiles repo, which git hosts get a key, and which API keys were given (the names, never the values).

#### Scenario: Git shown
- **WHEN** the user typed the git name Ada Lovelace and the email ada@example.com
- **THEN** the review has a line `Git: Ada Lovelace <ada@example.com>` before "Ready?"

#### Scenario: Keys named, not shown
- **WHEN** the user gave an Anthropic API key
- **THEN** the review says the Anthropic key was given and does not print it

### Requirement: Customize… keeps the answers
Customize… on the review SHALL open the Tools board with the choices queued. `a` on the board SHALL ask only what the items added there need beyond the answers already given, then show the review again, and Install SHALL start the run. Nothing SHALL be asked twice.

#### Scenario: Add Java on the board
- **WHEN** the user picks Customize…, ticks Java, presses `a`, and picks Install
- **THEN** no question from the first review is repeated and the run includes Java

### Requirement: Quit saves nothing
Quit on the review, or `q` on Choices or on a question, SHALL end the run with nothing installed and no answers saved; the next `sudo yardo` SHALL be a first run again.

#### Scenario: Quit at the review
- **WHEN** the user answers every question and picks Quit
- **THEN** yardo exits 0, no state file exists for the user, and `sudo yardo` opens Choices again
