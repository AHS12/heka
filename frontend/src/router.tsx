// router.tsx (SPEC-12 §3) — HashRouter (embedded wails:// serving has no
// server-side history fallback) with the PRD §24 route table. Unknown paths
// redirect to Dashboard.
import {HashRouter, Navigate, Route, Routes} from 'react-router-dom'
import {AppLayout} from './layouts/AppLayout'
import {Placeholder} from './pages/Placeholder'
import {TasksPage} from './pages/TasksPage'
import {TaskEditorPage} from './pages/TaskEditorPage'
import {SettingsPage} from './pages/SettingsPage'

export function AppRouter() {
  return (
    <HashRouter>
      <Routes>
        <Route element={<AppLayout />}>
          <Route index element={<Placeholder title="Dashboard" />} />
          <Route path="tasks" element={<TasksPage />} />
          <Route path="tasks/new" element={<TaskEditorPage />} />
          <Route path="tasks/:slug" element={<TaskEditorPage />} />
          <Route path="schedules" element={<Placeholder title="Schedules" />} />
          <Route path="jobs" element={<Placeholder title="Jobs" />} />
          <Route path="runs" element={<Placeholder title="Runs" />} />
          <Route path="logs" element={<Placeholder title="Logs" />} />
          <Route path="settings" element={<SettingsPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </HashRouter>
  )
}