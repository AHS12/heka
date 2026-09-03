// lib/backup.ts — Settings → Backup server state: config, live status
// (polled faster while a job runs), and job history.
import {useQuery, useQueryClient, useMutation} from '@tanstack/react-query'
import {
  getBackupConfig,
  updateBackupConfig,
  getBackupStatus,
  getBackupHistory,
  runBackupNow,
  type BackupConfig,
} from './api'

export const BACKUP_CONFIG_KEY = ['backup-config'] as const
export const BACKUP_STATUS_KEY = ['backup-status'] as const
export const BACKUP_HISTORY_KEY = ['backup-history'] as const

export function useBackupConfig() {
  return useQuery({queryKey: BACKUP_CONFIG_KEY, queryFn: getBackupConfig})
}

/**
 * Live job status. Poll cadence follows the state: fast while a backup is
 * in flight, slower when idle.
 */
export function useBackupStatus() {
  return useQuery({
    queryKey: BACKUP_STATUS_KEY,
    queryFn: getBackupStatus,
    refetchInterval: (query) => (query.state.data?.running ? 2000 : 10000),
  })
}

export function useBackupHistory(limit = 20) {
  return useQuery({queryKey: [...BACKUP_HISTORY_KEY, limit], queryFn: () => getBackupHistory(limit)})
}

export function useSaveBackupConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (config: BackupConfig) => updateBackupConfig(config),
    onSuccess: () => {
      void qc.invalidateQueries({queryKey: BACKUP_CONFIG_KEY})
      void qc.invalidateQueries({queryKey: BACKUP_STATUS_KEY})
    },
  })
}

export function useRunBackup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => runBackupNow(),
    onSuccess: () => {
      void qc.invalidateQueries({queryKey: BACKUP_STATUS_KEY})
      void qc.invalidateQueries({queryKey: BACKUP_HISTORY_KEY})
    },
  })
}

/** Formats a byte count for status cards and history rows. */
export function formatBytes(n?: number): string {
  if (n == null || n <= 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = n
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return unit === 0 ? `${value} ${units[unit]}` : `${value.toFixed(1)} ${units[unit]}`
}

/** Local-time rendering for RFC 3339 timestamps; '' for absent values. */
export function formatStamp(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (isNaN(d.getTime())) return iso
  return d.toLocaleString()
}
