# Frontend Rules

## Component Architecture
- Component co-location: `Foo.tsx` + `Foo.test.tsx` in same directory
- Tests use `@testing-library/react` + `screen` queries
- Wails bindings mocked in `src/test/setup.ts` — never import real bindings in tests

## State Management
- React Query for server state (tasks, secrets, health, daemon status)
- Zustand for UI state (theme, accent color)
- URL params for filters (type, enabled on TasksPage)
- Never store server state in React state or context

## Styling
- Tailwind v4 utilities + HeroUI v3 components
- Custom controls in `components/controls.tsx` — use `inputCls` pattern
- Use HeroUI v3 `SelectField` from controls.tsx — NEVER native `<select>`
- All form controls must match border styles, padding, sizing
- Dark mode: `@custom-variant dark (&:where(.dark, .dark *))` — NOT OS preference

## Forms
- Canonical `TaskDraft` model with `emptyDraft()`, `draftFromTask()`, `draftToTask()`
- YAML serialization uses deterministic emitter — never `json.Marshal`
- Validation at Save time, not on tab switch
- Hidden fields for state-only inputs — never show disabled inputs

## Dark Mode
- Body: `overflow-hidden bg-background text-foreground`
- Theme tokens: CSS custom properties with `[data-theme="…"]` selectors
- HeroUI bridge: override `--foreground`, `--background`, `--field-foreground`, `--field-background`
- Tailwind dark mode tied to `.dark` class, NOT `prefers-color-scheme`

## Scrollbar
- Custom accent scrollbar via `::-webkit-scrollbar` only (no standard properties)
- Chromium 121+ disables webkit styling when `scrollbar-width` is set
- Firefox fallback: `@supports not selector(::-webkit-scrollbar)`
- Scrollbar gutter: `scrollbar-gutter: stable` to prevent layout shift

## Layout
- Shell: `flex h-screen flex-col`
- Scroll area: `min-h-0 flex-1 overflow-y-auto`
- Body: `overflow-hidden` — prevents body scroll
- TopNav: `shrink-0` — stays at top, never scrolls

## Secrets
- Values stored as `${KEY}` references in task YAML
- Never add literal/free-text input for secret values
- API never returns secret values — only keys
- Dropdown shows vault keys, user must add secrets in Settings first

## Testing
- Environment: jsdom
- HeroUI Select hidden native `<select>` in jsdom
- `fireEvent.change` on hidden select, not on labels
- Tests co-located with components
