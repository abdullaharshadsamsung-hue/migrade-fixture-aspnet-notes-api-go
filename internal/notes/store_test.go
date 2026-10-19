package notes

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"sync"
	"testing"
)

func storeStrPtr(s string) *string { return &s }

func seed(t *testing.T, s *Store, n int) []Note {
	t.Helper()
	ctx := context.Background()
	out := make([]Note, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, s.Create(ctx, "t"+string(rune('a'+i)), "b"+string(rune('a'+i))))
	}
	return out
}

func assertSortedUnique(t *testing.T, notes []Note) {
	t.Helper()
	for i := 1; i < len(notes); i++ {
		if notes[i-1].ID >= notes[i].ID {
			t.Fatalf("not strictly ascending at %d: %v", i, notes)
		}
	}
}

func equalNotes(a, b []Note) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAll(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		setup func(s *Store)
		want  []Note
	}{
		{
			name:  "empty store",
			setup: func(s *Store) {},
			want:  []Note{},
		},
		{
			name: "three notes in order",
			setup: func(s *Store) {
				s.Create(ctx, "a", "1")
				s.Create(ctx, "b", "2")
				s.Create(ctx, "c", "3")
			},
			want: []Note{{1, "a", "1"}, {2, "b", "2"}, {3, "c", "3"}},
		},
		{
			name: "deleted notes absent",
			setup: func(s *Store) {
				s.Create(ctx, "a", "1")
				s.Create(ctx, "b", "2")
				s.Create(ctx, "c", "3")
				s.Create(ctx, "d", "4")
				s.Delete(ctx, 2)
				s.Delete(ctx, 4)
			},
			want: []Note{{1, "a", "1"}, {3, "c", "3"}},
		},
		{
			name: "updated note returned",
			setup: func(s *Store) {
				s.Create(ctx, "a", "1")
				s.Create(ctx, "b", "2")
				s.Update(ctx, 2, storeStrPtr("B"), nil)
			},
			want: []Note{{1, "a", "1"}, {2, "B", "2"}},
		},
		{
			name: "many notes sorted despite map ordering",
			setup: func(s *Store) {
				for i := 0; i < 50; i++ {
					s.Create(ctx, "x", "y")
				}
			},
			want: func() []Note {
				out := make([]Note, 50)
				for i := range out {
					out[i] = Note{int32(i + 1), "x", "y"}
				}
				return out
			}(),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStore()
			tc.setup(s)
			got := s.All(ctx)
			if got == nil {
				t.Fatal("All returned nil slice")
			}
			if !equalNotes(got, tc.want) {
				t.Fatalf("All = %v, want %v", got, tc.want)
			}
			assertSortedUnique(t, got)
			again := s.All(ctx)
			if !equalNotes(got, again) {
				t.Fatalf("All modified store: %v vs %v", got, again)
			}
		})
	}
}

func TestAllEmptyEncodesAsJSONArray(t *testing.T) {
	b, err := json.Marshal(NewStore().All(context.Background()))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "[]" {
		t.Fatalf("got %s, want []", b)
	}
}

func TestAllReturnsSnapshotCopy(t *testing.T) {
	ctx := context.Background()
	s := NewStore()
	s.Create(ctx, "a", "1")
	got := s.All(ctx)
	got[0].Title = "mutated"
	n, _ := s.Get(ctx, 1)
	if n.Title != "a" {
		t.Fatalf("store mutated via All result: %v", n)
	}
}

func TestGet(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		setup  func(s *Store)
		id     int32
		want   Note
		wantOK bool
	}{
		{"existing", func(s *Store) { s.Create(ctx, "T", "B") }, 1, Note{1, "T", "B"}, true},
		{"second existing", func(s *Store) { s.Create(ctx, "a", "b"); s.Create(ctx, "c", "d") }, 2, Note{2, "c", "d"}, true},
		{"zero id", func(s *Store) { s.Create(ctx, "T", "B") }, 0, Note{}, false},
		{"negative id", func(s *Store) { s.Create(ctx, "T", "B") }, -1, Note{}, false},
		{"min int32", func(s *Store) { s.Create(ctx, "T", "B") }, math.MinInt32, Note{}, false},
		{"greater than issued", func(s *Store) { s.Create(ctx, "T", "B") }, 2, Note{}, false},
		{"max int32", func(s *Store) {}, math.MaxInt32, Note{}, false},
		{"deleted", func(s *Store) { s.Create(ctx, "T", "B"); s.Delete(ctx, 1) }, 1, Note{}, false},
		{"updated", func(s *Store) { s.Create(ctx, "T", "B"); s.Update(ctx, 1, storeStrPtr("T2"), storeStrPtr("B2")) }, 1, Note{1, "T2", "B2"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStore()
			tc.setup(s)
			before := s.All(ctx)
			got, ok := s.Get(ctx, tc.id)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("Get(%d) = %v,%v want %v,%v", tc.id, got, ok, tc.want, tc.wantOK)
			}
			if !equalNotes(before, s.All(ctx)) {
				t.Fatal("Get modified store")
			}
		})
	}
}

func TestCreate(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		setup     func(s *Store)
		title     string
		body      string
		want      Note
		wantCount int
	}{
		{"first create", func(s *Store) {}, "T", "B", Note{1, "T", "B"}, 1},
		{"sequential", func(s *Store) { s.Create(ctx, "a", "b"); s.Create(ctx, "c", "d") }, "x", "y", Note{3, "x", "y"}, 3},
		{"after delete of last no reuse", func(s *Store) { s.Create(ctx, "a", "b"); s.Create(ctx, "c", "d"); s.Delete(ctx, 2) }, "x", "y", Note{3, "x", "y"}, 2},
		{"after delete all no reuse", func(s *Store) { s.Create(ctx, "a", "b"); s.Delete(ctx, 1) }, "x", "y", Note{2, "x", "y"}, 1},
		{"duplicate values", func(s *Store) { s.Create(ctx, "T", "B") }, "T", "B", Note{2, "T", "B"}, 2},
		{"empty strings stored exactly", func(s *Store) {}, "", "", Note{1, "", ""}, 1},
		{"unicode and whitespace exact", func(s *Store) {}, "  héllo 世界 ", "line1\nline2\t", Note{1, "  héllo 世界 ", "line1\nline2\t"}, 1},
		{"after update counter unchanged", func(s *Store) {
			s.Create(ctx, "a", "b")
			s.Update(ctx, 1, storeStrPtr("z"), nil)
			s.Update(ctx, 99, storeStrPtr("z"), nil)
		}, "x", "y", Note{2, "x", "y"}, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStore()
			tc.setup(s)
			got := s.Create(ctx, tc.title, tc.body)
			if got != tc.want {
				t.Fatalf("Create = %v, want %v", got, tc.want)
			}
			stored, ok := s.Get(ctx, got.ID)
			if !ok || stored != got {
				t.Fatalf("Get(%d) = %v,%v want %v", got.ID, stored, ok, got)
			}
			all := s.All(ctx)
			if len(all) != tc.wantCount {
				t.Fatalf("len(All) = %d, want %d", len(all), tc.wantCount)
			}
			if all[len(all)-1] != got {
				t.Fatalf("All last = %v, want %v", all[len(all)-1], got)
			}
		})
	}
}

func TestCreateSequentialIDs(t *testing.T) {
	ctx := context.Background()
	s := NewStore()
	var prev int32
	for i := 0; i < 20; i++ {
		n := s.Create(ctx, "t", "b")
		if n.ID != prev+1 {
			t.Fatalf("id %d, want %d", n.ID, prev+1)
		}
		if i%3 == 0 {
			s.Delete(ctx, n.ID)
		}
		prev = n.ID
	}
}

func TestCreateConcurrent(t *testing.T) {
	ctx := context.Background()
	s := NewStore()
	const workers, per = 16, 100
	ids := make(chan int32, workers*per)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < per; i++ {
				ids <- s.Create(ctx, "t", "b").ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[int32]bool{}
	var list []int
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate id %d", id)
		}
		seen[id] = true
		list = append(list, int(id))
	}
	sort.Ints(list)
	for i, id := range list {
		if id != i+1 {
			t.Fatalf("ids not contiguous: got %d at %d", id, i)
		}
	}
	all := s.All(ctx)
	if len(all) != workers*per {
		t.Fatalf("len(All) = %d, want %d", len(all), workers*per)
	}
	assertSortedUnique(t, all)
}

func TestUpdate(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		setup  func(s *Store)
		id     int32
		title  *string
		body   *string
		want   Note
		wantOK bool
	}{
		{"both set", func(s *Store) { s.Create(ctx, "T", "B") }, 1, storeStrPtr("T2"), storeStrPtr("B2"), Note{1, "T2", "B2"}, true},
		{"title only", func(s *Store) { s.Create(ctx, "T", "B") }, 1, storeStrPtr("T2"), nil, Note{1, "T2", "B"}, true},
		{"body only", func(s *Store) { s.Create(ctx, "T", "B") }, 1, nil, storeStrPtr("B2"), Note{1, "T", "B2"}, true},
		{"both nil", func(s *Store) { s.Create(ctx, "T", "B") }, 1, nil, nil, Note{1, "T", "B"}, true},
		{"empty title", func(s *Store) { s.Create(ctx, "T", "B") }, 1, storeStrPtr(""), nil, Note{1, "", "B"}, true},
		{"empty body", func(s *Store) { s.Create(ctx, "T", "B") }, 1, nil, storeStrPtr(""), Note{1, "T", ""}, true},
		{"both empty", func(s *Store) { s.Create(ctx, "T", "B") }, 1, storeStrPtr(""), storeStrPtr(""), Note{1, "", ""}, true},
		{"non-existent", func(s *Store) { s.Create(ctx, "T", "B") }, 5, storeStrPtr("x"), storeStrPtr("y"), Note{}, false},
		{"zero id", func(s *Store) { s.Create(ctx, "T", "B") }, 0, storeStrPtr("x"), nil, Note{}, false},
		{"negative id", func(s *Store) {}, -3, nil, nil, Note{}, false},
		{"deleted", func(s *Store) { s.Create(ctx, "T", "B"); s.Delete(ctx, 1) }, 1, storeStrPtr("x"), storeStrPtr("y"), Note{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStore()
			tc.setup(s)
			before := s.All(ctx)
			got, ok := s.Update(ctx, tc.id, tc.title, tc.body)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("Update = %v,%v want %v,%v", got, ok, tc.want, tc.wantOK)
			}
			after := s.All(ctx)
			if !tc.wantOK {
				if !equalNotes(before, after) {
					t.Fatalf("store changed on failed update: %v -> %v", before, after)
				}
				if _, found := s.Get(ctx, tc.id); found {
					t.Fatal("failed update created a note")
				}
			} else {
				stored, _ := s.Get(ctx, tc.id)
				if stored != tc.want {
					t.Fatalf("stored = %v, want %v", stored, tc.want)
				}
				if len(after) != len(before) {
					t.Fatal("update changed note count")
				}
			}
			// counter not advanced
			var maxID int32
			for _, n := range before {
				if n.ID > maxID {
					maxID = n.ID
				}
			}
			next := s.Create(ctx, "n", "n")
			if len(before) > 0 && next.ID != maxID+1 && tc.name != "deleted" {
				t.Fatalf("next id %d, want %d", next.ID, maxID+1)
			}
		})
	}
}

func TestUpdateDoesNotAdvanceCounter(t *testing.T) {
	ctx := context.Background()
	s := NewStore()
	s.Create(ctx, "a", "b")
	s.Update(ctx, 1, storeStrPtr("x"), storeStrPtr("y"))
	s.Update(ctx, 42, storeStrPtr("x"), nil)
	if n := s.Create(ctx, "c", "d"); n.ID != 2 {
		t.Fatalf("id = %d, want 2", n.ID)
	}
}

func TestUpdateConcurrentNoLostWrites(t *testing.T) {
	ctx := context.Background()
	s := NewStore()
	s.Create(ctx, "T", "B")
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); s.Update(ctx, 1, storeStrPtr("title"), nil) }()
		go func() { defer wg.Done(); s.Update(ctx, 1, nil, storeStrPtr("body")) }()
	}
	wg.Wait()
	n, _ := s.Get(ctx, 1)
	if n != (Note{1, "title", "body"}) {
		t.Fatalf("got %v", n)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		setup     func(s *Store)
		ids       []int32
		want      []bool
		remaining []Note
	}{
		{"existing", func(s *Store) { s.Create(ctx, "a", "1"); s.Create(ctx, "b", "2") }, []int32{1}, []bool{true}, []Note{{2, "b", "2"}}},
		{"non-existent", func(s *Store) { s.Create(ctx, "a", "1") }, []int32{7}, []bool{false}, []Note{{1, "a", "1"}}},
		{"zero and negative", func(s *Store) { s.Create(ctx, "a", "1") }, []int32{0, -1}, []bool{false, false}, []Note{{1, "a", "1"}}},
		{"empty store", func(s *Store) {}, []int32{1}, []bool{false}, []Note{}},
		{"twice", func(s *Store) { s.Create(ctx, "a", "1"); s.Create(ctx, "b", "2") }, []int32{2, 2}, []bool{true, false}, []Note{{1, "a", "1"}}},
		{"middle only", func(s *Store) { s.Create(ctx, "a", "1"); s.Create(ctx, "b", "2"); s.Create(ctx, "c", "3") }, []int32{2}, []bool{true}, []Note{{1, "a", "1"}, {3, "c", "3"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStore()
			tc.setup(s)
			for i, id := range tc.ids {
				if got := s.Delete(ctx, id); got != tc.want[i] {
					t.Fatalf("Delete(%d) #%d = %v, want %v", id, i, got, tc.want[i])
				}
				if _, ok := s.Get(ctx, id); ok {
					t.Fatalf("Get(%d) still found after delete", id)
				}
			}
			if got := s.All(ctx); !equalNotes(got, tc.remaining) {
				t.Fatalf("All = %v, want %v", got, tc.remaining)
			}
		})
	}
}

func TestDeleteDoesNotResetCounter(t *testing.T) {
	ctx := context.Background()
	s := NewStore()
	s.Create(ctx, "a", "b")
	s.Create(ctx, "c", "d")
	s.Delete(ctx, 2)
	s.Delete(ctx, 1)
	if n := s.Create(ctx, "e", "f"); n.ID != 3 {
		t.Fatalf("id = %d, want 3", n.ID)
	}
}

func TestStoresAreIndependent(t *testing.T) {
	ctx := context.Background()
	a, b := NewStore(), NewStore()
	a.Create(ctx, "a", "1")
	a.Create(ctx, "a", "2")
	if n := b.Create(ctx, "b", "1"); n.ID != 1 {
		t.Fatalf("second store id = %d, want 1", n.ID)
	}
	if len(a.All(ctx)) != 2 || len(b.All(ctx)) != 1 {
		t.Fatal("stores share state")
	}
	if _, ok := b.Get(ctx, 2); ok {
		t.Fatal("store b sees store a's note")
	}
}

func TestKeyEqualsID(t *testing.T) {
	ctx := context.Background()
	s := NewStore()
	seed(t, s, 5)
	s.Delete(ctx, 3)
	for _, n := range s.All(ctx) {
		got, ok := s.Get(ctx, n.ID)
		if !ok || got.ID != n.ID {
			t.Fatalf("key/id mismatch for %v", n)
		}
	}
}

func TestConcurrentMixedOperations(t *testing.T) {
	ctx := context.Background()
	s := NewStore()
	seed(t, s, 10)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := int32(i%10 + 1)
			s.Create(ctx, "c", "c")
			s.Get(ctx, id)
			s.Update(ctx, id, storeStrPtr("u"), nil)
			s.All(ctx)
			if i%4 == 0 {
				s.Delete(ctx, id)
			}
		}(i)
	}
	wg.Wait()
	all := s.All(ctx)
	assertSortedUnique(t, all)
	if n := s.Create(ctx, "last", "last"); n.ID != 31 {
		t.Fatalf("id = %d, want 31", n.ID)
	}
}