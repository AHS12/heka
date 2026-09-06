package ipc

import (
	"testing"

	"heka/internal/db"
)

func seedSevenTasks(t *testing.T, d *db.DB) {
	t.Helper()
	for i := 0; i < 7; i++ {
		slug := string(rune('a' + i))
		seedTask(t, d, "task-"+slug, "Task "+string(rune('A'+i)), i%2 == 0)
	}
}

func TestTasksPageEnvelopeAndCursorWalk(t *testing.T) {
	database := openDB(t)
	seedSevenTasks(t, database)
	cfg := startTestServer(t, Deps{
		Health: func() Health { return Health{Core: "healthy"} },
		Tasks:  database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(), Runner: &fakeRunner{},
	})
	client := NewClient(cfg)

	// Cursor walk: 7 tasks, 3 per page.
	var got []string
	cursor := ""
	pages := 0
	for {
		res, err := client.ListTasksPage(TaskFilters{Cursor: cursor, Limit: 3})
		if err != nil {
			t.Fatal(err)
		}
		pages++
		if res.Total != 7 {
			t.Fatalf("page %d: total = %d, want 7", pages, res.Total)
		}
		for _, s := range res.Tasks {
			got = append(got, s.Slug)
		}
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
		if pages > 10 {
			t.Fatal("cursor walk did not terminate")
		}
	}
	if pages != 3 || len(got) != 7 || got[0] != "task-a" || got[6] != "task-g" {
		t.Fatalf("walk: pages = %d, slugs = %v", pages, got)
	}

	// No limit → full list in one envelope.
	res, err := client.ListTasksPage(TaskFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 7 || len(res.Tasks) != 7 || res.NextCursor != "" {
		t.Fatalf("full list: total = %d, len = %d, cursor = %q", res.Total, len(res.Tasks), res.NextCursor)
	}

	// Search with characters that need URL escaping (space, %, &).
	res, err = client.ListTasksPage(TaskFilters{Q: "task c%", Limit: 0})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 0 {
		t.Fatalf("escaped search leaked wildcards: total = %d", res.Total)
	}
	res, err = client.ListTasksPage(TaskFilters{Q: "Task C"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || res.Tasks[0].Slug != "task-c" {
		t.Fatalf("name search: total = %d", res.Total)
	}

	// Enabled filter.
	res, err = client.ListTasksPage(TaskFilters{Enabled: "enabled"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 4 {
		t.Fatalf("enabled filter total = %d, want 4", res.Total)
	}
	res, err = client.ListTasksPage(TaskFilters{Enabled: "disabled"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 {
		t.Fatalf("disabled filter total = %d, want 3", res.Total)
	}

	// Malformed cursor silently restarts from the top (runs precedent).
	res, err = client.ListTasksPage(TaskFilters{Cursor: "!!!not-base64!!!", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 7 || len(res.Tasks) != 3 || res.Tasks[0].Slug != "task-a" {
		t.Fatalf("bad cursor: total = %d, first = %q", res.Total, res.Tasks[0].Slug)
	}
}

func TestSchedulesPageEnvelopeAndKind(t *testing.T) {
	database := openDB(t)
	seedTask(t, database, "alpha", "Alpha", true)
	for i, kind := range []string{"recurring", "onetime", "recurring", "recurring"} {
		if err := database.Schedules().Save(db.Schedule{
			ID: schSeedID(i), Slug: schSeedID(i), TaskSlug: "alpha", Kind: kind,
			Enabled: true, MissedPolicy: "skip", CreatedAt: db.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	cfg := startTestServer(t, Deps{
		Health: func() Health { return Health{Core: "healthy"} },
		Tasks:  database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(), Runner: &fakeRunner{},
	})
	client := NewClient(cfg)

	// Kind filter is server-side now.
	res, err := client.ListSchedulesPage(ScheduleFilters{Kind: "recurring", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 || len(res.Schedules) != 2 || res.NextCursor == "" {
		t.Fatalf("kind page: total = %d, len = %d, cursor = %q", res.Total, len(res.Schedules), res.NextCursor)
	}
	res2, err := client.ListSchedulesPage(ScheduleFilters{Kind: "recurring", Limit: 2, Cursor: res.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Schedules) != 1 || res2.NextCursor != "" {
		t.Fatalf("kind page 2: len = %d, cursor = %q", len(res2.Schedules), res2.NextCursor)
	}

	// Legacy full-list clients still work (envelope unwrapped).
	all, err := client.ListSchedules()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("ListSchedules len = %d, want 4", len(all))
	}
	kinded, err := client.ListSchedulesFiltered("onetime")
	if err != nil {
		t.Fatal(err)
	}
	if len(kinded) != 1 || kinded[0].Kind != "onetime" {
		t.Fatalf("ListSchedulesFiltered = %+v", kinded)
	}

	// Search over slug and task_slug.
	res, err = client.ListSchedulesPage(ScheduleFilters{Q: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 4 {
		t.Fatalf("task_slug search total = %d, want 4", res.Total)
	}
}

func TestRevisionEndpoint(t *testing.T) {
	database := openDB(t)
	cfg := startTestServer(t, Deps{
		Health: func() Health { return Health{Core: "healthy"} },
		Tasks:  database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(), Runner: &fakeRunner{},
	})
	client := NewClient(cfg)

	before, err := client.Revision()
	if err != nil {
		t.Fatal(err)
	}

	seedTask(t, database, "alpha", "Alpha", true)
	after, err := client.Revision()
	if err != nil {
		t.Fatal(err)
	}
	if before.Tasks == after.Tasks {
		t.Fatal("tasks signature did not change after task insert")
	}

	// CLI-style toggle bumps the signature too (updated_at + enabled sum).
	if err := client.SetTaskEnabled("alpha", false); err != nil {
		t.Fatal(err)
	}
	afterToggle, err := client.Revision()
	if err != nil {
		t.Fatal(err)
	}
	if afterToggle.Tasks == after.Tasks {
		t.Fatal("tasks signature did not change on disable")
	}
	if afterToggle.Schedules != after.Schedules {
		t.Fatal("schedules signature moved without schedule changes")
	}
}

func schSeedID(i int) string {
	return "sched-" + string(rune('0'+i))
}
