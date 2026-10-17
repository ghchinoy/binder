package okf

import (
	"reflect"
	"testing"
)

func TestOrderedMapPreservesInsertionOrder(t *testing.T) {
	m := &OrderedMap{}
	m.Set("type", "Note")
	m.Set("title", "Hello")
	m.Set("custom", 1)
	if got := m.Keys(); !reflect.DeepEqual(got, []string{"type", "title", "custom"}) {
		t.Fatalf("unexpected key order: %v", got)
	}
}

func TestOrderedMapUpdateKeepsPosition(t *testing.T) {
	m := &OrderedMap{}
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("a", 3) // update, must not move to the end
	if got := m.Keys(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("update changed order: %v", got)
	}
	if v, _ := m.Get("a"); v != 3 {
		t.Fatalf("update lost value: %v", v)
	}
}

func TestOrderedMapKeysCopy(t *testing.T) {
	m := &OrderedMap{}
	m.Set("a", 1)
	keys := m.Keys()
	keys[0] = "mutated"
	if got, _ := m.Get("a"); got != 1 {
		t.Fatal("Keys() should return a copy that cannot corrupt the map")
	}
	if m.Keys()[0] != "a" {
		t.Fatal("Keys() copy mutation leaked into the map")
	}
}

func TestOrderedMapHasAndNilSafety(t *testing.T) {
	var m *OrderedMap
	if m.Has("x") || m.Len() != 0 {
		t.Fatal("nil OrderedMap should be safe and empty")
	}
	m = &OrderedMap{}
	if m.Has("x") {
		t.Fatal("empty map should not have key")
	}
	m.Set("x", nil)
	if !m.Has("x") {
		t.Fatal("Has should be true even for nil value")
	}
}

func TestOrderedMapDelete(t *testing.T) {
	var nilMap *OrderedMap
	if nilMap.Delete("a") {
		t.Fatal("Delete on nil OrderedMap must return false")
	}

	m := &OrderedMap{}
	if m.Delete("missing") {
		t.Fatal("Delete on empty OrderedMap must return false")
	}

	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("c", 3)
	m.Set("d", 4)

	if m.Delete("missing") {
		t.Fatal("Delete of absent key must return false")
	}
	// Delete a middle key: relative order of remaining keys is preserved.
	if !m.Delete("b") {
		t.Fatal("Delete of existing key 'b' must return true")
	}
	if m.Has("b") {
		t.Fatal("'b' must no longer be present after Delete")
	}
	if _, ok := m.Get("b"); ok {
		t.Fatal("Get('b') must report false after Delete")
	}
	if got := m.Keys(); !reflect.DeepEqual(got, []string{"a", "c", "d"}) {
		t.Fatalf("Keys() after Delete('b') = %v, want [a c d]", got)
	}
	if m.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", m.Len())
	}

	// Re-inserting a deleted key appends it at the end as a fresh insertion.
	m.Set("b", 20)
	if got := m.Keys(); !reflect.DeepEqual(got, []string{"a", "c", "d", "b"}) {
		t.Fatalf("Keys() after re-Set('b') = %v, want [a c d b]", got)
	}

	// Delete first and last keys.
	if !m.Delete("a") || !m.Delete("b") {
		t.Fatal("Delete of first/last keys must return true")
	}
	if got := m.Keys(); !reflect.DeepEqual(got, []string{"c", "d"}) {
		t.Fatalf("Keys() = %v, want [c d]", got)
	}
}
