// components/tasks/SecretValue.tsx — the "pick a secret from the vault"
// dropdown (SPEC-11 surface for task fields). Values are always stored as
// ${KEY} references. The dropdown is disabled while the vault has no keys.
import {SelectField} from '../controls'
import {useSecrets} from '../../lib/secrets'

export function SecretValue({
  value,
  onChange,
  ariaLabel,
}: {
  value: string
  onChange: (value: string) => void
  ariaLabel: string
}) {
  const secrets = useSecrets()
  const keys = secrets.data ?? []
  const selected = value.startsWith('${') ? value.slice(2, -1) : ''

  return (
    <SelectField
      aria-label={`${ariaLabel} source`}
      value={selected}
      onChange={(next) => onChange(`\${${next}}`)}
      className="w-full"
      placeholder="Select a secret…"
      items={keys.map((k) => ({id: k, label: `\${${k}}`}))}
    />
  )
}