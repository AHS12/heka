package db

import (
	"testing"
)

func seedPageTasks(t *testing.T, d *DB, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		slug := "task-" + string(rune('a'+i))
		task := Task{
			ID: slug, Slug: slug, Name: "Task " + string(rune('A'+i)),
			YAMLPath: "/tasks/" + slug + ".yaml",
			ParsedJSON: `{"name":"` + slug + `","type":"script"}`,
			Enabled:    i%2 == 0, CreatedAt: Now(),
		}
		if err := d.Tasks().Save(task); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTaskListPageCursorWalk(t *testing.T) {
	d := openTest(t)
	seedPageTasks(t, d, 7)

	var got []string
	cursor := ""
	pages := 0
	for {
		res, err := d.Tasks().ListPage(TaskPageFilter{Cursor: cursor, Limit: 3})
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, tw := range res.Tasks {
			got = append(got, tw.Slug)
		}
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
		if pages > 10 {
			t.Fatal("cursor walk did not terminate")
		}
	}
	if pages != 3 || len(got) != 7 {
		t.Fatalf("pages = %d, slugs = %v", pages, got)
	}
	for i, slug := range got {
		if want := "task-" + string(rune('a'+i)); slug != want {
			t.Fatalf("slugs[%d] = %q, want %q (order broken)", i, slug, want)
		}
	}
}

func TestTaskListPageFiltersAndTotal(t *testing.T) {
	d := openTest(t)
	seedPageTasks(t, d, 6)

	// Search matches slug and name.
	res, err := d.Tasks().ListPage(TaskPageFilter{Q: "task-b", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || len(res.Tasks) != 1 || res.Tasks[0].Slug != "task-b" {
		t.Fatalf("slug search: total = %d, tasks = %+v", res.Total, res.Tasks)
	}
	res, err = d.Tasks().ListPage(TaskPageFilter{Q: "TASK-C", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || len(res.Tasks) != 1 || res.Tasks[0].Slug != "task-c" {
		t.Fatalf("name search (case-insensitive via LIKE): total = %d", res.Total)
	}

	// % and _ are literals, not wildcards.
	res, err = d.Tasks().ListPage(TaskPageFilter{Q: "%", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 0 {
		t.Fatalf("LIKE wildcard leaked: total = %d, want 0", res.Total)
	}

	// Enabled filter.
	yes := true
	no := false
	res, err = d.Tasks().ListPage(TaskPageFilter{Enabled: &yes, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 {
		t.Fatalf("enabled filter total = %d, want 3", res.Total)
	}
	res, err = d.Tasks().ListPage(TaskPageFilter{Enabled: &no, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 {
		t.Fatalf("disabled filter total = %d, want 3", res.Total)
	}

	// Type filter via parsed JSON.
	res, err = d.Tasks().ListPage(TaskPageFilter{Type: "script", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 6 {
		t.Fatalf("type filter total = %d, want 6", res.Total)
	}
	res, err = d.Tasks().ListPage(TaskPageFilter{Type: "command", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 0 {
		t.Fatalf("wrong type total = %d, want 0", res.Total)
	}

	// Total is cursor-independent: page 2 of a filtered set keeps the full count.
	res, err = d.Tasks().ListPage(TaskPageFilter{Q: "task-", Limit: 2, Cursor: encodeSlugCursor("task-b")})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 6 || len(res.Tasks) != 2 || res.Tasks[0].Slug != "task-c" {
		t.Fatalf("cursor+search: total = %d, first = %q", res.Total, res.Tasks[0].Slug)
	}
}

func TestScheduleListPageAndRevision(t *testing.T) {
	d := openTest(t)
	// schedules.task_slug has a foreign key onto tasks(slug).
	seedPageTasks(t, d, 1)
	store := d.Schedules()
	for i, kind := range []string{"recurring", "onetime", "recurring", "recurring"} {
		sch := Schedule{
			ID: schID(i), Slug: schID(i), TaskSlug: "task-a", Kind: kind,
			Enabled: true, MissedPolicy: "skip", CreatedAt: Now(),
		}
		if err := store.Save(sch); err != nil {
			t.Fatal(err)
		}
	}

	res, err := store.ListPage(SchedulePageFilter{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 4 || len(res.Schedules) != 3 || res.NextCursor == "" {
		t.Fatalf("first page: total = %d, len = %d, cursor = %q", res.Total, len(res.Schedules), res.NextCursor)
	}
	res2, err := store.ListPage(SchedulePageFilter{Limit: 3, Cursor: res.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Schedules) != 1 || res2.NextCursor != "" {
		t.Fatalf("second page: len = %d, cursor = %q", len(res2.Schedules), res2.NextCursor)
	}

	kindRes, err := store.ListPage(SchedulePageFilter{Kind: "onetime", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if kindRes.Total != 1 || len(kindRes.Schedules) != 1 {
		t.Fatalf("kind filter: total = %d", kindRes.Total)
	}
	qRes, err := store.ListPage(SchedulePageFilter{Q: "sched-1", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if qRes.Total != 1 {
		t.Fatalf("schedule search total = %d", qRes.Total)
	}
}

func schID(i int) string {
	return "sched-" + string(rune('0'+i))
}

func TestRevisions(t *testing.T) {
	d := openTest(t)

	taskRev, err := d.Tasks().Revision()
	if err != nil {
		t.Fatal(err)
	}
	schedRev, err := d.Schedules().Revision()
	if err != nil {
		t.Fatal(err)
	}
	runRev, err := d.Runs().Revision()
	if err != nil {
		t.Fatal(err)
	}

	seedPageTasks(t, d, 3)
	taskRev2, _ := d.Tasks().Revision()
	if taskRev2 == taskRev {
		t.Fatal("task revision did not change after insert")
	}
	// Toggle a disabled task to enabled: SUM(enabled) changes deterministically
	// even though updated_at is second-precision.
	if err := d.Tasks().SetEnabled("task-b", true); err != nil {
		t.Fatal(err)
	}
	taskRev3, _ := d.Tasks().Revision()
	if taskRev3 == taskRev2 {
		t.Fatal("task revision did not change on enable toggle")
	}
	if sr, _ := d.Schedules().Revision(); sr != schedRev {
		t.Fatal("schedule revision changed without schedule changes")
	}
	if rr, _ := d.Runs().Revision(); rr != runRev {
		t.Fatal("run revision changed without run changes")
	}

	if err := d.Schedules().Save(Schedule{
		ID: "s1", Slug: "s1", TaskSlug: "task-a", Kind: "recurring",
		Enabled: true, MissedPolicy: "skip", CreatedAt: Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if sr, _ := d.Schedules().Revision(); sr == schedRev {
		t.Fatal("schedule revision did not change after insert")
	}

	run := Run{
		RunID: "01JZZZZZZZZZZZZZZZZZ0RUN1", GroupID: "g1", Attempt: 1,
		TaskSlug: "task-a", Trigger: "manual", Status: "running", CreatedAt: Now(),
	}
	if err := d.Runs().Create(run); err != nil {
		t.Fatal(err)
	}
	rr2, _ := d.Runs().Revision()
	if rr2 == runRev {
		t.Fatal("run revision did not change when a run started")
	}
	run.Status = "success"
	run.FinishedAt = new(string)
	if err := d.Runs().Update(run); err != nil {
		t.Fatal(err)
	}
	rr3, _ := d.Runs().Revision()
	if rr3 == rr2 {
		t.Fatal("run revision did not change when a run finished in place")
	}
}
