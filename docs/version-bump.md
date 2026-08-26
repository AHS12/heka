# Version Bump Guide

To bump the version, run one command:

```bash
node scripts/bump-version.js 0.2.0
cd frontend && npm install
```

This updates all version sources in one shot:

| File | What it controls |
|------|-----------------|
| `main.go` | Go binary version (fallback when ldflags not set) |
| `wails.json` | NSIS installer metadata |
| `frontend/src/lib/version.ts` | React components (imported by TopNav + About page) |
| `frontend/package.json` | npm package version |

The `Makefile` `VERSION` variable is **not** updated by the script — it controls
the `-ldflags` value for release builds. Update it separately if needed:

```makefile
VERSION ?= 0.2.0
```

## Verify

```bash
make check          # Go tests + lint
cd frontend && npm test   # Frontend tests
make release-windows      # Build installer
```
