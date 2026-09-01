#!/usr/bin/env node
// scripts/bump-version.js — single command to update the version everywhere.
// Usage: node scripts/bump-version.js 0.5.1

const fs = require('fs')
const path = require('path')

const newVersion = process.argv[2]
if (!newVersion || !/^\d+\.\d+\.\d+$/.test(newVersion)) {
  console.error('Usage: node scripts/bump-version.js <version>')
  console.error('  e.g. node scripts/bump-version.js 0.5.1')
  process.exit(1)
}

const root = path.resolve(__dirname, '..')
const semver = newVersion.replace(/\./g, '\\.')

const files = [
  {file: 'main.go', pattern: /var appVersion = ".*"/, replace: `var appVersion = "${newVersion}"`},
  {file: 'wails.json', pattern: /"productVersion": ".*"/, replace: `"productVersion": "${newVersion}"`},
  {file: 'Makefile', pattern: /^VERSION\s*\?=\s*.*$/m, replace: `VERSION   ?= ${newVersion}`},
  {file: 'frontend/src/lib/version.ts', pattern: /export const APP_VERSION = '.*'/, replace: `export const APP_VERSION = '${newVersion}'`},
]

for (const {file, pattern, replace} of files) {
  const abs = path.join(root, file)
  const content = fs.readFileSync(abs, 'utf8')
  if (!pattern.test(content)) {
    console.error(`  WARN: pattern not found in ${file}`)
    continue
  }
  fs.writeFileSync(abs, content.replace(pattern, replace))
  console.log(`  ✓ ${file}`)
}

// package.json — update version field
const pkgPath = path.join(root, 'frontend/package.json')
const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'))
pkg.version = newVersion
fs.writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + '\n')
console.log('  ✓ frontend/package.json')

// package-lock.json — top-level version fields (name/root package entries)
const lockPath = path.join(root, 'frontend/package-lock.json')
const lock = JSON.parse(fs.readFileSync(lockPath, 'utf8'))
lock.version = newVersion
if (lock.packages && lock.packages['']) {
  lock.packages[''].version = newVersion
}
fs.writeFileSync(lockPath, JSON.stringify(lock, null, 2) + '\n')
console.log('  ✓ frontend/package-lock.json')

console.log(`\nVersion bumped to ${newVersion}`)
console.log(`Verify: grep -rn "${semver}" --include="*.go" --include="*.json" --include="*.ts" --include="Makefile" .`)
