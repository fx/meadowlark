# PR Review

## Task Cross-Reference

Cross-reference every PR against task lists in `docs/changes/` and `docs/tasks.md`. If the PR completes work tracked in those files, the task checkboxes MUST be updated in this same PR. Request changes if missing.

---

# Copilot Instructions

## PR Review Checklist (CRITICAL)
- **TypeScript in devDependencies**: TypeScript belongs in `devDependencies` for this project. Do not suggest moving or removing it. It is used for type-checking only (`noEmit: true`).
- **`@theme inline` in globals.css**: Intentional. Tailwind v4 uses `@theme inline` with CSS custom property indirection (`:root`/`.dark` blocks + `var()` refs). Do not flag as breaking theme switching.
- **Dark variant `(&:is(.dark *))`**: Intentional. This is the upstream shadcn/ui pattern for Tailwind v4. The `.dark` class is on `<html>`, descendants match via `*`.
- **`--destructive-foreground` matching `--destructive`**: Intentional. Newer shadcn/ui uses `text-white` on destructive buttons; this variable is for destructive text on neutral backgrounds.
- **CLAUDE.md changes**: CLAUDE.md is part of this repo and tracked in git. Changes to it are intentional project documentation updates.
- **No SSRF protection on probe/TTS endpoints**: Intentional. Meadowlark is a self-hosted home automation bridge that must connect to user-configured TTS endpoints, often on the local network. SSRF protection would break the core use case.
- **Context propagation via derived contexts**: When goroutines use `context.WithTimeout(ctx, ...)` derived from a parent context, cancellation already propagates. Adding select-on-`ctx.Done()` wrappers around `wg.Wait()` is unnecessary and leaks goroutines.
