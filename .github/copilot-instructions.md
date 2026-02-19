# Copilot Instructions

## PR Review Checklist (CRITICAL)

- **Task completion**: EVERY PR MUST mark completed task(s) as done (`- [x]`) in the relevant tracking file (`docs/PROJECT.md` or the spec file in `docs/specs/`). REQUEST CHANGES if missing.
- **TypeScript in devDependencies**: TypeScript belongs in `devDependencies` for this project. Do not suggest moving or removing it. It is used for type-checking only (`noEmit: true`).
- **`@theme inline` in globals.css**: Intentional. Tailwind v4 uses `@theme inline` with CSS custom property indirection (`:root`/`.dark` blocks + `var()` refs). Do not flag as breaking theme switching.
- **Dark variant `(&:is(.dark *))`**: Intentional. This is the upstream shadcn/ui pattern for Tailwind v4. The `.dark` class is on `<html>`, descendants match via `*`.
- **`--destructive-foreground` matching `--destructive`**: Intentional. Newer shadcn/ui uses `text-white` on destructive buttons; this variable is for destructive text on neutral backgrounds.
- **CLAUDE.md changes**: CLAUDE.md is part of this repo and tracked in git. Changes to it are intentional project documentation updates.
