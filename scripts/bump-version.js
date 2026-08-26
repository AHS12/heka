#!/usr/bin/env node
// scripts/bump-version.js — single command to update the version everywhere.
// Usage: node scripts/bump-version.js 0.2.0

const fs = require('fs')
const path = require('path')

const newVersion = process.argv[2]
if (!newVersion || !/^\d+\.\d+\.\d+/.test(newVersion)) {
  console.error('Usage: node scripts/bump-version.js <version>')
  console.error('  e.g. node scripts/bump-version.js 0.2.0')
  process.exit(1)
}

const root = path.resolve(__dirname, '..')

const files = [
  {file: 'main.go', pattern: /var appVersion = ".*"/, replace: `var appVersion = "${newVersion}"`},
  {file: 'wails.json', pattern: /"productVersion": ".*"/, replace: `"productVersion": "${newVersion}"`},
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

console.log(`\nVersion bumped to ${newVersion}`)
console.log('Next: cd frontend && npm install')
