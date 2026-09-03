// lib/secrets.ts — the vault surface: names only. useSecrets backs the
// Settings manager and the editor's "pick a secret" dropdowns.
import {useQuery} from '@tanstack/react-query'
import {listSecrets} from './api'

export const SECRETS_KEY = ['secrets'] as const

export function useSecrets() {
  return useQuery({queryKey: SECRETS_KEY, queryFn: listSecrets})
}

/** Matches a ${KEY} vault reference. */
export const SECRET_REF = /^\$\{([A-Z0-9_]+)\}$/

/** True when value is a ${KEY} reference to an existing vault key. */
export function isSecretRef(value: string, keys: string[]): keys is string[] {
  const ref = SECRET_REF.exec(value)
  return !!ref && keys.includes(ref[1])
}

/** Vault keys owned by Heka's automatic backup system — names must match
 *  internal/core/backup/config.go. They are consumed by the daemon itself,
 *  not by tasks, so the usage tracker shows them as unused. The UI badges
 *  them and warns before deletion: scheduled backups fail until the key is
 *  set up again. */
export const BACKUP_SECRET_KEYS = [
  'BACKUP_PASSPHRASE',
  'BACKUP_S3_ACCESS_KEY_ID',
  'BACKUP_S3_SECRET_ACCESS_KEY',
]

export function isBackupSecret(key: string): boolean {
  return BACKUP_SECRET_KEYS.includes(key)
}

/** Delete-warning copy for a backup-managed key. */
export function backupSecretWarning(key: string): string {
  if (key === 'BACKUP_PASSPHRASE') {
    return 'Scheduled backups will fail until you set up the passphrase again in Settings → Backup.'
  }
  return 'Mirroring to your S3 bucket will fail until the access keys are stored again in Settings → Backup.'
}