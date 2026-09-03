package daemon

import (
	"encoding/json"
	"regexp"

	"heka/internal/core/task"
	"heka/internal/db"
)

// secretRefRe matches a ${KEY} vault reference anywhere in a value.
var secretRefRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// secretsUsage maps every vault key to the task slugs that reference it —
// env values, webhook URL/chat ID, and command/args are scanned (the same
// places the executor resolves ${VAR}). Used by the secrets manager page.
func secretsUsage(database *db.DB) (map[string][]string, error) {
	usage := map[string][]string{}
	tasks, err := database.Tasks().List()
	if err != nil {
		return nil, err
	}
	for _, row := range tasks {
		var t task.Task
		if err := json.Unmarshal([]byte(row.ParsedJSON), &t); err != nil {
			continue // a broken index row must not break the whole map
		}
		refs := map[string]bool{}
		scanRefs(refs, t.Command)
		for _, a := range t.Args {
			scanRefs(refs, a)
		}
		for _, v := range t.Environment {
			scanRefs(refs, v)
		}
		for _, wb := range t.Notify.Webhooks {
			scanRefs(refs, wb.URL)
			scanRefs(refs, wb.ChatID)
		}
		for key := range refs {
			usage[key] = append(usage[key], row.Slug)
		}
	}
	return usage, nil
}

func scanRefs(into map[string]bool, value string) {
	for _, m := range secretRefRe.FindAllStringSubmatch(value, -1) {
		into[m[1]] = true
	}
}
