# Changelog

All notable changes to Heka are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.6.9] - 2026-09-01

### Added
- README with project overview, features, and use cases, including images for dashboard, tasks, schedules, and settings.
- GitHub badges for downloads, stars, and latest release.

### Fixed
- Watchdog terminal flash issue on Windows.

## [0.6.5] - 2026-09-01

### Added
- Latest run ID, status, start and finish times, skipped count, and missed count to the Schedule model.
- `ListWithLatestRun` method in ScheduleStore to retrieve schedules with their latest run information.
- Search functionality on the SchedulesPage and display of latest run details.
- Tests validating new schedule behavior.

### Changed
- IPC types and conversion functions to handle the new schedule structure.

## [0.6.4] - 2026-09-01

### Changed
- Executable handling refactored for improved OS compatibility.
- OS startup registration now uses `GUIExecutable`.

## [0.6.3] - 2026-09-01

### Changed
- Enhanced daemon startup logging.

## [0.6.1] - 2026-08-28

### Added
- Sound notification settings.

## [0.6.0] - 2026-08-27

### Added
- Light and dark theme variants with specific visual styles.
- Animation utility to manage user-configurable animation preferences.
- Animation toggle in SettingsPage for user preference management.
- Sound notification settings groundwork.

### Changed
- AppLayout enhanced with scanline overlay and gradient background.
- Theme management extended to include light and dark variants.
- LogsPage uses SelectField for pagination and improved filter handling.
- TaskEditorPage uses Tabs component for better tab navigation.
- main.css updated with new theme styles and animation suppression utilities.

### Fixed
- Watchdog installation error handling on Windows for better user feedback.

### Tests
- Theme tests updated to reflect new theme variants and data attributes.
- ResizeObserver and getAnimations mocks added for HeroUI components in test setup.

## [0.5.1] - 2026-08-26

### Added
- Script to automate version bumps across multiple files.
- Tests for watchdog and startup registration functionality.

### Changed
- Windows installer handles upgrades gracefully by closing running instances.
- Watchdog and startup entry reconciliation to ensure correct paths after upgrades.
- Daemon status handling in the frontend reflects changes in real time.

### Fixed
- Improved watchdog interval handling.

## [0.5.0] - 2026-08-26

### Added
- Data directory and retention settings to the settings page.
- System tray support, OS startup registration, and installer.
- Schedules and runs management in the app.
- Comprehensive guidelines for AI agent development and styling rules.
- Backend, daemon, and frontend task module.

### Changed
- Routing refactored to replace the placeholder page with the dashboard page.
- API calls extended with `getSettings` and `updateSettings` for managing settings.
- Backend enhanced to support new settings and statistics retrieval for the dashboard.

### Fixed
- Watchdog interval handling.

### Tests
- Updated to reflect changes in routing and settings management.

## [0.1.0] - 2026-08-25

### Added
- Basic Wails skeleton and daemon foundation.
