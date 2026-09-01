# CSS & Styling Rules

## Tailwind v4
- Use Vite plugin (`@tailwindcss/vite`), NOT PostCSS
- Theme tokens via `@theme inline { ... }` in `main.css`
- Dark mode: `@custom-variant dark (&:where(.dark, .dark *))`

## HeroUI v3 Integration
- Import: `@import "@heroui/styles"` in `main.css`
- No provider needed (v3 removed HeroUIProvider requirement)
- Compound component API: `Select`, `Select.Trigger`, `Select.Value`, `Select.Indicator`, `Select.Popover`
- Style sub-components via `className` prop (NOT `classNames` object — that was v2)
- Bridge theme tokens: override `--foreground`, `--background`, etc. in theme blocks

## Form Controls
- All controls use `inputCls` from `controls.tsx`
- Pattern: `rounded-lg border border-zinc-200 bg-white/70 px-2.5 py-1.5 text-sm text-zinc-900`
- Dark: `dark:border-zinc-700 dark:bg-zinc-900/70 dark:text-zinc-100`
- Focus: `focus:border-accent focus:ring-1 focus-within:ring-accent-ring`

## Scrollbar
- Chromium: ONLY `::-webkit-scrollbar` pseudo-elements (no standard properties)
- Standard properties (`scrollbar-width`, `scrollbar-color`) disable webkit styling in Chromium 121+
- Firefox: `@supports not selector(::-webkit-scrollbar)` with standard properties
- Gutter: `scrollbar-gutter: stable` on scroll containers

## Dark Mode
- `@custom-variant dark (&:where(.dark, .dark *))` — NOT `prefers-color-scheme`
- Theme blocks: `:root { ... }` for light, `.dark, [data-theme="dark"] { ... }` for dark
- Body: `bg-background text-foreground`
- Color scheme: `color-scheme: light` / `color-scheme: dark`

## Layout
- Shell: `flex h-screen flex-col`
- Scroll area: `min-h-0 flex-1 overflow-y-auto`
- Body: `overflow-hidden` — prevents body scroll
- TopNav: `shrink-0` — stays at top

## Accent Colors
- 6 presets: blue, violet, emerald, amber, rose, cyan
- CSS variables: `--accent`, `--accent-contrast`, `--accent-ring`
- Applied via `data-accent` attribute on `<html>`
- Tailwind tokens: `@theme inline { --color-accent: var(--accent); ... }`

## Component Styling
- Never use native HTML `<select>` with `appearance: none` — use HeroUI SelectField
- Remove/delete buttons: borderless `×` icons, red on hover
- Action buttons in rows: icon-only (play, trash SVGs)
- Floating elements: individual border + shadow + backdrop blur
