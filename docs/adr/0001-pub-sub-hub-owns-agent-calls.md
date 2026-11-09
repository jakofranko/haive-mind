# 0001. The hub is a pub/sub bus and the only thing that calls the agent

Status: accepted

## Context

The first design gave the hub typed channels per message flow and methods like `hub.Aggregate` and `MarkConflict`. In practice the TUIs, the SSH server and `main` each consumed hub output and several of them invoked the agent. That produced duplicated agent calls, lost replies (two readers on one outbound channel), out-of-order delivery and an aggregate loop.

## Decision

The hub is a pub/sub bus. Subscribers register for message types and receive them in send order from per-subscriber mailboxes. The hub is the sole owner of agent and synthesizer invocation, through the `agent.Runner` interface. Session state (transcript, round, responses, flags) lives in the `coordinator`, which reacts to hub messages and never invokes the agent. TUIs and the SSH server only send and subscribe.

## Consequences

- One place decides when the agent runs, so a directive produces exactly one reply and a failure produces exactly one `agent_error`.
- Tests sit at three seams: the hub (`NewHub()` with a stub runner), the coordinator, and the TUI models with a real or fake hub, plus an e2e test against the real binary.
- Ticket criteria written for the channel API (`hub.Aggregate`, `MarkConflict`, `ResolveConflict`) are retired; tickets #1 and #3 say what replaced them.
- The hub has no targeted send. Messages go to all subscribers and a UI filters by `To`/`FromID` when it needs to.
- History reaches a session as an addressed message: on connect the coordinator publishes a transcript snapshot with `To` set to that participant id, not as a hub replay.
