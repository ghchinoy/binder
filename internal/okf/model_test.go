package okf

import (
	"reflect"
	"testing"
)

func TestOrderedMapPreservesInsertionOrder(t *testing.T) {
	m := NewOrderedMap()
	m.Set("type", "Note")
	m.Set("title", "Hello")
	m.Set("custom", 1)
	if got := m.Keys(); !reflect.DeepEqual(got, []string{"type", "title", "custom"}) {
		t.Fatalf("unexpected key order: %v", got)
	}
}

func TestOrderedMapUpdateKeepsPosition(t *testing.T) {
	m := NewOrderedMap()
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
	m := NewOrderedMap()
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
	m = NewOrderedMap()
	if m.Has("x") {
		t.Fatal("empty map should not have key")
	}
	m.Set("x", nil)
	if !m.Has("x") {
		t.Fatal("Has should be true even for nil value")
	}
}
