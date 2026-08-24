# Heka — Specs

This folder is where Heka's technical design lives.

## How we work

1. **`00-master-technical-spec.md`** is the single, up-to-date technical reference for the whole project: architecture, contracts, schema, decisions. It changes only through deliberate review.
2. Work is delivered as **bite-size specs** (`01-…`, `02-…`, …), one per implementable unit. Each bite-size spec:
   - depends on and cites the master spec,
   - is small enough to implement and review in one sitting,
   - contains its own acceptance criteria,
   - is implemented **one at a time**, in order.
3. A spec is only ever in one of these states:

   ```text
   Draft → Approved → In Progress → Done
   ```

   `Approved` means the user has signed off on it. No implementation starts on a `Draft`.

## Current status

| # | Spec | State |
|---|------|-------|
| 00 | Master technical spec | Approved |
| 01 | Scaffold & tooling | Approved |
| 02 | Config & paths | Approved |
| 03 | Database | Approved |
| 04 | Task model | Approved |
| 05 | Executor | Approved |
| 06 | Daemon & health | Approved |
| 07 | IPC | Approved |
| 08 | CLI | Approved |
| 09 | Scheduler | Approved |
| 10 | Watchdog | Approved |
| 11 | Notifications & secrets | Approved |
| 12 | GUI shell | Approved |
| 13 | Tasks UI | Approved |
| 14 | Schedules/Jobs/Runs UI | Approved |
| 15 | Tray & OS startup | Approved |
| 16 | Polish & package | Approved |