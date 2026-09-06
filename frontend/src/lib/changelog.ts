// lib/changelog.ts — parse the embedded CHANGELOG.md (Keep a Changelog
// format: `## [x.y.z] - date` headings) into structured releases. Pure
// functions only, so the parsing is directly unit-testable.

export interface Release {
  version: string
  date: string
  /** Prose paragraph(s) between the heading and the first `###` section. */
  summary: string
  /** The `###` sections (Added/Changed/Fixed…) as raw markdown. */
  body: string
}

const HEADING_RE = /^##\s*\[(.+?)\]\s*-\s*(.+?)\s*$/
const SEMVER_RE = /^\d+\.\d+\.\d+$/

/** Splits the changelog into releases, newest first. Non-semver sections
 *  (`[Unreleased]`) and the preamble before the first heading are dropped. */
export function parseChangelog(md: string): Release[] {
  const releases: {version: string; date: string; lines: string[]}[] = []
  let current: {version: string; date: string; lines: string[]} | null = null
  for (const line of md.split(/\r?\n/)) {
    const m = HEADING_RE.exec(line)
    if (m) {
      if (current) releases.push(current)
      current = {version: m[1].trim(), date: m[2].trim(), lines: []}
    } else if (current) {
      current.lines.push(line)
    }
  }
  if (current) releases.push(current)
  return releases
    .filter((r) => SEMVER_RE.test(r.version))
    .map(splitSummary)
    .sort((a, b) => compareVersions(b.version, a.version))
}

function splitSummary(r: {version: string; date: string; lines: string[]}): Release {
  const text = r.lines.join('\n').trim()
  const firstSection = text.search(/^###\s/m)
  return {
    version: r.version,
    date: r.date,
    summary: (firstSection >= 0 ? text.slice(0, firstSection) : text).trim(),
    body: firstSection >= 0 ? text.slice(firstSection).trim() : '',
  }
}

/** Three-part numeric compare ("0.10.0" > "0.9.1"); missing or non-numeric
 *  parts count as 0. */
export function compareVersions(a: string, b: string): number {
  const pa = a.split('.').map(partNumber)
  const pb = b.split('.').map(partNumber)
  for (let i = 0; i < 3; i++) {
    const x = pa[i] ?? 0
    const y = pb[i] ?? 0
    if (x !== y) return x - y
  }
  return 0
}

function partNumber(part: string): number {
  const n = Number(part)
  return Number.isFinite(n) ? n : 0
}
