import {describe, expect, it} from 'vitest'
import {compareVersions, parseChangelog} from './changelog'

const SAMPLE = [
  '# Changelog',
  '',
  'All notable changes to Heka are documented in this file.',
  '',
  '## [0.8.2] - 2026-09-05',
  '',
  'The schedule creator grows out of its dropdowns.',
  '',
  '### Added',
  '- Pattern-based schedule builder.',
  '',
  '### Changed',
  '- Cards describe rules in plain language.',
  '',
  '## [0.8.1] - 2026-09-04',
  '',
  'A correctness pass.',
  '',
  '### Fixed',
  '- Webhook notifications name the task.',
  '',
  '## [Unreleased]',
  '',
  'Draft material.',
  '',
  '## [broken] - 2026-01-01',
  '',
  'Not a semantic version.',
].join('\n')

describe('parseChangelog', () => {
  it('parses releases newest first, skipping preamble, Unreleased, and non-semver', () => {
    const releases = parseChangelog(SAMPLE)
    expect(releases.map((r) => r.version)).toEqual(['0.8.2', '0.8.1'])
    expect(releases[0].date).toBe('2026-09-05')
  })

  it('splits the prose summary from the ### sections', () => {
    const [first, second] = parseChangelog(SAMPLE)
    expect(first.summary).toBe('The schedule creator grows out of its dropdowns.')
    expect(first.body).toContain('### Added')
    expect(first.body).toContain('- Pattern-based schedule builder.')
    expect(first.body).not.toContain('### Changed\n\n- Pattern')
    expect(second.summary).toBe('A correctness pass.')
  })

  it('handles CRLF line endings', () => {
    const releases = parseChangelog(SAMPLE.replace(/\n/g, '\r\n'))
    expect(releases.map((r) => r.version)).toEqual(['0.8.2', '0.8.1'])
  })

  it('returns [] for empty or unparseable input', () => {
    expect(parseChangelog('')).toEqual([])
    expect(parseChangelog('# just a title\n')).toEqual([])
  })
})

describe('compareVersions', () => {
  it('compares numerically per part', () => {
    expect(compareVersions('0.10.0', '0.9.1')).toBeGreaterThan(0)
    expect(compareVersions('0.8.2', '0.8.1')).toBeGreaterThan(0)
    expect(compareVersions('1.0.0', '1.0.0')).toBe(0)
    expect(compareVersions('0.8.1', '0.8.2')).toBeLessThan(0)
  })

  it('treats missing or non-numeric parts as 0', () => {
    expect(compareVersions('1.0', '1.0.0')).toBe(0)
    expect(compareVersions('1.0.0-beta', '1.0.0')).toBe(0)
  })
})
