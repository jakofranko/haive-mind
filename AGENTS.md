## Checks

`bash scripts/check.sh` while working (fast, no e2e); `bash scripts/check.sh --full` before committing (CI's exact run). `bash scripts/red.sh` runs new tests against an earlier ref to show red.

## Agent skills

### Issue tracker

Issues live in GitHub Issues (uses `gh` CLI). See `docs/agents/issue-tracker.md`.

### Triage labels

Default five canonical labels: needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one GLOSSARY.md + docs/adr/ at repo root. See `docs/agents/domain.md`.