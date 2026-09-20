package okf

import "testing"

func TestIsValidActor(t *testing.T) {
	valid := []string{"human:alice", "process:nightly", "team:finance", "reference_agent/gemini-2.5-pro", "binder/0.1.0"}
	for _, a := range valid {
		if !IsValidActor(a) {
			t.Errorf("IsValidActor(%q) = false, want true", a)
		}
	}
	invalid := []string{"", "human:", "alice", "just text", "no space/ok but has space", "/leading", "trailing/"}
	for _, a := range invalid {
		if IsValidActor(a) {
			t.Errorf("IsValidActor(%q) = true, want false", a)
		}
	}
}

func TestIsValidISODates(t *testing.T) {
	if !IsValidISODate("2026-12-31") {
		t.Error("2026-12-31 should be a valid date")
	}
	if IsValidISODate("2026-13-40") || IsValidISODate("not-a-date") {
		t.Error("bad dates should be invalid")
	}
}
