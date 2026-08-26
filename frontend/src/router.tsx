// router.tsx (SPEC-12 §3) — HashRouter (embedded wails:// serving has no
// server-side history fallback) with the PRD §24 route table. Unknown paths
// redirect to Dashboard.
import {HashRouter, Navigate, Route, Routes} from 'react-router-dom'
import {AppLayout} from './layouts/AppLayout'
import {DashboardPage} from './pages/DashboardPage'
import {TasksPage} from './pages/TasksPage'
import {TaskEditorPage} from './pages/TaskEditorPage'
import {SettingsPage} from './pages/SettingsPage'
import {SchedulesPage} from './pages/SchedulesPage'
import {LogsPage} from './pages/LogsPage'
import {LogDetailPage} from './pages/RunDetailPage'
import {AboutPage} from './pages/AboutPage'

export function AppRouter() {
  return (
    <HashRouter>
      <Routes>
        <Route element={<AppLayout />}>
          <Route index element={<DashboardPage />} />
          <Route path="tasks" element={<TasksPage />} />
          <Route path="tasks/new" element={<TaskEditorPage />} />
          <Route path="tasks/:slug" element={<TaskEditorPage />} />
          <Route path="schedules" element={<SchedulesPage />} />
          <Route path="logs" element={<LogsPage />} />
          <Route path="logs/:runId" element={<LogDetailPage />} />
          <Route path="settings" element={<SettingsPage />} />
          <Route path="about" element={<AboutPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </HashRouter>
  )
}