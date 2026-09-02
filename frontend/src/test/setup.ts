// Shared Vitest setup (SPEC-12 §6, extended SPEC-13): jest-dom matchers, RTL
// cleanup (vitest runs without globals, so the auto-registered cleanup never
// fires on its own), and a project-wide mock of the generated Wails bindings
// (regenerated at build time — tests never touch the real window['go'] bridge).
import '@testing-library/jest-dom/vitest'
import {afterEach, beforeAll, vi} from 'vitest'
import {cleanup} from '@testing-library/react'

beforeAll(() => {
  const warn = console.warn
  vi.spyOn(console, 'warn').mockImplementation((...args: unknown[]) => {
    const message = String(args[0] ?? '')
    if (
      message.startsWith('⚠️ React Router Future Flag Warning:') ||
      message.startsWith('React Router Future Flag Warning:')
    ) {
      return
    }
    warn(...args)
  })
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks() // reset spy call history, keep default implementations
})

vi.mock('@wailsjs/go/app/App', () => ({
  AppInfo: vi.fn(),
  DaemonStatus: vi.fn(),
  Health: vi.fn().mockResolvedValue({
    version: 'test', uptime_seconds: 0, core: 'healthy', scheduler: 'running',
  }),
  StartDaemon: vi.fn(),
  CreateTask: vi.fn(),
  UpdateTask: vi.fn(),
  DeleteTask: vi.fn(),
  GetTask: vi.fn(),
  ListTasks: vi.fn(),
  GetTaskYAML: vi.fn(),
  ValidateTaskYAML: vi.fn(),
  ParseTaskYAML: vi.fn(),
  RunTask: vi.fn(),
  SetTaskEnabled: vi.fn(),
  ImportTaskFromFile: vi.fn(),
  ExportTaskYAML: vi.fn(),
  PickScriptFile: vi.fn(),
  PickWorkingDir: vi.fn(),
  ListSecrets: vi.fn(),
  SetSecret: vi.fn(),
  DeleteSecret: vi.fn(),
  ListSchedules: vi.fn(),
  CreateSchedule: vi.fn(),
  UpdateSchedule: vi.fn(),
  DeleteSchedule: vi.fn(),
  EnableSchedule: vi.fn(),
  DisableSchedule: vi.fn(),
  ListRuns: vi.fn(),
  GetRun: vi.fn(),
  CancelRun: vi.fn(),
  ListSystemLog: vi.fn(),
  StartupEnabled: vi.fn().mockResolvedValue(false),
  StartupSet: vi.fn(),
  WatchdogEnabled: vi.fn().mockResolvedValue({installed: false, interval_minutes: 0}),
  WatchdogSet: vi.fn(),
  PauseScheduler: vi.fn(),
  ResumeScheduler: vi.fn(),
  ReconcileSchedules: vi.fn(),
  Stats: vi.fn().mockResolvedValue({
    tasks: 0, tasks_enabled: 0, schedules_enabled: 0, running: 0,
    runs_today: 0, success_today: 0, failed_today: 0,
    run_history: [], status_distribution: [], recent_activity: [],
  }),
  GetSettings: vi.fn().mockResolvedValue({
    log_retention_days: 90,
    sound_success: 'system',
    sound_failure: 'system',
    sound_timeout: 'system',
    reconcile_interval_min: 2,
    watchdog_interval_min: 5,
  }),
  UpdateSettings: vi.fn(),
  DataDir: vi.fn().mockResolvedValue(''),
  TasksDir: vi.fn().mockResolvedValue(''),
  OpenDataDir: vi.fn(),
}))

// CodeMirror measures text with Range.getClientRects on every animation
// frame — jsdom doesn't implement it. Patch so the editor runs quietly under
// tests (the layout numbers are unused there).
const emptyRectList = [] as unknown as DOMRectList
if (typeof Range !== 'undefined') {
  Range.prototype.getClientRects = () => emptyRectList
  Range.prototype.getBoundingClientRect = () =>
    ({
      x: 0,
      y: 0,
      top: 0,
      left: 0,
      right: 0,
      bottom: 0,
      width: 0,
      height: 0,
      toJSON: () => ({}),
    }) as DOMRect
}

// HeroUI Tabs uses useScrollShadow which depends on ResizeObserver.
if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as any
}

// HeroUI Toast uses useMediaQuery (matchMedia) for mobile layout detection.
if (typeof window !== 'undefined' && !window.matchMedia) {
  window.matchMedia = (query: string) =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }) as MediaQueryList
}

// HeroUI Tabs/Tooltip use SharedElementTransition which calls getAnimations().
if (typeof Element !== 'undefined' && !Element.prototype.getAnimations) {
  Element.prototype.getAnimations = () => []
}

// Sane defaults so pages render without "query data undefined" noise; tests
// override per-case. Missing tasks reject as the daemon does.
const bindings = await import('@wailsjs/go/app/App')
vi.mocked(bindings.ListTasks).mockResolvedValue([])
vi.mocked(bindings.GetTask).mockRejectedValue(new Error('not_found: task not found'))
vi.mocked(bindings.GetTaskYAML).mockRejectedValue(new Error('not_found: task not found'))
vi.mocked(bindings.ValidateTaskYAML).mockResolvedValue([])
vi.mocked(bindings.ListSecrets).mockResolvedValue([])
vi.mocked(bindings.ListSchedules).mockResolvedValue([])
vi.mocked(bindings.ListRuns).mockResolvedValue({runs: [], total: 0} as any)
vi.mocked(bindings.ListSystemLog).mockResolvedValue([] as any)