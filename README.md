# Haive Mind

SSH TUI Collaborative Agent App

## Overview

Haive Mind is a collaborative agent application that runs over SSH. A host connects and sends directives to an AI agent. Multiple participants connect via SSH and respond to the agent's prompts. The host can aggregate responses and send them back to the agent for further processing.

## Install

Requires [Go](https://go.dev/dl/) 1.26 or newer.

```bash
go install github.com/jakofranko/haive-mind/cmd/haive-mind@latest
```

This puts a `haive-mind` binary in `$(go env GOPATH)/bin` (usually `~/go/bin`); make sure that directory is on your `PATH`.

The server calls the agent by running [`opencode`](https://opencode.ai), so `opencode` must be installed and on the `PATH` of the machine running `haive-mind`. The host and participants need only an `ssh` client.

## Quick Start

```bash
# Run the server (default port 2222)
haive-mind

# Accept local connections only (no firewall prompt on Windows)
haive-mind --bind 127.0.0.1

# In separate terminals, connect as host and participants:
ssh -p 2222 localhost
```

Run `haive-mind --help` for all flags (`--port`, `--bind`, `--host-key`, `--data-dir`, `--debug`, `--log-file`, `--agent-timeout`).

### Where files are kept

Nothing is written to the directory you start the server from. Unless you pass a flag, the server uses these locations, each under a `haive-mind` directory:

| What | Flag | Linux | Windows | macOS |
| --- | --- | --- | --- | --- |
| Saved transcripts | `--data-dir` | `~/.local/share` | `%LocalAppData%` | `~/Library/Application Support` |
| SSH host key (`id_ed25519`, generated on first run) | `--host-key` | same as transcripts | same as transcripts | same as transcripts |
| Log file (`haive-mind.log`, JSON) | `--log-file` | `~/.local/state` | `%LocalAppData%` | `~/Library/Logs` |

On every platform, an absolute `XDG_DATA_HOME` or `XDG_STATE_HOME` replaces the base directory for data or logs. Each flag moves only its own item. Transcripts are always saved: on shutdown, and on each export from the host.

Logs go to the log file; they are also printed to the terminal when the server is run from one. If you pass `--log-file`, logs are printed as well as written.

The server now keeps the host key in the data directory instead of the current directory. If you ran an earlier version, an `id_ed25519` in the directory you started it from is no longer used: clients will see a new host key, and you can pass `--host-key ./id_ed25519` to keep the old one.

**Who is the host?** The first SSH connection becomes the host, whatever its SSH username, and it is registered as the participant `host`. Everyone who connects while a host session is live is a participant. The role is freed when the host's session ends, so whoever connects next becomes the host. SSH usernames are not authenticated, so this suits a single trusted group: start the host session yourself before sharing the address.

## Architecture

- **Server** (`cmd/haive-mind`): SSH server that manages sessions and message routing
- **Host TUI** (`tui/host`): Host interface for sending directives, viewing transcripts, aggregating responses; served to the first SSH connection
- **Participant TUI** (`tui/participant`): Participant interface for responding to agent prompts; served to every other SSH connection
- **Hub** (`hub/`): Message routing and agent communication
- **SSH Server** (`sshserver/`): SSH session management with Bubble Tea TUI integration

## End-to-End Integration Test

### Automated Tests

The end-to-end tests in `e2e/` build the real server and drive it over SSH with a fake agent:

```bash
go test -v ./e2e/...
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
   haive-mind --port 2222 --data-dir ./test-data --debug
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
# Fast checks while working (formatting, vet, every test except e2e)
bash scripts/check.sh

# Exactly what CI runs; do this before committing
bash scripts/check.sh --full

# Specific package
go test ./hub/...
go test ./tui/host/...
go test ./tui/participant/...
```

### Building from source

```bash
go build -o haive-mind ./cmd/haive-mind
```

## Project Structure

```
.
├── cmd/
│   └── haive-mind/      # The server: the only binary
├── hub/                 # Message hub and agent runner
├── sshserver/           # SSH server with TUI session management
├── tui/
│   ├── host/            # Host TUI components
│   └── participant/     # Participant TUI components
├── e2e/                 # End-to-end tests against the real binary
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

Copyright (C) 2026 Jake Franklin

Haive Mind is free software under the [GNU Affero General Public License v3.0](LICENSE). If you run a modified version as a network service, you must offer its users the source of your modifications.
