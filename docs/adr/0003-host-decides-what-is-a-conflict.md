# 0003. The host decides what is a conflict; the model does not

Status: accepted

## Context

The spec asked the host to "highlight conflicting responses and request revisions until resolved". The first synthesis prompt also asked the model to point out disagreements, which mixed two jobs: combining answers and judging which differences matter. Detecting divergence automatically would need a model call or a heuristic, and neither is reliable.

## Decision

The host flags participants (`c` in the participants list). A flagged participant is told, gets their last answer back to edit, and shows as `revise` to everyone. Resubmitting lifts that flag; `[Resolve]` clears all of them; a new round clears them too. Aggregating while participants are flagged warns first but does not block. The coordinator owns the flags. The synthesis prompt does not ask the model to find disagreements, and only each participant's latest response in a round is synthesized.

## Consequences

- No automatic conflict detection exists or is planned; the host's judgement is the signal.
- Flags are per participant per round, not per message.
- Responses carry the sender's stable id (`FromID`) so "latest per participant" and flags do not depend on display names.
