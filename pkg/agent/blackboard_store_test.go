package agent

import "testing"

func TestBlackboardStore_EvictsOldest(t *testing.T) {
	store := newBlackboardStore(2)
	store.Get("a")
	store.Get("b")
	store.Get("c")

	if len(store.entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(store.entries))
	}
	if _, ok := store.entries["a"]; ok {
		t.Fatalf("expected oldest key to be evicted")
	}
}

func TestBlackboardStore_UpdatesAccessOrder(t *testing.T) {
	store := newBlackboardStore(2)
	store.Get("a")
	store.Get("b")

	// Refresh "a" so "b" becomes the oldest.
	store.Get("a")
	store.Get("c")

	if _, ok := store.entries["b"]; ok {
		t.Fatalf("expected least-recently-used key to be evicted")
	}
	if _, ok := store.entries["a"]; !ok {
		t.Fatalf("expected most-recently-used key to be retained")
	}
}

func TestBlackboardStore_ReusesEntry(t *testing.T) {
	store := newBlackboardStore(2)
	board1 := store.Get("same")
	board2 := store.Get("same")
	if board1 != board2 {
		t.Fatalf("expected same board instance for same key")
	}
}
