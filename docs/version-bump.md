# Version Bump Guide

To bump the version, run one command:

```bash
node scripts/bump-version.js 0.5.1
```

This updates all version sources in one shot:

| File | What it controls |
|------|-----------------|
| `main.go` | Go binary version (fallback when ldflags not set) |
| `wails.json` | NSIS installer metadata |
| `Makefile` | `VERSION` for the release `-ldflags` |
| `frontend/src/lib/version.ts` | React components (imported by TopNav + About page) |
| `frontend/package.json` | npm package version |
| `frontend/package-lock.json` | lockfile version (keeps `npm ci` in sync) |

No separate steps — `make release-windows` and `npm ci` pick up the new
version automatically.

## Verify

```bash
make check              # Go tests + lint
cd frontend && npm test # Frontend tests
make release-windows    # Build installer (uses the new version)
```
