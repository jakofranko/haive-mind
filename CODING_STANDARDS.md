# Coding standards

Judgement calls for review. Formatting, `go vet`, always-true `if` guards and the race detector are enforced by `scripts/check.sh`, which CI runs, so they are not repeated here. Vocabulary comes from `GLOSSARY.md`; decisions are in `docs/adr/`.

## Tests

- **Test the real code through a public seam**: the hub via `NewHub()` with a stub `agent.Runner`, the coordinator, a TUI model's `Update` with a real or fake hub, or the built binary in `e2e/`. A test that reimplements logic from `main.go` tests a copy.
- **Assert on what a user sees**: strip terminal styling (`visible()`) before matching text. Golden files hold styled bytes; regenerate with `-update` and confirm the diff is only the lines you meant to change. To show something leaves a pane unchanged, compare its whole `View()` before and after rather than a hand-picked size: a pane is only drawn as wide as its content.
- **Wait for the outcome**: poll with `eventually`, `waitFor` or `eventuallyDrawn`, with deadlines sized for a loaded machine. Use a fixed sleep only to let a "nothing else happens" check settle.
- **Observe a running Bubble Tea program through its drawn output**, never its model fields, so the test and the program do not touch the same state from two goroutines.
- **Give a hub a stub runner (`SetAgentRunner`/`SetSynthesizer`) when a test sends a directive or aggregate.** `NewHub()` has no runner of its own, so nothing in a test can launch the real `opencode`.
- **Show red first**: a new test fails for the stated reason before the code that fixes it; break the fix once to confirm the test can fail. The commit message says how it went red: the failing test and what it reported.
- **Red for a race is a fail rate**: run the test against the code before the fix with `scripts/red.sh` and quote its tally in the commit message, and pair it with a test at a synchronous seam that fails every time.

## Design

- **Key identity by participant id** (`Agent-<ssh username>`, `host`). Display names are labels and are neither unique nor stable.
- **Only the hub invokes `agent.Runner`.** The coordinator and the TUIs send and subscribe.
- **Keep each message type's meaning in one place**: the hub's constants in `hub/message.go`, described in `GLOSSARY.md`.

## Commits

- **One logical change per commit, with its tests**, and each commit builds and passes alone. `git show --stat` should list the implementation beside its test.
