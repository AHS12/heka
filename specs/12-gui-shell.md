# SPEC-12 — GUI Shell

Status: **Draft** · Depends on: SPEC-01 (scaffold), SPEC-07 (ipc client), SPEC-08 (daemon control reuse) · Master spec: §3, §11, PRD §23–24

## Goal

Replace the placeholder frontend with the real desktop-shell architecture: Tailwind + HeroUI wired into the Wails Vite template, routing, dark/light theming, the typed API layer that bridges React → Wails bindings → `ipc.Client` → daemon, and the sidebar layout with per-route placeholders (PRD §24). Pages ship in SPEC-13/14; this spec is the skeleton they hang on.

## Scope

**In:**
- Frontend deps: Tailwind (v4), `@heroui/react`, `framer-motion`, `react-router` (HashRouter), TanStack Query, zustand, Vitest
- `frontend/src/lib`: API client + error mapping, health polling hooks, theme store
- `internal/app`: passthrough bindings `Health`, `StartDaemon`, `DaemonStatus`
- Layout: `AppLayout` (sidebar + topbar + health pill), router, placeholder pages
- The shell's "daemon not running" affordance (PRD §3.1)

**Out:** real pages (SPEC-13 Tasks, SPEC-14 Schedules/Jobs/Runs/Logs, settings/dashboard content), task editor, import/export UI, iconography polish, custom fonts (all SPEC-16 or later).

## 1. Binding passthrough (bridge)

Browser JS can't open named pipes, so the frontend never calls `ipc.Client` directly. `internal/app.App` (bound since SPEC-01) grows typed methods that wrap the client:

```go
func (a *App) Health() (HealthDTO, error)          // ipc.Client.Health() → DTO
func (a *App) DaemonStatus() (string, error)       // "running" | "not-running" (ErrDaemonNotRunning)
func (a *App) StartDaemon() error                  // daemon.Start(cfg) — same code the CLI uses (SPEC-08)
```

- **Why passthrough and not binding `ipc.Client` directly**: one place for DTO shapes the JS side owns, one place to translate IPC envelope errors into JS `Error`s with `code`, and one place to surface `daemon_not_running` before pages even load.
- Only shell-level methods land in this spec. Task/schedule/run DTOs come with their pages (SPEC-13/14) — same pattern, no new architecture.

## 2. Frontend deps & tooling

| Package | Version lane | Role |
|---|---|---|
| `tailwindcss` + `@tailwindcss/vite` | v4 | styling engine (Vite plugin — replaces the template's PostCSS setup) |
| `@heroui/react` + `framer-motion` | current 2.x | component library + motion peer dep |
| `react-router` | current | routing — **HashRouter** (embedded `wails://` serving has no server-side history fallback) |
| `@tanstack/react-query` | v5 | server state (health + lists), polling |
| `zustand` | v5 | small UI state (theme, daemon state) |
| `vitest` + `jsdom` | current | unit + minimal component tests |

- HeroUI loading: `HeroUIProvider` at the app root + Tailwind integration via HeroUI's v4-friendly CSS plugin — the **exact plugin token is pinned during implementation** (master §15 risk: if it fights Tailwind v4, the documented fallback is Tailwind v3).
- Keep React 18 (template baseline); no upgrade in this spec.

## 3. App structure

```text
frontend/src/
├── lib/
│   ├── api.ts         # typed wrappers over ../wailsjs/go/app/App + error mapping
│   ├── query.ts       # useHealth() polling hook (5s), useDaemonStatus()
│   └── theme.ts       # ThemeStore: 'light'|'dark'|'system' (default), persisted to localStorage
├── layouts/AppLayout.tsx   # sidebar + topbar + <Outlet/>
├── components/             # Sidebar, Topbar, HealthPill, DaemonDownBanner
├── pages/          # Placeholder.tsx (shell-time stand-in for all routes)
├── router.tsx       # HashRouter with PRD §24 routes
├── App.tsx          # HeroUIProvider + QueryClientProvider + RouterProvider + theme wiring
└── main.css         # tailwind v4 entry (+ heroui plugin/preset)
```

### Routing (PRD §24)

```text
/            → Dashboard    /tasks      → Tasks
/schedules   → Schedules   /jobs       → Jobs
/runs        → Runs        /logs       → Logs
/settings    → Settings
```

All render `Placeholder` in SPEC-12 (each shows its route name + a "lands in SPEC-13/14" note).

- **Sidebar**: rounded-list nav with route links; active state via `NavLink` (PRD §23: dense, keyboard-friendly — focus-visible rings from HeroUI).
- **Topbar**: app name/version + `HealthPill` (green "daemon healthy" / red "daemon not running" / amber "starting").
- **DaemonDownBanner**: when `DaemonStatus ≠ running`, a non-blocking banner at the top of the content area: "Heka daemon is not running." + **Start Daemon** button (calls `StartDaemon`, then re-polls).

## 4. Theme

- `theme.ts` store: `light | dark | system` (default `system`), resolved via `matchMedia('(prefers-color-scheme: dark)')`.
- Toggle in the topbar (dev affordance; the Settings page in SPEC-16 formalizes it). Persisted in `localStorage`; applied by setting `data-theme` on `<html>` + HeroUI provider theme prop.
- Dark and light palettes are hard-coded design tokens in this spec (PRD §23: no clutter, developer-tool feel); theme-able tokens via CSS variables so SPEC-16 can surface a picker.

## 5. Health polling

`useHealth()` polls the `Health` binding every 5 s (React Query `refetchInterval`). The pill and banner derive from it. Pause polling while the daemon is down (cheap backoff: poll every 10 s when down) — the Query config encodes that.

## 6. Testing

`vitest` + `jsdom`; the `../wailsjs/go/app/App` module is `vi.mock`'ed (bindings are generated at build time, not checked in):

1. `api.ts`: happy path JSON → typed DTO; IPC error envelope → `Error` with `code`; `ErrDaemonNotRunning` → sentinel.
2. `theme.ts`: default `system` resolves to matchMedia; explicit selection persists; `data-theme` applied.
3. `router.tsx`: every route renders; unknown path → redirect `/`.
4. `Sidebar`: active `NavLink` styling; keyboard focus present.
5. `HealthPill` states: running/not-running/starting render text + classes.
6. `DaemonDownBanner`: shows when down; clicking Start invokes the mock, then re-polls.
7. Shell smoke: `AppLayout` renders children via `<Outlet/>` (component render test).
8. `make check` green (+ Vitest through the Makefile, see §8).

## 7. Files

```text
frontend/package.json / vite.config.ts / tsconfig.json   # deps + tailwind v4 + vitest
frontend/src/main.css                                    # tailwind entry + heroui plugin
frontend/src/lib/{api,query,theme}.ts
frontend/src/layouts/AppLayout.tsx
frontend/src/components/{Sidebar,Topbar,HealthPill,DaemonDownBanner}.tsx
frontend/src/pages/Placeholder.tsx
frontend/src/router.tsx  frontend/src/App.tsx  frontend/src/main.tsx (providers)
internal/app/app.go      # + Health / DaemonStatus / StartDaemon bindings
Makefile                 # test target runs `go test` + Vitest (frontend test script)
```

## 8. Makefile

`make test` becomes Go + frontend: `test: frontend/dist` no longer sufficient — add `frontend-test` target as a dependency of `test`/`check`: `cd frontend && npx vitest run`. Keeps the quality gate honest for the new code.

## 9. Acceptance criteria

1. `make dev` opens the shell: sidebar with all 7 routes, topbar with health pill, placeholders render on navigation; URL hash changes.
2. Daemon stopped (kill it) → pill red + banner with a working **Start Daemon** (daemon back → pill green within a poll cycle).
3. Theme toggle switches dark/light and survives a restart (localStorage).
4. Unknown route → redirect `/`.
5. `npx vitest run` green; `make check` green.
6. No task/schedule functionality in this spec — pages are placeholders.

## DoD

1. Spec approved by user.
2. Criteria verified on this machine (`make dev` run with the user).
3. HeroUI×Tailwind v4 integration resolved (plugin pinned) or documented fallback to v3 chosen.