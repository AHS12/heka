# Heka UI Prettification — Research & Design Direction

> Scope: **Frontend only**. No changes to daemon, Go backend, or IPC layer.

---

## 1. Current State Assessment

### What We Have

| Layer | Current Implementation | Notes |
|-------|----------------------|-------|
| **Theme system** | `theme.ts` — light/dark/system | Zustand store, sets `data-theme` + `.dark` class on `<html>` |
| **Accent system** | `accent.ts` — 6 presets + custom | blue, violet, emerald, amber, rose, cyan + custom hex |
| **CSS variables** | `main.css` — 132 lines | `--foreground`, `--background`, `--accent`, `--accent-contrast`, `--accent-ring` |
| **HeroUI integration** | `@heroui/styles` imported | Only 3 HeroUI components used: `Select`, `Label`, `ListBox` |
| **Controls** | Custom `controls.tsx` | `TextInput`, `NumberInput`, `Toggle`, `SelectField`, `Field` — all hand-rolled |
| **Font** | System default (no web font loaded) | Falls through to OS monospace stack |
| **Animations** | Minimal | `framer-motion` installed but limited use; scrollbar fade transition |
| **Background** | Subtle gradient in `AppLayout` | `bg-gradient-to-b from-zinc-100 via-zinc-50 to-zinc-100` (light) / zinc-950 (dark) |
| **Cards/panels** | Manual Tailwind classes | `rounded-2xl border border-zinc-200/80 bg-white/70 backdrop-blur-sm` |

### Current Color Tokens

```
Light: foreground oklch(0.21 0.006 286) / background oklch(0.98 0 0)
Dark:  foreground oklch(0.95 0 0) / background oklch(0.14 0.005 286)
Accent: oklch(0.55 0.22 250) default blue
```

---

## 2. Design Vision

### Overall Aesthetic Direction

**"Terminal Elegance"** — A warm, developer-focused desktop app that blends:
- **OpenCode's** monospace-first, terminal-native warmth (but lighter, not full dark terminal)
- **RetroUI's** pixel-inspired texture on select elements (subtle borders, not full pixel art)
- **Gradient depth** that gives the UI dimensionality without being flashy
- **Modern HeroUI** components as the foundation for all interactive elements

The goal: make Heka feel like a **premium developer tool** — not a generic web app, not a full retro game UI, but something in between with character.

---

## 3. Theme Variants

### 3.1 Theme Architecture

Current: `light` | `dark` | `system`

Proposed: **6 themes** organized as variant pairs within light/dark.

```
light:     default | khaki | gradient
dark:      default | high-contrast | gradient
```

Each theme is a full `data-theme` value with its own CSS variable block.

### 3.2 Light — Default (Prettified)

Evolution of current light theme with warmer tones and more depth.

```css
[data-theme="light"] {
  color-scheme: light;

  /* Warmer foreground — not pure black, slight brown warmth like OpenCode's inverse */
  --foreground: oklch(0.18 0.008 50);
  --background: oklch(0.99 0.002 80);         /* warm off-white, subtle peachy tint */

  /* Surface layers with increasing depth */
  --surface: oklch(1.0 0 0);                   /* pure white cards */
  --surface-secondary: oklch(0.97 0.003 80);   /* slightly tinted panels */

  /* Fields */
  --field-background: oklch(1.0 0 0);
  --field-foreground: var(--foreground);
  --field-border: oklch(0.88 0.01 70);         /* warm gray border */

  /* Accent — keeps existing accent system, these are overrides for default blue */
  --accent: oklch(0.55 0.22 250);
  --accent-contrast: oklch(0.99 0 0);

  /* Status */
  --success: oklch(0.65 0.18 155);
  --warning: oklch(0.78 0.14 75);
  --danger: oklch(0.62 0.22 25);

  /* Borders — warmer than zinc */
  --border: oklch(0.88 0.012 70);

  /* Shadows — warm-tinted */
  --shadow-color: 30 20% 50%;
}
```

### 3.3 Light — Khaki Variant

A warm, earthy light theme. Think desert sand meets developer tool. Distinct from default by its khaki/olive undertone.

```css
[data-theme="khaki"] {
  color-scheme: light;

  /* Earthy palette */
  --foreground: oklch(0.22 0.015 70);          /* dark warm brown */
  --background: oklch(0.97 0.015 85);          /* warm cream with khaki tint */

  --surface: oklch(0.99 0.008 80);             /* near-white with warm cast */
  --surface-secondary: oklch(0.95 0.02 80);    /* light khaki */

  --field-background: oklch(0.99 0.008 80);
  --field-foreground: var(--foreground);
  --field-border: oklch(0.85 0.025 80);        /* visible khaki border */

  /* Accent — earthy amber/copper by default (still overridable) */
  --accent: oklch(0.62 0.14 60);               /* warm copper */
  --accent-contrast: oklch(0.99 0 0);

  --success: oklch(0.58 0.14 145);             /* olive green */
  --warning: oklch(0.72 0.16 65);              /* golden amber */
  --danger: oklch(0.58 0.18 20);               /* brick red */

  --border: oklch(0.84 0.025 75);              /* khaki border */

  /* Gradient for background */
  --gradient-start: oklch(0.97 0.015 85);
  --gradient-mid: oklch(0.95 0.02 82);
  --gradient-end: oklch(0.93 0.018 78);
}
```

### 3.4 Dark — High Contrast Variant

Maximum readability, sharp borders, no subtlety — for power users and bright environments.

```css
[data-theme="high-contrast"] {
  color-scheme: dark;

  --foreground: oklch(1.0 0 0);                /* pure white */
  --background: oklch(0.08 0.005 286);         /* near-black */

  --surface: oklch(0.12 0.005 286);
  --surface-secondary: oklch(0.16 0.005 286);

  --field-background: oklch(0.12 0.005 286);
  --field-foreground: var(--foreground);
  --field-border: oklch(0.35 0 0);             /* high contrast border */

  /* Bright accent */
  --accent: oklch(0.7 0.2 250);                /* vivid blue */
  --accent-contrast: oklch(0.08 0 0);          /* dark text on bright accent */

  --success: oklch(0.8 0.18 155);
  --warning: oklch(0.85 0.15 75);
  --danger: oklch(0.75 0.22 25);

  --border: oklch(0.35 0 0);                   /* clear borders */

  /* High-contrast shadows */
  --shadow-color: 0 0% 0%;
}
```

### 3.5 Gradient Variants (Light + Dark)

Both light and dark get a gradient variant. These use CSS custom properties for the gradient stops, applied to the `<body>` / app shell.

```css
/* Light gradient */
[data-theme="gradient"] {
  color-scheme: light;

  --foreground: oklch(0.18 0.008 50);
  --background: oklch(0.98 0.003 80);          /* transparent-ish, gradient shows through */

  /* Gradient stops driven by accent */
  --gradient-start: color-mix(in oklch, var(--accent) 6%, oklch(0.98 0 0));
  --gradient-mid: oklch(0.99 0.001 80);
  --gradient-end: color-mix(in oklch, var(--accent) 4%, oklch(0.97 0.002 80));

  /* Subtle noise/grain texture via CSS — optional */
  --gradient-angle: 135deg;
}

/* Dark gradient */
[data-theme="gradient"][data-theme="gradient"] /* dark applied via .dark class */ {
  --gradient-start: color-mix(in oklch, var(--accent) 10%, oklch(0.14 0.005 286));
  --gradient-mid: oklch(0.13 0.004 286);
  --gradient-end: color-mix(in oklch, var(--accent) 6%, oklch(0.16 0.005 286));
}
```

**Applied in AppLayout:**
```tsx
// Instead of hardcoded gradient:
<div className="min-h-screen bg-[var(--gradient-start)] via-[var(--gradient-mid)] to-[var(--gradient-end)]"
     style={{ backgroundImage: 'linear-gradient(var(--gradient-angle, 180deg), var(--gradient-start), var(--gradient-mid), var(--gradient-end))' }}>
```

---

## 4. Font Strategy

### 4.1 Research Findings

| Font | Source | Character | Trade-off |
|------|--------|-----------|-----------|
| **Berkeley Mono** | Commercial (Type Network) | OpenCode's exact font — warm, terminal-native | Paid, not redistributable |
| **Space Mono** | Google Fonts | Retro-futuristic, geometric, distinctive | Slightly quirky for body text |
| **JetBrains Mono** | Google Fonts | Modern, high readability, coding ligatures | Very common, less distinctive |
| **IBM Plex Mono** | Google Fonts | Professional, neutral, excellent weight range | Less "character" |
| **Fira Code** | Google Fonts | Developer favorite, ligatures | Common, less retro |
| **Victor Mono** | Google Fonts | Semi-cursive italics, distinctive | Less known |
| **Geist Mono** | Vercel (free) | Vercel's new mono, clean + modern | Very new, less retro feel |

### 4.2 Recommendation: Space Mono + Inter

Use a **two-font system**:
- **Space Mono** for headings, labels, navigation, badges — gives the retro/terminal character
- **Space Mono** for code blocks and mono contexts (already its job)
- **Inter** (or **DM Sans**) as the body font for longer text — clean, readable, pairs well

This mirrors how OpenCode uses monospace everywhere, but we keep body text readable with a sans-serif. The key identity comes from Space Mono being prominent in headings and UI chrome.

**Alternative: Single monospace** (like OpenCode) — use Space Mono everywhere for a bolder identity. This is more polarizing but more distinctive.

### 4.3 Font Loading

```css
@import url('https://fonts.googleapis.com/css2?family=Space+Mono:wght@400;700&family=Inter:wght@400;500;600;700&display=swap');

@theme inline {
  --font-display: 'Space Mono', ui-monospace, monospace;
  --font-body: 'Inter', system-ui, sans-serif;
  --font-mono: 'Space Mono', ui-monospace, monospace;
}
```

Or self-host via `@fontsource/space-mono` and `@fontsource/inter` for offline desktop use.

---

## 5. Retro UI Elements (Subtle)

### 5.1 What to Borrow from RetroUI

RetroUI uses pixel-art borders and blocky shadows. We do **NOT** want full pixel art. Instead, we extract these principles:

| RetroUI Concept | Our Adaptation | Where Applied |
|----------------|----------------|---------------|
| Pixel-art borders | **2px solid borders** instead of 1px — thicker, more visible, slightly retro | Cards, panels, inputs, buttons |
| Blocky shadows | **Offset box-shadows** with no blur — `2px 2px 0 var(--accent)` | Buttons on hover, active states, focused inputs |
| Vintage color palettes | Warm-tinted neutrals (khaki undertone) | All theme variants |
| Chunky padding | Slightly more generous padding on buttons/inputs | Interactive elements |
| Rounded corners | Keep moderate radius (0.75rem–1rem) — NOT 4px pixel sharp | Consistent with current style |

### 5.2 Retro Accents (CSS Implementation)

```css
/* Retro-style offset shadow on buttons and cards */
.retro-shadow {
  box-shadow: 3px 3px 0 var(--accent);
  transition: box-shadow 0.1s ease, transform 0.1s ease;
}
.retro-shadow:hover {
  box-shadow: 4px 4px 0 var(--accent);
  transform: translate(-1px, -1px);
}
.retro-shadow:active {
  box-shadow: 1px 1px 0 var(--accent);
  transform: translate(2px, 2px);
}

/* Subtle scanline overlay — optional, for gradient theme only */
.scanline-overlay::after {
  content: '';
  position: fixed;
  inset: 0;
  pointer-events: none;
  background: repeating-linear-gradient(
    0deg,
    transparent,
    transparent 2px,
    rgba(0, 0, 0, 0.015) 2px,
    rgba(0, 0, 0, 0.015) 4px
  );
  z-index: 9999;
  opacity: 0;
  transition: opacity 0.3s;
}
[data-theme="gradient"] .scanline-overlay::after {
  opacity: 1;
}
```

### 5.3 What NOT to Do

- No pixel fonts (RetroUI's Press Start 2P / Minecraft font)
- No 8-bit color palettes
- No full pixel-art borders (the `border-image` pixel technique)
- No blocky/stepped shadows on everything
- The retro feel should come from **warmth, thickness, and texture** — not from being obviously "retro"

---

## 6. Gradient Design

### 6.1 Gradient Philosophy

Gradients should feel like **ambient lighting** — not like a 2015 website. The accent color tints the gradient subtly, creating a cohesive environment.

### 6.2 Gradient Types

| Type | Use Case | Implementation |
|------|----------|---------------|
| **Background gradient** | App shell background | Diagonal or vertical, accent-tinted neutrals |
| **Card sheen** | Subtle surface interest on cards | Near-invisible gradient overlay: `bg-gradient-to-br from-white/50 to-transparent` |
| **Accent gradient** | Primary buttons, active elements | `bg-gradient-to-r from-accent to-accent/80` — subtle, not rainbow |
| **Header gradient** | Top nav bar | Slightly darker tint for depth separation |
| **Status gradients** | Success/warning/danger indicators | Subtle directional tint on status backgrounds |

### 6.3 CSS Implementation

```css
/* App shell gradient — driven by theme variables */
.bg-theme-gradient {
  background: linear-gradient(
    var(--gradient-angle, 160deg),
    var(--gradient-start, var(--background)),
    var(--gradient-mid, var(--background)),
    var(--gradient-end, var(--background))
  );
}

/* Card with subtle sheen */
.card-sheen {
  position: relative;
  overflow: hidden;
}
.card-sheen::before {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(
    135deg,
    rgba(255, 255, 255, 0.08) 0%,
    transparent 50%,
    rgba(0, 0, 0, 0.02) 100%
  );
  pointer-events: none;
}

/* Accent gradient button */
.btn-gradient {
  background: linear-gradient(
    135deg,
    var(--accent),
    color-mix(in oklch, var(--accent) 80%, black)
  );
}
```

---

## 7. Animation System

### 7.1 Principles

- **Respect `prefers-reduced-motion`** — zero animations when OS requests it
- **User-configurable kill switch** — Settings toggle that persists to localStorage
- **Subtle by default** — nothing should distract from the task
- **Consistent easing** — use HeroUI's provided easing curves

### 7.2 Animation Catalog

| Animation | Trigger | Duration | Easing | Description |
|-----------|---------|----------|--------|-------------|
| **Page enter** | Route change | 200ms | `ease-out-cubic` | Fade in + slight upward slide (8px) |
| **Card appear** | Scroll into view / mount | 300ms | `ease-out-quart` | Fade in + scale from 0.98 to 1.0 |
| **Modal open** | Dialog trigger | 200ms | `ease-out-cubic` | Scale from 0.95 + fade |
| **Toast enter** | Notification | 250ms | `ease-out-expo` | Slide in from right + fade |
| **Toast exit** | Dismiss | 150ms | `ease-in-cubic` | Slide out right + fade |
| **Hover lift** | Button/card hover | 150ms | `ease-out-quad` | `translateY(-1px)` + shadow deepen |
| **Toggle switch** | Switch flip | 150ms | `ease-out-cubic` | Thumb slides with spring feel |
| **Focus ring** | Keyboard focus | 100ms | `ease` | Ring appears with subtle scale |
| **Skeleton pulse** | Loading state | 2s | linear | Shimmer gradient sweep |
| **Scrollbar fade** | Scroll stop | 600ms delay | ease | Already implemented |

### 7.3 Implementation

**Option A: CSS-only (lighter)** — All animations via Tailwind utilities and CSS `@keyframes`. No JS runtime.

```css
/* Animation utility classes */
.animate-in {
  animation: animate-in 0.3s var(--ease-out-quart) both;
}
@keyframes animate-in {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
}

/* Reduced motion override */
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    animation-duration: 0.01ms !important;
    transition-duration: 0.01ms !important;
  }
}

/* User kill-switch — class on <html> */
[data-no-animations="true"] *,
[data-no-animations="true"] *::before,
[data-no-animations="true"] *::after {
  animation-duration: 0.01ms !important;
  transition-duration: 0.01ms !important;
}
```

**Option B: framer-motion (already installed)** — For page transitions and orchestrated animations. More powerful but heavier.

**Recommendation:** CSS-only for micro-interactions (hover, focus, toggle), framer-motion only for page transitions if needed. The `AnimatePresence` + `motion.div` pattern is already available.

### 7.4 Settings Toggle

```tsx
// lib/animations.ts
export const STORAGE_KEY = 'heka-animations'

export function animationsEnabled(): boolean {
  const stored = localStorage.getItem(STORAGE_KEY)
  if (stored === 'false') return false
  return true // default on
}

export function setAnimationsEnabled(enabled: boolean) {
  localStorage.setItem(STORAGE_KEY, String(enabled))
  document.documentElement.dataset.noAnimations = String(!enabled)
}
```

Applied at startup in `main.tsx`:
```tsx
if (!animationsEnabled()) {
  document.documentElement.dataset.noAnimations = 'true'
}
```

---

## 8. HeroUI Component Migration

### 8.1 Priority Matrix

| Priority | Component | Current | Replacement | Files Affected |
|----------|-----------|---------|-------------|----------------|
| **P0** | Switch | Custom `<button role="switch">` | HeroUI `Switch` | `controls.tsx`, `SettingsPage.tsx`, `TaskTable.tsx`, `ScheduleTable.tsx` |
| **P0** | DatePicker | Native `<input type="datetime-local">` | HeroUI `DatePicker` | `ScheduleForm.tsx` |
| **P0** | Date picker | Native `<input type="date">` | HeroUI `DatePicker` | `RunFilters.tsx` |
| **P1** | Tabs | Manual `role="tablist"` | HeroUI `Tabs` | `TaskEditorPage.tsx`, `OutputViewer.tsx` |
| **P1** | Toast | Custom `useToast()` | HeroUI `Toast` | `TasksPage.tsx`, `SchedulesPage.tsx` |
| **P1** | Tooltip | CSS `group-hover` | HeroUI `Tooltip` | `DaemonStatusIcon.tsx` |
| **P1** | Chip/StatusChip | Custom `<span>` | HeroUI `Chip` | `TaskTable.tsx`, `ScheduleTable.tsx`, `RunsTable.tsx` |
| **P2** | Button | Native `<button>` (30+) | HeroUI `Button` | Across entire app |
| **P2** | Table | Native `<table>` (4) | HeroUI `Table` | All table components |
| **P2** | Input | Custom `TextInput` | HeroUI `Input` | `controls.tsx` + downstream |
| **P2** | NumberInput | Custom `NumberInput` | HeroUI `NumberInput` | `controls.tsx` + `SettingsPage.tsx` |
| **P2** | Pagination | Two bare buttons | HeroUI `Pagination` | `LogsPage.tsx` |
| **P3** | TimeField | `<input type="number">` for hours/mins | HeroUI `TimeField` | `RecurrenceBuilder.tsx` |
| **P3** | Link | Native `<a>` | HeroUI `Link` | `AboutPage.tsx` |

### 8.2 DatePicker Integration

HeroUI v3 `DatePicker` uses `@internationalized/date` types. Key integration points:

```tsx
import {Calendar, DateField, DatePicker, Label} from '@heroui/react'
import {parseDate, parseZonedDateTime, getLocalTimeZone} from '@internationalized/date'

// For ScheduleForm (datetime-local replacement):
<DatePicker granularity="minute" name="schedule-time" value={value} onChange={setValue}>
  <Label>Schedule time</Label>
  <DateField.Group fullWidth>
    <DateField.Input>{(segment) => <DateField.Segment segment={segment} />}</DateField.Input>
    <DateField.Suffix>
      <DatePicker.Trigger>
        <DatePicker.TriggerIndicator />
      </DatePicker.Trigger>
    </DateField.Suffix>
  </DateField.Group>
  <DatePicker.Popover>
    <Calendar aria-label="Schedule date">
      {/* ... standard calendar composition ... */}
    </Calendar>
  </DatePicker.Popover>
</DatePicker>

// For RunFilters (date-only filter):
<DatePicker granularity="day" name="from-date" value={fromDate} onChange={setFromDate}>
  <Label>From</Label>
  {/* ... same composition with day granularity ... */}
</DatePicker>
```

**Value conversion:** The existing ISO string values will need conversion helpers:
```tsx
// DateValue (from @internationalized/date) ↔ ISO string
function dateValueToISO(v: DateValue | null): string | null {
  return v ? v.toString() : null
}
function isoToDateValue(iso: string | null): DateValue | null {
  return iso ? parseDate(iso.slice(0, 10)) : null
}
```

### 8.3 Required Dependencies

```bash
npm install @internationalized/date
```

Already transitive through `@heroui/react`, but should be explicit.

---

## 9. Settings Page Updates

The Settings > Appearance section needs expansion:

### New Controls

| Control | Type | Persisted To | Notes |
|---------|------|-------------|-------|
| Theme variant | `SelectField` | `heka-theme-variant` localStorage | light-default, light-khaki, light-gradient, dark-default, dark-hc, dark-gradient |
| Animations | `Switch` | `heka-animations` localStorage | Toggle all animations on/off |
| (existing) Theme mode | `SelectField` | light/dark/system | Already exists |
| (existing) Accent | Swatches + custom | Already exists | Keep as-is |

### Theme Variant → data-theme Mapping

```ts
type ThemeVariant = 'default' | 'khaki' | 'gradient' | 'high-contrast'

// Resolved data-theme values:
// light + default    → "light"
// light + khaki      → "khaki"
// light + gradient   → "gradient"
// dark + default     → "dark"
// dark + high-contrast → "high-contrast"
// dark + gradient    → "gradient-dark"
// system delegates to light or dark, then applies variant
```

---

## 10. Implementation Phases

### Phase 1: Foundation (no visual changes yet)
1. Add font loading (Space Mono + Inter or JetBrains Mono)
2. Expand theme system to support variants
3. Add animation toggle infrastructure
4. Install `@internationalized/date` explicitly

### Phase 2: Theme Variants
1. Define all 6 theme CSS variable blocks in `main.css`
2. Update `theme.ts` to support variant choice
3. Update Settings UI with variant selector
4. Update `AppLayout` to use gradient CSS variables

### Phase 3: Component Migration (HeroUI)
1. Replace Toggle → Switch
2. Replace native date inputs → DatePicker
3. Replace tabs → Tabs
4. Replace toasts → Toast
5. Replace StatusChip → Chip
6. Replace tooltips → Tooltip

### Phase 4: Polish
1. Add retro accent styles (thicker borders, offset shadows)
2. Add micro-interactions (hover lifts, focus rings, page transitions)
3. Apply gradient system to background and cards
4. Add scanline overlay for gradient theme
5. Settings page polish with all new controls

### Phase 5: Testing & Refinement
1. Visual QA across all 6 themes
2. Animation toggle verification
3. Reduced-motion compliance
4. Accessibility audit (focus management, screen reader)
5. Performance check (font loading, CSS specificity)

---

## 11. Design Token Summary

### Full Token Set (all themes)

```css
@theme inline {
  /* Colors — from CSS vars */
  --color-background: var(--background);
  --color-foreground: var(--foreground);
  --color-surface: var(--surface);
  --color-surface-foreground: var(--surface-foreground);
  --color-accent: var(--accent);
  --color-accent-contrast: var(--accent-contrast);
  --color-border: var(--border);

  /* New: gradient tokens */
  --color-gradient-start: var(--gradient-start, var(--background));
  --color-gradient-mid: var(--gradient-mid, var(--background));
  --color-gradient-end: var(--gradient-end, var(--background));

  /* Typography */
  --font-display: 'Space Mono', ui-monospace, monospace;
  --font-body: 'Inter', system-ui, -apple-system, sans-serif;
  --font-mono: 'Space Mono', ui-monospace, monospace;

  /* Retro accent tokens */
  --shadow-retro: 3px 3px 0 var(--accent);
  --shadow-retro-hover: 4px 4px 0 var(--accent);
  --shadow-retro-active: 1px 1px 0 var(--accent);

  /* Border thickness (retro feel) */
  --border-width-retro: 2px;
}
```

---

## 12. Reference Links

### Research Sources
- [HeroUI v3 Theming Docs](https://heroui.com/en/docs/react/getting-started/theming) — CSS variable architecture, custom themes
- [HeroUI v3 DatePicker](https://heroui.com/en/docs/react/components/date-picker) — Composition API, `@internationalized/date` types
- [HeroUI v3 TimeField](https://heroui.com/en/docs/react/components/time-field) — Time input with segmented editing
- [OpenCode Design System](https://open-design.ai/plugins/design-system-opencode-ai/) — Monospace-first, warm dark palette, terminal aesthetic tokens
- [RetroUI GitHub](https://github.com/Dksie09/RetroUI) — Pixel-bordered React components, blocky shadows
- [WebGradients](https://webgradients.com/) — Gradient inspiration
- [HeroUI Theme Builder](https://heroui.com/themes) — Visual theme customization tool
- [OKLCH Color Tool](https://oklch.com/) — Modern color space for token definitions

### OpenCode's Design Tokens (for reference)
- Background: `#201d1d` (warm near-black with reddish-brown undertone)
- Text: `#fdfcfc` (warm off-white)
- Font: Berkeley Mono (commercial) — our free alternative: Space Mono
- Border radius: 4px default, 6px inputs
- Shadows: **zero** — flat, terminal aesthetic
- Accent: `#007aff` (Apple system blue)
