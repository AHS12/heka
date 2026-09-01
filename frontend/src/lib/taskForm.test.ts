// taskForm tests (SPEC-13 §6.1): form↔model mapping for script + binary
// fixtures, canonical YAML emission, and rename detection.
import {describe, expect, it} from 'vitest'
import type {task} from '@wailsjs/go/models'
import {
  draftFromTask,
  draftToTask,
  draftToYAML,
  emptyDraft,
  renamePlan,
  slugify,
  validateTaskDraft,
} from './taskForm'

function scriptTask(): task.Task {
  return {
    version: 1,
    name: 'Backup',
    slug: 'backup',
    type: 'script',
    runtime: 'custom',
    script: 'backup.sh',
    args: ['--verbose', 'out=./tmp'],
    working_directory: './scripts',
    environment: {TARGET: '/data', TOKEN: '${SECRET_TOKEN}'},
    timeout: 120,
    retry: {max_attempts: 3, delay_seconds: 5},
    capture_output: true,
    notify_on: ['failure'],
    notify: {webhooks: [{format: 'slack', url: 'https://hooks/${URL}'}]},
  } as unknown as task.Task
}

function binaryTask(): task.Task {
  return {
    version: 1,
    name: 'Packer',
    slug: 'packer',
    type: 'binary',
    command: './pack',
  } as unknown as task.Task
}

describe('draft mappings', () => {
  it('round-trips a script task with advanced fields', () => {
    const src = scriptTask()
    const draft = draftFromTask(src)
    expect(draft.name).toBe('Backup')
    expect(draft.maxAttempts).toBe(3)
    expect(draft.environment).toEqual([
      ['TARGET', '/data'],
      ['TOKEN', '${SECRET_TOKEN}'],
    ])
    expect(draft.webhooks[0].chatId).toBe('')

    const back = draftToTask(draft)
    expect(back.name).toBe('Backup')
    expect(back.slug).toBe('backup')
    expect(back.script).toBe('backup.sh')
    expect(back.args).toEqual(['--verbose', 'out=./tmp'])
    expect(back.environment).toEqual({TARGET: '/data', TOKEN: '${SECRET_TOKEN}'})
    expect(back.retry).toEqual({max_attempts: 3, delay_seconds: 5})
    expect(back.notify_on).toEqual(['failure'])
  })

  it('maps a minimal binary task', () => {
    const draft = draftFromTask(binaryTask())
    expect(draft.type).toBe('binary')
    expect(draft.command).toBe('./pack')
    const back = draftToTask(draft)
    expect(back.type).toBe('binary')
    expect(back.command).toBe('./pack')
    expect(back.notify).toBeUndefined() // empty advanced fields omitted
  })

  it('omits empty advanced fields rather than emitting empties', () => {
    const model = draftToTask(emptyDraft())
    expect(model.environment).toBeUndefined()
    expect(model.args).toBeUndefined()
    expect(model.notify).toBeUndefined()
    expect(model.notify_on).toBeUndefined()
  })
})

describe('draftToYAML', () => {
  it('emits canonical field order for a populated draft', () => {
    const yaml = draftToYAML(draftFromTask(scriptTask()))
    expect(yaml).toContain('version: 1')
    expect(yaml).toContain('slug: backup')
    expect(yaml).toContain('type: script')
    expect(yaml).toContain('script: backup.sh')
    expect(yaml).toContain('args:')
    expect(yaml).toContain('  - --verbose')
    expect(yaml).toContain('TARGET: /data')
    expect(yaml).toContain('timeout: 120')
    expect(yaml).toContain('max_attempts: 3')
    expect(yaml).toContain('notify_on: [failure]')
    expect(yaml).toContain('- format: slack')
  })

  it('keeps a binary task to command + no retry block when defaults', () => {
    const yaml = draftToYAML(draftFromTask(binaryTask()))
    expect(yaml).toContain('type: binary')
    expect(yaml).toContain('command: ./pack')
    expect(yaml).not.toContain('retry:')
    expect(yaml).not.toContain('script:')
  })
})

describe('slugify', () => {
  it('turns a name into a slug usable as a filename', () => {
    expect(slugify('Nightly Backup!')).toBe('nightly-backup')
    expect(slugify('  Node   cron runner  ')).toBe('node-cron-runner')
    expect(slugify('PACK')).toBe('pack')
    expect(slugify('...')).toBe('')
  })
})

describe('validateTaskDraft', () => {
  it('returns actionable errors for an empty visual draft', () => {
    const errors = validateTaskDraft(emptyDraft())
    expect(errors).toEqual([
      'name: Enter a task name.',
      'slug: Enter a task slug.',
      'runtime: Choose a runtime.',
      'script: Choose or enter a script path.',
    ])
  })

  it('accepts a complete script draft', () => {
    expect(validateTaskDraft(draftFromTask(scriptTask()))).toEqual([])
  })
})

describe('renamePlan', () => {
  it('flags a slug change on an existing task', () => {
    const d = emptyDraft()
    d.slug = 'new-slug'
    expect(renamePlan('old-slug', d)).toEqual({
      newSlug: 'new-slug',
      isRenaming: true,
    })
    const same = emptyDraft()
    same.slug = 'same'
    expect(renamePlan('same', same)).toEqual({
      newSlug: 'same',
      isRenaming: false,
    })
  })

  it('never renames a brand-new task', () => {
    const d = emptyDraft()
    d.slug = 'fresh'
    expect(renamePlan(undefined, d)).toEqual({newSlug: 'fresh', isRenaming: false})
  })
})