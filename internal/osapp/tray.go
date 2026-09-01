package osapp

import (
	_ "embed"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/getlantern/systray"

	"heka/internal/config"
	"heka/internal/db"
)

//go:embed icon.png
var iconPNG []byte

// TrayDeps are everything the tray needs from the daemon (SPEC-15 §1).
type TrayDeps struct {
	Cfg        config.Config
	DB         *db.DB
	Pause      func()
	Resume     func()
	IsPaused   func() bool
	Version    string
	OnShutdown func()
}

const (
	maxTaskItems   = 10
	maxActiveItems = 5
	maxRecentItems = 5
)

// RunTray starts the system tray lifecycle on the main thread (SPEC-15 §1).
// This call blocks until the user clicks Quit.
func RunTray(deps TrayDeps) {
	systray.Run(
		func() { onReady(deps) },
		func() { _ = deps },
	)
}

func onReady(deps TrayDeps) {
	systray.SetTitle("Heka")
	systray.SetTooltip(fmt.Sprintf("Heka v%s — task scheduler", deps.Version))
	if len(iconPNG) > 0 {
		if icoBytes, err := pngToICO(iconPNG); err == nil {
			systray.SetIcon(icoBytes)
		}
	}

	// ---- Static top-level items
	openItem := systray.AddMenuItem("Open", "Open the Heka GUI")

	// ---- Run Task submenu (fixed pool of slots)
	runTaskParent := systray.AddMenuItem("Run Task", "Run a task")
	runTaskSlots := make([]*systray.MenuItem, maxTaskItems)
	for i := range runTaskSlots {
		runTaskSlots[i] = runTaskParent.AddSubMenuItem("—", "")
		runTaskSlots[i].Hide()
	}

	// ---- Active Jobs submenu
	activeParent := systray.AddMenuItem("Active Jobs", "Currently running groups")
	activeSlots := make([]*systray.MenuItem, maxActiveItems)
	for i := range activeSlots {
		activeSlots[i] = activeParent.AddSubMenuItem("—", "")
		activeSlots[i].Hide()
	}

	// ---- Recent Runs submenu
	recentParent := systray.AddMenuItem("Recent Runs", "Last 5 runs")
	recentSlots := make([]*systray.MenuItem, maxRecentItems)
	for i := range recentSlots {
		recentSlots[i] = recentParent.AddSubMenuItem("—", "")
		recentSlots[i].Hide()
	}

	systray.AddSeparator()

	// ---- Pause Scheduler
	pauseItem := systray.AddMenuItemCheckbox("Pause Scheduler", "Toggle scheduler pause", deps.IsPaused())

	systray.AddSeparator()

	// ---- Start with system
	startupRegistrar := NewStartupRegistrar()
	startupEnabled, _ := startupRegistrar.Enabled()
	startupItem := systray.AddMenuItemCheckbox("Start with system", "Register daemon for OS startup", startupEnabled)

	// ---- Watchdog guard
	watchdogInstaller := NewInstaller()
	watchdogEnabled, _, _ := watchdogInstaller.Status()
	watchdogItem := systray.AddMenuItemCheckbox("Watchdog guard", "Enable the OS watchdog", watchdogEnabled)

	systray.AddSeparator()

	// ---- Quit
	quitItem := systray.AddMenuItem("Quit", "Shut down the daemon gracefully")

	// ---- Periodic refresh (DB → menu slots)
	var mu sync.Mutex
	var taskSnapshot []db.Task

	refresh := func() {
		mu.Lock()
		defer mu.Unlock()
		refreshSlots(deps.DB, runTaskParent, runTaskSlots, maxTaskItems, func(t db.Task) (string, string) {
			label := fmt.Sprintf("%s (%s)", t.Name, lastStatusIcon(deps.DB, t.Slug))
			return label, fmt.Sprintf("Run %s", t.Slug)
		})
		allTasks, _ := deps.DB.Tasks().List()
		taskSnapshot = taskSnapshot[:0]
		for _, t := range allTasks {
			if t.Enabled {
				taskSnapshot = append(taskSnapshot, t)
			}
		}
		refreshActiveJobs(deps.DB, activeSlots)
		refreshRecentRuns(deps.DB, recentSlots)
	}
	refresh()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			<-ticker.C
			refresh()
		}
	}()

	// ---- Wire click handlers for task slots (index into snapshot)
	for idx, slot := range runTaskSlots {
		go func(s *systray.MenuItem, i int) {
			for range s.ClickedCh {
				mu.Lock()
				if i < len(taskSnapshot) {
					slug := taskSnapshot[i].Slug
					mu.Unlock()
					triggerRun(slug)
				} else {
					mu.Unlock()
				}
			}
		}(slot, idx)
	}

	// ---- Event loop
	for {
		select {
		case <-openItem.ClickedCh:
			spawnGUI(deps.Cfg)

		case <-pauseItem.ClickedCh:
			if deps.IsPaused() {
				deps.Resume()
				pauseItem.Uncheck()
			} else {
				deps.Pause()
				pauseItem.Check()
			}

		case <-startupItem.ClickedCh:
			en, _ := startupRegistrar.Enabled()
			if en {
				_ = startupRegistrar.Disable()
				startupItem.Uncheck()
			} else {
				exe, err := GUIExecutable()
				if err == nil {
					_ = startupRegistrar.Enable(exe)
					startupItem.Check()
				}
			}

		case <-watchdogItem.ClickedCh:
			en, _, _ := watchdogInstaller.Status()
			if en {
				_ = watchdogInstaller.Uninstall()
				watchdogItem.Uncheck()
			} else {
				exe, err := ConsoleExecutable()
				if err == nil {
					_ = watchdogInstaller.Install(DefaultWatchdogInterval, exe)
					watchdogItem.Check()
				}
			}

		case <-quitItem.ClickedCh:
			if deps.OnShutdown != nil {
				deps.OnShutdown()
			}
			systray.Quit()
			return
		}
	}
}

// ---- Slot refresh helpers

type slotInfo struct {
	Label   string
	Tooltip string
	Slug    string
}

func refreshSlots(
	database *db.DB,
	parent *systray.MenuItem,
	slots []*systray.MenuItem,
	max int,
	extract func(db.Task) (label, tooltip string),
) {
	tasks, err := database.Tasks().List()
	if err != nil {
		tasks = nil
	}

	enabled := make([]db.Task, 0)
	for _, t := range tasks {
		if t.Enabled {
			enabled = append(enabled, t)
		}
	}

	for i, slot := range slots {
		if i < len(enabled) {
			label, tooltip := extract(enabled[i])
			slot.SetTitle(label)
			slot.SetTooltip(tooltip)
			slot.Show()
			slot.Enable()
		} else {
			slot.Hide()
		}
	}
}

func refreshActiveJobs(database *db.DB, slots []*systray.MenuItem) {
	result, err := database.Runs().ListRuns(db.RunsFilter{Status: "running", Limit: maxActiveItems})
	if err != nil {
		result.Runs = nil
	}

	for i, slot := range slots {
		if i < len(result.Runs) {
			r := result.Runs[i]
			slot.SetTitle(fmt.Sprintf("%s", r.TaskSlug))
			slot.SetTooltip(fmt.Sprintf("Run %s", r.RunID))
			slot.Show()
			slot.Enable()
		} else {
			slot.Hide()
		}
	}
}

func refreshRecentRuns(database *db.DB, slots []*systray.MenuItem) {
	result, err := database.Runs().ListRuns(db.RunsFilter{Limit: maxRecentItems})
	if err != nil {
		result.Runs = nil
	}

	for i, slot := range slots {
		if i < len(result.Runs) {
			r := result.Runs[i]
			icon := statusIcon(r.Status)
			slot.SetTitle(fmt.Sprintf("%s %s", icon, r.TaskSlug))
			slot.SetTooltip(fmt.Sprintf("Run %s", r.RunID))
			slot.Show()
			slot.Enable()
		} else {
			slot.Hide()
		}
	}
}

func lastStatusIcon(database *db.DB, taskSlug string) string {
	rows, err := database.Runs().ListByTask(taskSlug, 1)
	if err != nil || len(rows) == 0 {
		return "○"
	}
	return statusIcon(rows[0].Status)
}

func statusIcon(status string) string {
	switch status {
	case "success":
		return "✓"
	case "running":
		return "●"
	case "failed":
		return "✗"
	case "skipped", "missed":
		return "–"
	default:
		return "○"
	}
}

// spawnGUI launches `heka gui` via the GUI executable (SPEC-15 §1).
func spawnGUI(cfg config.Config) {
	binary, err := GUIExecutable()
	if err != nil {
		return
	}
	cmd := exec.Command(binary, "gui")
	cmd.Dir = cfg.DataDir
	_ = cmd.Start()
	_ = cmd.Process.Release()
}

// triggerRun runs a task via the CLI binary (SPEC-15 §1).
func triggerRun(slug string) {
	binary, err := ConsoleExecutable()
	if err != nil {
		return
	}
	cmd := exec.Command(binary, "run", slug)
	cmd.Run()
}
