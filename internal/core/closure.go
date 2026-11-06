package core

func validateClosure(s Object, table string, row Object) {
	if row["closure"] == nil {
		return
	}
	c := obj(row["closure"], "closure")
	extras := map[string]string{"project": "", "milestones": "successor", "worksets": "results goal_acceptance", "lines": "verification delivery_record retention discard_decision successor last_release extensions", "topics": "verification memberships delivery_record integration_target delivery_scope retention successor extensions"}
	exact(c, "outcome closed_on summary evidence "+extras[table], "Invalid "+table+" closure fields")
	if has(c, "extensions") {
		extensions(c["extensions"])
	}
	switch table {
	case "project":
		enum(c["outcome"], "completed", "Project outcome")
	case "milestones":
		enum(c["outcome"], "completed cancelled superseded", "Milestone outcome")
		if c["outcome"] != "superseded" {
			need(c["successor"] == nil, "Successor only applies to supersession")
		}
	case "worksets":
		enum(c["outcome"], "completed replanned cancelled", "Iteration outcome")
		if c["outcome"] == "completed" {
			text(c["goal_acceptance"], "Achieved goal acceptance")
		} else {
			need(c["goal_acceptance"] == nil, "Goal acceptance only applies to completed iteration")
		}
	case "lines":
		outcomes := "integrated cancelled superseded"
		if row["type"] == "maintenance" {
			outcomes = "retired"
		}
		outcome := enum(c["outcome"], outcomes, "Line outcome")
		text(c["verification"], "Line verification")
		if outcome == "integrated" {
			text(c["delivery_record"], "Integration delivery record")
			for _, k := range []string{"retention", "discard_decision", "successor", "last_release"} {
				need(c[k] == nil, "Incompatible integration fields")
			}
		} else {
			need(c["delivery_record"] == nil, "Non-integrated closure cannot claim integration")
			need(truth(c["retention"]) != truth(c["discard_decision"]), "Exactly one retention record or discard decision required")
			if c["retention"] != nil {
				text(c["retention"], "Retention record")
			}
			if c["discard_decision"] != nil {
				text(c["discard_decision"], "Explicit work discard decision")
			}
			if outcome == "retired" {
				need(c["retention"] != nil && c["discard_decision"] == nil, "Maintenance history must be retained")
				text(c["last_release"], "Last release record")
			} else {
				need(c["last_release"] == nil, "Release record only applies to retirement")
			}
			if outcome != "superseded" {
				need(c["successor"] == nil, "Successor only applies to supersession")
			}
		}
	case "topics":
		text(c["verification"], "Topic verification")
		id := str(row["id"])
		lineID := relation(s, "line", id)
		line := entity(s, "lines", str(lineID))
		need(c["delivery_scope"] == deliveryScope(str(line["type"])) && c["integration_target"] == lineID, "Closure delivery scope/target mismatch")
		membership := obj(c["memberships"], "Closure memberships")
		exact(membership, "topic_id milestone_id workset_id line_id revision", "Invalid closure membership snapshot")
		need(membership["topic_id"] == id && membership["line_id"] == lineID, "Closure membership identity mismatch")
		need(equal(membership["milestone_id"], relation(s, "milestone", id)), "Closure milestone snapshot mismatch")
		need(membership["workset_id"] == nil || has(rows(s, "worksets"), str(membership["workset_id"])), "Missing closure workset")
		need(integer(membership["revision"]) && num(membership["revision"]) >= 1 && num(membership["revision"]) <= num(s["revision"]), "Invalid closure snapshot revision")
		if c["outcome"] != "superseded" {
			need(c["successor"] == nil, "Successor only applies to supersession")
		}
		if c["retention"] != nil {
			text(c["retention"], "Work disposition")
		}
	}
}
