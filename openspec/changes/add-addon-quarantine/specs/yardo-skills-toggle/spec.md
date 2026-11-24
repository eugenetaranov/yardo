## ADDED Requirements

### Requirement: Adding skills from the Skills screen
`n` on the Skills screen SHALL ask for one or more GitHub sources (`owner/repo[/path][@ref]`). When a source holds several skills, yardo SHALL list them to pick from. Each picked skill SHALL go through checking before it is installed, with its report shown on the screen.

#### Scenario: A repo with several skills
- **WHEN** the user presses `n` and enters `acme/skills`, which has `a`, `b` and `c`
- **THEN** yardo lists the three to pick, and checks each one picked

### Requirement: The Skills screen shows what was checked
Each skill's row SHALL say whether it's yardo's, checked (with the date it was approved), held (waiting for a decision), or not checked. A skill that isn't yardo's and isn't checked SHALL offer **Check it** on its screen.

#### Scenario: Not checked
- **WHEN** `extra` was installed by another tool
- **THEN** its row reads not checked, and its screen offers Check it
