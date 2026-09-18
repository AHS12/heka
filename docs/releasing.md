# Releasing Heka

Installers are built by GitHub Actions from a Git tag — there is no need to
build them by hand for a release.

## Checklist

1. **Bump the version everywhere:**
   ```bash
   node scripts/bump-version.js 0.9.0
   node scripts/bump-version.js --check 0.9.0   # verify all six sources
   ```
2. **Add a changelog entry** — a `## [0.9.0] - YYYY-MM-DD` section at the top
   of [CHANGELOG.md](../CHANGELOG.md), in a human voice.
3. **Run the quality gate:** `make check`.
4. **Commit and tag:**
   ```bash
   git add -A
   git commit -m "release: v0.9.0"
   git tag v0.9.0
   git push origin main --tags
   ```
5. **Create the GitHub Release** for the tag (UI, or
   `gh release create v0.9.0 --generate-notes`), then **publish it**. Draft
   releases do not trigger the build.
6. The **Release Installers** workflow builds and attaches:
   - `heka-0.9.0-amd64-setup.exe` — Windows installer (NSIS)
   - `heka-0.9.0.dmg` — macOS universal app (drag to Applications)

## Rebuilding an existing release

Actions → **Release Installers** → **Run workflow**, enter the release tag
(for example `v0.9.0`). The workflow checks out that tag, rebuilds, and
replaces the release's assets in place. The release must already exist.

This is the way to re-cut an artifact for a specific version without touching
the source tree.

## How the workflow works

- `.github/workflows/release.yml` runs on `release: published` and on manual
  dispatch.
- **Windows** (`windows-latest`): Go 1.25 + Node + Wails v2.15 + NSIS. Builds
  `heka-gui.exe` and the console `heka.exe`, then packs them with
  `build/windows/installer/project.nsi`.
- **macOS** (`macos-14`, pinned): Go + Node + Wails. Builds a universal
  `Heka.app`, ad-hoc signs it, and packages it with
  `build/darwin/make_dmg.sh`.
- Each build job uploads its installer straight to the release with
  `gh release upload … --clobber`, so a re-run replaces the old asset.
- The tag's version is validated against the repo's version sources before
  building, so installers can never be mislabeled.

## Version sources

`bump-version.js` updates all six: `main.go`, `wails.json`, `Makefile`,
`frontend/src/lib/version.ts`, `frontend/package.json`, and
`frontend/package-lock.json`. The macOS `Info.plist`, the DMG filename, and
the NSIS installer version all derive from these at build time — there is
nothing extra to update for a release.
