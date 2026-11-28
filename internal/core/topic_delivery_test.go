package core

import "testing"

func TestMilestoneRemainsOptionalForOtherTopicOutcomes(t *testing.T) {
	s, _ := numberingStep(t, nil, "project.init", Object{"id": "P-test", "name": "Test"})
	s, spike := numberingStep(t, s, "topic.create", Object{"name": "Research", "type": "spike", "slug": "research", "unassigned_reason": "Pending planning"})
	s, _ = numberingStep(t, s, "topic.state", Object{"id": spike, "status": "active"})
	s, _ = numberingStep(t, s, "topic.state", Object{"id": spike, "status": "review", "verification": "Tested"})
	s, _ = numberingStep(t, s, "topic.state", Object{"id": spike, "status": "ready", "acceptance": "Accepted"})
	s, _ = numberingStep(t, s, "topic.close", Object{"id": spike, "outcome": "research-complete", "summary": "Complete", "verification": "Tested", "delivery_record": "Report"})

	s, cancelled := numberingTopic(t, s, "Cancelled")
	s, _ = numberingStep(t, s, "topic.close", Object{"id": cancelled, "outcome": "cancelled", "summary": "Cancelled", "verification": "Inspected", "retention": "Retain documents"})

	s, superseded := numberingTopic(t, s, "Superseded")
	s, successor := numberingTopic(t, s, "Successor")
	numberingStep(t, s, "topic.close", Object{"id": superseded, "outcome": "superseded", "summary": "Superseded", "verification": "Inspected", "retention": "Retain documents", "successor": successor})
}
