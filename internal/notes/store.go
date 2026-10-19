package notes

import (
	"context"
	"sort"
	"sync"
)

// Store is an in-memory, concurrency-safe repository of Note values.
//
// It replaces the C# NoteStore (ConcurrentDictionary + Interlocked.Increment).
// A single Store is intended to be created at startup and shared by all
// request handlers, mirroring the singleton DI registration in the source.
// The zero value is not usable; construct one with NewStore.
type Store struct {
	mu     sync.RWMutex
	notes  map[int32]Note
	nextID int32
}

// NewStore returns an empty Store. The first created note receives ID 1.
func NewStore() *Store {
	return &Store{notes: make(map[int32]Note)}
}

// All returns every note currently stored, ordered by ID ascending.
//
// The returned slice is a snapshot copy and is never nil, so it encodes to
// JSON as [] rather than null when the store is empty.
func (s *Store) All(_ context.Context) []Note {
	s.mu.RLock()
	out := make([]Note, 0, len(s.notes))
	for _, n := range s.notes {
		out = append(out, n)
	}
	s.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get returns the note with the given ID. The boolean is false when no such
// note exists (the C# version returned null).
func (s *Store) Get(_ context.Context, id int32) (Note, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.notes[id]
	return n, ok
}

// Create stores a new note with the next sequential ID and returns it.
// IDs start at 1 and are never reused, even after a delete.
//
// MIGRATION_NOTE: C#'s Interlocked.Increment wraps on int overflow; that
// edge case (more than 2^31-1 creations) is intentionally not replicated.
func (s *Store) Create(_ context.Context, title, body string) Note {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	n := NewNote(s.nextID, title, body)
	s.notes[n.ID] = n
	return n
}

// Update partially updates an existing note. A nil title or body keeps the
// existing value; a non-nil pointer (including one to "") replaces it.
// The boolean is false when no note with the given ID exists.
//
// The lock is held across the read-modify-write, so concurrent updates
// cannot lose each other's changes (stricter than the C# original).
func (s *Store) Update(_ context.Context, id int32, title, body *string) (Note, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.notes[id]
	if !ok {
		return Note{}, false
	}
	if title != nil {
		n.Title = *title
	}
	if body != nil {
		n.Body = *body
	}
	s.notes[id] = n
	return n, true
}

// Delete removes the note with the given ID and reports whether a note was
// actually removed.
func (s *Store) Delete(_ context.Context, id int32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.notes[id]; !ok {
		return false
	}
	delete(s.notes, id)
	return true
}
