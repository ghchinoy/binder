package okf

import (
	"slices"
	"testing"
)

func TestOrderedMapPreservesInsertionOrder(t *testing.T) {
	m := &OrderedMap{}
	m.Set("type", "Note")
	m.Set("title", "Hello")
	m.Set("custom", 1)
	if got := m.Keys(); !slices.Equal(got, []string{"type", "title", "custom"}) {
		t.Fatalf("m.Keys() = %v, want [type title custom]", got)
	}
}

func TestOrderedMapUpdateKeepsPosition(t *testing.T) {
	m := &OrderedMap{}
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("a", 3) // update, must not move to the end
	if got := m.Keys(); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("m.Keys() after update = %v, want [a b]", got)
	}
	if v, _ := m.Get("a"); v != 3 {
		t.Fatalf("m.Get(\"a\") = %v, want 3", v)
	}
}

func TestOrderedMapKeysCopy(t *testing.T) {
	m := &OrderedMap{}
	m.Set("a", 1)
	keys := m.Keys()
	keys[0] = "mutated"
	if got, _ := m.Get("a"); got != 1 {
		t.Fatalf("m.Get(\"a\") after mutating Keys() slice = %v, want 1", got)
	}
	if got := m.Keys()[0]; got != "a" {
		t.Fatalf("m.Keys()[0] after mutating copy = %q, want \"a\"", got)
	}
}

func TestOrderedMapHasAndNilSafety(t *testing.T) {
	var m *OrderedMap
	if m.Has("x") || m.Len() != 0 {
		t.Fatalf("nil OrderedMap Has(\"x\")=%v Len()=%d, want false/0", m.Has("x"), m.Len())
	}
	m = &OrderedMap{}
	if m.Has("x") {
		t.Fatal("empty OrderedMap.Has(\"x\") = true, want false")
	}
	m.Set("x", nil)
	if !m.Has("x") {
		t.Fatal("OrderedMap.Has(\"x\") with nil value = false, want true")
	}
}

func TestOrderedMapDelete(t *testing.T) {
	var nilMap *OrderedMap
	if nilMap.Delete("a") {
		t.Fatal("nil OrderedMap.Delete(\"a\") = true, want false")
	}

	m := &OrderedMap{}
	if m.Delete("missing") {
		t.Fatal("empty OrderedMap.Delete(\"missing\") = true, want false")
	}

	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("c", 3)
	m.Set("d", 4)

	if m.Delete("missing") {
		t.Fatal("m.Delete(\"missing\") = true, want false")
	}
	// Delete a middle key: relative order of remaining keys is preserved.
	if !m.Delete("b") {
		t.Fatal("m.Delete(\"b\") = false, want true")
	}
	if m.Has("b") {
		t.Fatal("m.Has(\"b\") after Delete(\"b\") = true, want false")
	}
	if _, ok := m.Get("b"); ok {
		t.Fatal("m.Get(\"b\") after Delete(\"b\") reported ok=true, want false")
	}
	if got := m.Keys(); !slices.Equal(got, []string{"a", "c", "d"}) {
		t.Fatalf("m.Keys() after Delete(\"b\") = %v, want [a c d]", got)
	}
	if got := m.Len(); got != 3 {
		t.Fatalf("m.Len() after Delete(\"b\") = %d, want 3", got)
	}

	// Re-inserting a deleted key appends it at the end as a fresh insertion.
	m.Set("b", 20)
	if got := m.Keys(); !slices.Equal(got, []string{"a", "c", "d", "b"}) {
		t.Fatalf("m.Keys() after re-Set(\"b\", 20) = %v, want [a c d b]", got)
	}

	// Delete first and last keys.
	if !m.Delete("a") || !m.Delete("b") {
		t.Fatal("m.Delete(\"a\") / m.Delete(\"b\") = false, want true")
	}
	if got := m.Keys(); !slices.Equal(got, []string{"c", "d"}) {
		t.Fatalf("m.Keys() after Delete(\"a\") and Delete(\"b\") = %v, want [c d]", got)
	}
}
