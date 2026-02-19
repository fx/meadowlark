# Copilot Instructions

## PR Review Checklist (CRITICAL)

- **Task completion**: EVERY PR MUST mark completed task(s) as done (`- [x]`) in the relevant tracking file (`docs/PROJECT.md` or the spec file in `docs/specs/`). REQUEST CHANGES if missing.
- **TypeScript in devDependencies**: TypeScript belongs in `devDependencies` for this project. Do not suggest moving or removing it. It is used for type-checking only (`noEmit: true`).
