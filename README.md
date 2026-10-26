# Haive Mind

SSH TUI Collaborative Agent App

## Overview

Haive Mind is a collaborative agent application that runs over SSH. A host connects and sends directives to an AI agent. Multiple participants connect via SSH and respond to the agent's prompts. The host can aggregate responses and send them back to the agent for further processing.

## Quick Start

```bash
# Build the server
go build -o haive-mind ./cmd/haive-mind

# Run the server (default port 2222)
./haive-mind --data-dir ./data

# In separate terminals, connect as host and participants:
ssh -p 2222 localhost
```

## Architecture

- **Server** (`cmd/haive-mind`): SSH server that manages sessions and message routing
- **Host TUI** (`cmd/host-tui`): Host interface for sending directives, viewing transcripts, aggregating responses
- **Participant TUI** (`cmd/participant-tui`): Participant interface for responding to agent prompts
- **Hub** (`hub/`): Message routing and agent communication
- **SSH Server** (`sshserver/`): SSH session management with Bubble Tea TUI integration

## End-to-End Integration Test

### Automated Tests

Run the automated integration tests:

```bash
go test -v -run TestIntegration
```

This tests:
- Full round trip: host sends directive → agent replies → participants respond → host aggregates → sends → agent replies
- Graceful shutdown saves transcript.json
- Participants see each other's responses
- Host sees agent reply and aggregate preview
- Aggregation is editable by host

### Manual Test Steps

1. **Start the server** with transcript saving:
   ```bash
   ./haive-mind --port 2222 --data-dir ./test-data --debug
   ```

2. **Connect as host** (first connection becomes host):
   ```bash
   ssh -p 2222 localhost
   ```
   - Enter display name when prompted (e.g., "Host")
   - You'll see the host TUI with transcript, participants, directive input, and buttons

3. **Connect as participant 1** (second terminal):
   ```bash
   ssh -p 2222 localhost
   ```
   - Enter display name (e.g., "Alice")
   - You'll see the participant TUI

4. **Connect as participant 2** (third terminal):
   ```bash
   ssh -p 2222 localhost
   ```
   - Enter display name (e.g., "Bob")

5. **Run a full round**:
   - On host: Type a directive in the directive pane (Tab to navigate), press Enter to send
   - Observe: Agent reply appears in transcript on all sides
   - On participants: Type responses in response pane, press Enter to submit
   - On host: Press Tab to reach buttons, select [Aggregate], press Enter
   - On host: Edit the aggregated response if needed, press [Send]
   - Observe: Agent reply to aggregate appears on all sides

6. **Verify transcript**:
   - Check `./test-data/transcript.json` exists and contains all messages
   - All three terminals should show the same message history

7. **Graceful shutdown**:
   - Press Ctrl+C in the server terminal
   - Verify `transcript.json` is saved with `EndedAt` timestamp

### Expected Behavior

- **Host sees**: Agent replies, participant responses, aggregated preview before sending
- **Participants see**: Agent replies, each other's responses in real-time
- **Aggregation**: Runs via `opencode` (mocked in tests), result editable by host before sending
- **Shutdown**: Graceful shutdown saves `transcript.json` with all messages and participants

## Development

### Running Tests

```bash
# All tests
go test ./...

# Specific package
go test ./hub/...
go test ./tui/host/...
go test ./tui/participant/...

# Integration tests
go test -v -run TestIntegration
```

### Building

```bash
go build -o haive-mind ./cmd/haive-mind
go build -o host-tui ./cmd/host-tui
go build -o participant-tui ./cmd/participant-tui
```

## Project Structure

```
.
├── cmd/
│   ├── haive-mind/      # Main SSH server entry point
│   ├── host-tui/        # Host TUI entry point
│   └── participant-tui/ # Participant TUI entry point
├── hub/                 # Message hub and agent runner
├── sshserver/           # SSH server with TUI session management
├── tui/
│   ├── host/            # Host TUI components
│   └── participant/     # Participant TUI components
├── integration_test.go  # End-to-end integration tests
└── go.mod
```

## Message Flow

```
Host                    Hub                    Participants
  |                      |                       |
  |--- Directive ------->|                       |
  |                      |--- Broadcast -------->|
  |                      |                       |
  |<-- Agent Reply ------|<-- Agent (opencode) --|
  |                      |                       |
  |<-- Broadcast --------|                       |
  |                      |                       |
  |                      |<--- Response 1 -------|
  |                      |<--- Response 2 -------|
  |                      |                       |
  |<-- Broadcast --------|--- Broadcast -------->|
  |                      |                       |
  |--- Aggregate ------->|                       |
  |                      |--- Broadcast -------->|
  |                      |                       |
  |<-- Agent Reply ------|<-- Agent (opencode) --|
  |                      |                       |
  |<-- Broadcast --------|--- Broadcast -------->|
```

## License

MIT