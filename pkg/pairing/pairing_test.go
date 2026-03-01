package pairing

import (
	"os"
	"testing"
)

func TestStore_GenerateCode(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pairing-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	code := store.GenerateCode("telegram", "user123", "chat456")
	if len(code) != 6 {
		t.Errorf("Expected 6-char code, got %d chars: %s", len(code), code)
	}

	// Same sender should get the same code
	code2 := store.GenerateCode("telegram", "user123", "chat456")
	if code2 != code {
		t.Errorf("Expected same code for same sender, got %s vs %s", code, code2)
	}

	// Different sender should get different code
	code3 := store.GenerateCode("telegram", "user789", "chat456")
	if code3 == code {
		t.Error("Expected different code for different sender")
	}
}

func TestStore_ApproveReject(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pairing-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	code := store.GenerateCode("telegram", "user1", "chat1")

	// Approve
	device, err := store.Approve(code, "cli")
	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}
	if device.SenderID != "user1" {
		t.Errorf("Expected senderID 'user1', got %s", device.SenderID)
	}

	// Should be approved now
	if !store.IsApproved("telegram", "user1") {
		t.Error("Expected user1 to be approved")
	}

	// Code should be consumed
	_, err = store.Approve(code, "cli")
	if err == nil {
		t.Error("Expected error for reused code")
	}

	// Test reject
	code2 := store.GenerateCode("discord", "user2", "chat2")
	err = store.Reject(code2)
	if err != nil {
		t.Fatalf("Reject failed: %v", err)
	}
	if store.IsApproved("discord", "user2") {
		t.Error("Expected user2 to NOT be approved after reject")
	}
}

func TestStore_Revoke(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pairing-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	code := store.GenerateCode("telegram", "user1", "chat1")
	store.Approve(code, "cli")

	if !store.IsApproved("telegram", "user1") {
		t.Fatal("Expected approved")
	}

	err = store.Revoke("telegram", "user1")
	if err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}

	if store.IsApproved("telegram", "user1") {
		t.Error("Expected revoked")
	}
}

func TestStore_Persistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pairing-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)
	code := store.GenerateCode("telegram", "user1", "chat1")
	store.Approve(code, "test")

	// Create new store and verify persistence
	store2 := NewStore(tmpDir)
	if !store2.IsApproved("telegram", "user1") {
		t.Error("Expected approval to persist across store instances")
	}
}

func TestStore_ListPending(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pairing-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)
	store.GenerateCode("telegram", "user1", "chat1")
	store.GenerateCode("discord", "user2", "chat2")

	pending := store.ListPending()
	if len(pending) != 2 {
		t.Errorf("Expected 2 pending, got %d", len(pending))
	}
}

func TestStore_ListApproved(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pairing-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)
	code1 := store.GenerateCode("telegram", "user1", "chat1")
	code2 := store.GenerateCode("discord", "user2", "chat2")
	store.Approve(code1, "cli")
	store.Approve(code2, "cli")

	approved := store.ListApproved()
	if len(approved) != 2 {
		t.Errorf("Expected 2 approved, got %d", len(approved))
	}
}
