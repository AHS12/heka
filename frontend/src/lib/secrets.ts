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