## ADDED Requirements

### Requirement: Servers from elsewhere
The catalog that `n` opens on MCP servers SHALL end with **From elsewhere**. It SHALL take a Docker image, an https URL or a spec file. For an image or URL, it SHALL ask for the server's name, description, the settings it needs, and whether it needs the network. The server SHALL then go through checking, and its row SHALL read held until the user decides. Approved servers SHALL be listed as checked, with their pin.

#### Scenario: An image
- **WHEN** the user picks From elsewhere, enters `ghcr.io/acme/mcp-x:1.2`, and fills in the form
- **THEN** the image is pulled and checked, the report is shown, and the server isn't registered until it's installed

#### Scenario: Held
- **WHEN** a server from elsewhere is waiting for a decision
- **THEN** its row reads held, and its tools aren't listed to any client
