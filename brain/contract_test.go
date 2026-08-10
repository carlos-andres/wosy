package brain

import "testing"

func TestSectionAllowed_TaskKnownAndUnknown(t *testing.T) {
	// Known per-type section.
	ok, allowed, err := SectionAllowed("task", "todo")
	if err != nil {
		t.Fatalf("SectionAllowed(task, todo): %v", err)
	}
	if !ok {
		t.Fatalf("expected todo to be allowed on task; allowed=%v", allowed)
	}
	// Known base-record section (merged in from _base.record.properties).
	ok, _, _ = SectionAllowed("task", "title")
	if !ok {
		t.Fatalf("expected base property 'title' to be allowed on task")
	}
	// Bogus section — must reject and surface a non-empty allow-list.
	ok, allowed, err = SectionAllowed("task", "fnord")
	if err != nil {
		t.Fatalf("SectionAllowed(task, fnord): %v", err)
	}
	if ok {
		t.Fatalf("expected fnord NOT allowed on task")
	}
	if len(allowed) == 0 {
		t.Fatalf("expected non-empty allow-list to report to user")
	}
}

func TestIsIDKeyed(t *testing.T) {
	// Annotated sections.
	if !IsIDKeyed("task", "todo") {
		t.Fatalf("expected task.todo to be x-id-keyed")
	}
	if !IsIDKeyed("project", "todo") {
		t.Fatalf("expected project.todo to be x-id-keyed")
	}
	if !IsIDKeyed("spec", "requirements") {
		t.Fatalf("expected spec.requirements to be x-id-keyed")
	}
	// Non-annotated sections.
	if IsIDKeyed("task", "scope") {
		t.Fatalf("scope is not x-id-keyed")
	}
	if IsIDKeyed("decision", "context") {
		t.Fatalf("decision.context is a string, not x-id-keyed")
	}
	// Unknown type / section — false, never panic.
	if IsIDKeyed("nope", "todo") {
		t.Fatalf("unknown type must report false")
	}
}
