#!/usr/bin/env node
// scripts/bump-version.js — single command to update (or verify) the version
// everywhere.
//
// Usage:
//   node scripts/bump-version.js <version>          # update all sources
//   node scripts/bump-version.js --check <version>  # verify, exit 1 on drift

const fs = require('fs')
const path = require('path')

const args = process.argv.slice(2)
const check = args[0] === '--check'
const newVersion = check ? args[1] : args[0]

if (!newVersion || !/^\d+\.\d+\.\d+$/.test(newVersion)) {
  console.error('Usage: node scripts/bump-version.js [--check] <version>')
  console.error('  e.g. node scripts/bump-version.js 0.5.1')
  console.error('       node scripts/bump-version.js --check 0.5.1')
  process.exit(1)
}

const root = path.resolve(__dirname, '..')

// Each entry: the exact text that must be present for a given version.
const textSources = [
  {file: 'main.go', pattern: /var appVersion = ".*"/, expected: v => `var appVersion = "${v}"`},
  {file: 'wails.json', pattern: /"productVersion": ".*"/, expected: v => `"productVersion": "${v}"`},
  {file: 'Makefile', pattern: /^VERSION\s*\?=\s*.*$/m, expected: v => `VERSION   ?= ${v}`},
  {file: 'frontend/src/lib/version.ts', pattern: /export const APP_VERSION = '.*'/, expected: v => `export const APP_VERSION = '${v}'`},
]

const ok = []
const failed = []

for (const {file, pattern, expected} of textSources) {
  const abs = path.join(root, file)
  const content = fs.readFileSync(abs, 'utf8')
  if (!pattern.test(content)) {
    console.error(`  WARN: pattern not found in ${file}`)
    failed.push(file)
    continue
  }
  if (check) {
    (content.includes(expected(newVersion)) ? ok : failed).push(file)
    continue
  }
  fs.writeFileSync(abs, content.replace(pattern, expected(newVersion)))
  console.log(`  ✓ ${file}`)
}

// package.json — version field
const pkgPath = path.join(root, 'frontend/package.json')
const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'))
if (check) {
  (pkg.version === newVersion ? ok : failed).push('frontend/package.json')
} else {
  pkg.version = newVersion
  fs.writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + '\n')
  console.log('  ✓ frontend/package.json')
}

// package-lock.json — top-level version fields (name/root package entries)
const lockPath = path.join(root, 'frontend/package-lock.json')
const lock = JSON.parse(fs.readFileSync(lockPath, 'utf8'))
if (check) {
  const rootPkg = lock.packages && lock.packages['']
  const matches = lock.version === newVersion && rootPkg && rootPkg.version === newVersion
  ;(matches ? ok : failed).push('frontend/package-lock.json')
} else {
  lock.version = newVersion
  if (lock.packages && lock.packages['']) {
    lock.packages[''].version = newVersion
  }
  fs.writeFileSync(lockPath, JSON.stringify(lock, null, 2) + '\n')
  console.log('  ✓ frontend/package-lock.json')
}

if (check) {
  for (const file of ok) console.log(`  ✓ ${file}`)
  if (failed.length > 0) {
    console.error(`\nVersion mismatch in ${failed.length} file(s):`)
    for (const file of failed) console.error(`  ✗ ${file}`)
    console.error(`\nExpected ${newVersion}. Run: node scripts/bump-version.js ${newVersion}`)
    process.exit(1)
  }
  console.log(`\nAll six version sources are ${newVersion}`)
} else {
  console.log(`\nVersion bumped to ${newVersion}`)
}
