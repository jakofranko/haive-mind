# Coding standards

Judgement calls for review. Formatting, `go vet` and the race detector are enforced by CI (`.github/workflows/ci.yml`), so they are not repeated here. Vocabulary comes from `GLOSSARY.md`; decisions are in `docs/adr/`.

## Tests

- **Test the real code through a public seam**: the hub via `NewHub()` with a stub `agent.Runner`, the coordinator, a TUI model's `Update` with a real or fake hub, or the built binary in `e2e/`. A test that reimplements logic from `main.go` tests a copy.
- **Assert on what a user sees**: strip terminal styling (`visible()`) before matching text. Golden files hold styled bytes; regenerate with `-update` and confirm the diff is only the lines you meant to change.
- **Wait for the outcome**: poll with `eventually`, `waitFor` or `eventuallyDrawn`, with deadlines sized for a loaded machine. Use a fixed sleep only to let a "nothing else happens" check settle.
- **Observe a running Bubble Tea program through its drawn output**, never its model fields, so the test and the program do not touch the same state from two goroutines.
- **Show red first**: a new test fails for the stated reason before the code that fixes it; break the fix once to confirm the test can fail.

## Design

- **Key identity by participant id** (`Agent-<ssh username>`, `host`). Display names are labels and are neither unique nor stable.
- **Only the hub invokes `agent.Runner`.** The coordinator and the TUIs send and subscribe.
- **Keep each message type's meaning in one place**: the hub's constants in `hub/message.go`, described in `GLOSSARY.md`.

## Commits

- **One logical change per commit, with its tests**, and each commit builds and passes alone. `git show --stat` should list the implementation beside its test.
