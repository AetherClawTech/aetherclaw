package agent

import (
	"sync"

	"github.com/AetherClawTech/aetherclaw/pkg/multiagent"
)

const defaultBlackboardCacheSize = 256

type blackboardEntry struct {
	board      *multiagent.Blackboard
	lastAccess uint64
}

type blackboardStore struct {
	mu      sync.Mutex
	maxSize int
	entries map[string]*blackboardEntry
	counter uint64
}

func newBlackboardStore(maxSize int) *blackboardStore {
	if maxSize <= 0 {
		maxSize = defaultBlackboardCacheSize
	}
	return &blackboardStore{
		maxSize: maxSize,
		entries: make(map[string]*blackboardEntry),
	}
}

func (s *blackboardStore) Get(key string) *multiagent.Blackboard {
	s.mu.Lock()
	defer s.mu.Unlock()

	if key == "" {
		key = "default"
	}

	if entry, ok := s.entries[key]; ok {
		entry.lastAccess = s.nextAccess()
		return entry.board
	}

	board := multiagent.NewBlackboard()
	s.entries[key] = &blackboardEntry{
		board:      board,
		lastAccess: s.nextAccess(),
	}

	if len(s.entries) > s.maxSize {
		s.evictOldest()
	}

	return board
}

func (s *blackboardStore) nextAccess() uint64 {
	s.counter++
	return s.counter
}

func (s *blackboardStore) evictOldest() {
	if len(s.entries) == 0 {
		return
	}
	var oldestKey string
	var oldestAccess uint64
	first := true
	for key, entry := range s.entries {
		if first || entry.lastAccess < oldestAccess {
			oldestKey = key
			oldestAccess = entry.lastAccess
			first = false
		}
	}
	if oldestKey != "" {
		delete(s.entries, oldestKey)
	}
}
