# 0002. The aggregate is synthesized by a one-off run, shared as a draft, and edited by the host

Status: accepted

## Context

Participants answer the host's question and the group needs one answer to send back to the agent. The agent keeps a long-running session whose context the group's raw answers would clutter, and nothing in a round needs that context to combine answers.

## Decision

When the host asks to aggregate, the coordinator builds a prompt from the round's question and each participant's latest response. The hub runs it through the synthesizer, a separate one-off `opencode run`. The result is published as an `aggregate_draft` that everyone sees and that opens in the host's input. The host edits it and Enter sends it to the agent as the aggregate. A synthesis failure becomes an `agent_error` with a Retry for the host.

## Consequences

- The main agent session never sees the raw responses, only the host-approved aggregate.
- Everyone sees the draft, which fits a long-running collaborative session.
- The synthesis call has no memory: anything the model needs must be in the prompt.
- Both agent and synthesizer share the `--agent-timeout` setting.
