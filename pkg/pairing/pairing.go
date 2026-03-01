package pairing

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PairRequest represents a pending pairing request.
type PairRequest struct {
	Code      string    `json:"code"`
	Channel   string    `json:"channel"`
	SenderID  string    `json:"sender_id"`
	ChatID    string    `json:"chat_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ApprovedDevice represents an approved sender.
type ApprovedDevice struct {
	Channel    string    `json:"channel"`
	SenderID   string    `json:"sender_id"`
	Label      string    `json:"label,omitempty"`
	ApprovedAt time.Time `json:"approved_at"`
	ApprovedBy string    `json:"approved_by,omitempty"` // "cli", "channel", "auto"
}

// Store manages device pairing state.
type Store struct {
	dir      string
	mu       sync.RWMutex
	pending  map[string]*PairRequest    // code -> request
	approved map[string]*ApprovedDevice // "channel:senderID" -> device
}

// NewStore creates a pairing store in the given directory.
func NewStore(workspaceDir string) *Store {
	dir := filepath.Join(workspaceDir, "pairing")
	os.MkdirAll(dir, 0o755)

	s := &Store{
		dir:      dir,
		pending:  make(map[string]*PairRequest),
		approved: make(map[string]*ApprovedDevice),
	}

	s.load()
	return s
}

// GenerateCode creates a pairing code for an unknown sender.
func (s *Store) GenerateCode(channel, senderID, chatID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already pending
	key := fmt.Sprintf("%s:%s", channel, senderID)
	for code, req := range s.pending {
		if fmt.Sprintf("%s:%s", req.Channel, req.SenderID) == key {
			if time.Now().Before(req.ExpiresAt) {
				return code
			}
			delete(s.pending, code)
		}
	}

	code := generateRandomCode()
	s.pending[code] = &PairRequest{
		Code:      code,
		Channel:   channel,
		SenderID:  senderID,
		ChatID:    chatID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	s.saveLocked()
	return code
}

// Approve approves a pending pairing request by code.
func (s *Store) Approve(code, approvedBy string) (*ApprovedDevice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, ok := s.pending[code]
	if !ok {
		return nil, fmt.Errorf("pairing code not found: %s", code)
	}

	if time.Now().After(req.ExpiresAt) {
		delete(s.pending, code)
		s.saveLocked()
		return nil, fmt.Errorf("pairing code expired")
	}

	key := fmt.Sprintf("%s:%s", req.Channel, req.SenderID)
	device := &ApprovedDevice{
		Channel:    req.Channel,
		SenderID:   req.SenderID,
		ApprovedAt: time.Now(),
		ApprovedBy: approvedBy,
	}

	s.approved[key] = device
	delete(s.pending, code)
	s.saveLocked()

	return device, nil
}

// Reject removes a pending pairing request.
func (s *Store) Reject(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.pending[code]; !ok {
		return fmt.Errorf("pairing code not found: %s", code)
	}

	delete(s.pending, code)
	s.saveLocked()
	return nil
}

// IsApproved checks if a sender is approved on a channel.
func (s *Store) IsApproved(channel, senderID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", channel, senderID)
	_, ok := s.approved[key]
	return ok
}

// Revoke removes an approved device.
func (s *Store) Revoke(channel, senderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%s", channel, senderID)
	if _, ok := s.approved[key]; !ok {
		return fmt.Errorf("device not found: %s", key)
	}

	delete(s.approved, key)
	s.saveLocked()
	return nil
}

// ListPending returns all pending pairing requests.
func (s *Store) ListPending() []*PairRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*PairRequest, 0, len(s.pending))
	now := time.Now()
	for _, req := range s.pending {
		if now.Before(req.ExpiresAt) {
			result = append(result, req)
		}
	}
	return result
}

// ListApproved returns all approved devices.
func (s *Store) ListApproved() []*ApprovedDevice {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*ApprovedDevice, 0, len(s.approved))
	for _, device := range s.approved {
		result = append(result, device)
	}
	return result
}

// CleanExpired removes expired pending requests.
func (s *Store) CleanExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	now := time.Now()
	for code, req := range s.pending {
		if now.After(req.ExpiresAt) {
			delete(s.pending, code)
			count++
		}
	}

	if count > 0 {
		s.saveLocked()
	}
	return count
}

// Internal persistence

type storeData struct {
	Pending  map[string]*PairRequest    `json:"pending"`
	Approved map[string]*ApprovedDevice `json:"approved"`
}

func (s *Store) load() {
	path := filepath.Join(s.dir, "store.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var sd storeData
	if err := json.Unmarshal(data, &sd); err != nil {
		return
	}

	if sd.Pending != nil {
		s.pending = sd.Pending
	}
	if sd.Approved != nil {
		s.approved = sd.Approved
	}
}

func (s *Store) saveLocked() {
	sd := storeData{
		Pending:  s.pending,
		Approved: s.approved,
	}

	data, err := json.MarshalIndent(sd, "", "  ")
	if err != nil {
		return
	}

	path := filepath.Join(s.dir, "store.json")
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return
	}
	os.Rename(tmpPath, path)
}

func generateRandomCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	code := make([]byte, 6)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			// Fallback to simple counter
			code[i] = chars[i%len(chars)]
			continue
		}
		code[i] = chars[n.Int64()]
	}
	return string(code)
}
