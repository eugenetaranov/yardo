## MODIFIED Requirements

### Requirement: One program, one screen
The home, Tools, Settings (Network access, Look), Fix, Fill in, Updates, Connection, the MCP servers screens and the Skills screens SHALL be views of one program in one alternate screen. Moving between them SHALL NOT leave and re-enter the alternate screen, and SHALL NOT read the machine's status again. Only a prompt that asks a question and a run SHALL leave the screen.

#### Scenario: Home to Tools and back
- **WHEN** the user presses enter on Tools, then `q`
- **THEN** the home shows again without "Checking what's set up" and without the terminal's own screen showing in between

#### Scenario: Skills from the home
- **WHEN** the user moves to Skills in Go to and presses enter, then `q`
- **THEN** the Skills screen opens and the home comes back, without the screen flashing or the status being read again
