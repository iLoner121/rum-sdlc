package core

import (
	"fmt"
	"rumsdlc/internal/jsonvalue"
	"strings"
)

// Markdown renders derived data only. Free-form document bodies are never read.
func Markdown(result Object) string {
	value := func(v any) string {
		var text string
		switch v.(type) {
		case Object, []any:
			raw, _ := jsonvalue.DisplayJSON(v)
			text = string(raw)
		case nil:
			text = "None"
		case bool:
			if v == true {
				text = "True"
			} else {
				text = "False"
			}
		default:
			text = fmt.Sprint(v)
		}
		return strings.ReplaceAll(strings.ReplaceAll(text, "|", `\|`), "\n", " ")
	}
	rows, ok := result["rows"].([]any)
	if !ok {
		lines := []string{}
		for _, k := range keys(result) {
			lines = append(lines, "- "+k+": "+value(result[k]))
		}
		return strings.Join(lines, "\n") + "\n"
	}
	intro := fmt.Sprintf("Revision: %v; total: %v", result["revision"], result["total"])
	if len(rows) == 0 {
		return intro + "\n\nNo matching records.\n"
	}
	columns := []string{}
	seen := map[string]bool{}
	order := strings.Fields("id name status position participation current_workset_id topic disposition note evidence version project_id created_on closure extensions type slug dependencies integration_recheck")
	for _, v := range rows {
		row := obj(v, "row")
		for _, k := range append(order, keys(row)...) {
			if has(row, k) && !seen[k] {
				seen[k] = true
				columns = append(columns, k)
			}
		}
	}
	separators := make([]string, len(columns))
	for i := range separators {
		separators[i] = "---"
	}
	lines := []string{intro, "", "| " + strings.Join(columns, " | ") + " |", "| " + strings.Join(separators, " | ") + " |"}
	for _, v := range rows {
		row := obj(v, "row")
		cells := []string{}
		for _, k := range columns {
			if has(row, k) {
				cells = append(cells, value(row[k]))
			} else {
				cells = append(cells, "")
			}
		}
		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
	}
	return strings.Join(lines, "\n") + "\n"
}
