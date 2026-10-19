package notes

import (
	"encoding/json"
	"reflect"
	"testing"
)

func strPtr(s string) *string { return &s }

func TestNewNote(t *testing.T) {
	tests := []struct {
		name  string
		id    int32
		title string
		body  string
	}{
		{"basic", 1, "a", "b"},
		{"empty strings", 0, "", ""},
		{"negative id", -5, "t", "x"},
		{"max int32", 2147483647, "max", "body"},
		{"min int32", -2147483648, "min", "body"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := NewNote(tc.id, tc.title, tc.body)
			if n.ID != tc.id || n.Title != tc.title || n.Body != tc.body {
				t.Fatalf("got %+v", n)
			}
			want := Note{ID: tc.id, Title: tc.title, Body: tc.body}
			if n != want {
				t.Fatalf("positional construction mismatch: %+v vs %+v", n, want)
			}
		})
	}
}

func TestNoteEquality(t *testing.T) {
	base := NewNote(1, "a", "b")
	tests := []struct {
		name  string
		other Note
		equal bool
	}{
		{"identical", NewNote(1, "a", "b"), true},
		{"different id", NewNote(2, "a", "b"), false},
		{"different title", NewNote(1, "x", "b"), false},
		{"different body", NewNote(1, "a", "x"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if (base == tc.other) != tc.equal {
				t.Fatalf("== got %v want %v", base == tc.other, tc.equal)
			}
			// Map key usage stands in for hash-code equality.
			m := map[Note]int{base: 1}
			_, ok := m[tc.other]
			if ok != tc.equal {
				t.Fatalf("map lookup got %v want %v", ok, tc.equal)
			}
		})
	}
}

func TestNoteCopyWithModification(t *testing.T) {
	orig := NewNote(1, "a", "b")
	modified := orig
	modified.Title = "new"
	if modified.ID != 1 || modified.Title != "new" || modified.Body != "b" {
		t.Fatalf("modified wrong: %+v", modified)
	}
	if orig != NewNote(1, "a", "b") {
		t.Fatalf("original mutated: %+v", orig)
	}
}

func TestNoteValueSemanticsImmutability(t *testing.T) {
	orig := NewNote(1, "a", "b")
	mutate := func(n Note) { n.Title = "z"; n.ID = 99 }
	mutate(orig)
	if orig.Title != "a" || orig.ID != 1 {
		t.Fatalf("original mutated via copy: %+v", orig)
	}
}

func TestNoteJSONMarshal(t *testing.T) {
	tests := []struct {
		name string
		note Note
		want string
	}{
		{"basic", NewNote(1, "a", "b"), `{"id":1,"title":"a","body":"b"}`},
		{"empty", NewNote(0, "", ""), `{"id":0,"title":"","body":""}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.note)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tc.want {
				t.Fatalf("got %s want %s", b, tc.want)
			}
			var m map[string]interface{}
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatal(err)
			}
			for _, k := range []string{"id", "title", "body"} {
				if _, ok := m[k]; !ok {
					t.Fatalf("missing key %q", k)
				}
			}
			var back Note
			if err := json.Unmarshal(b, &back); err != nil || back != tc.note {
				t.Fatalf("roundtrip got %+v err %v", back, err)
			}
		})
	}
}

func eqPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func TestInputJSONUnmarshal(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		wantTitle *string
		wantBody  *string
	}{
		{"both present", `{"title":"t","body":"b"}`, strPtr("t"), strPtr("b")},
		{"case insensitive", `{"Title":"t","BODY":"b"}`, strPtr("t"), strPtr("b")},
		{"missing body", `{"title":"t"}`, strPtr("t"), nil},
		{"missing title", `{"body":"b"}`, nil, strPtr("b")},
		{"explicit null title", `{"title":null,"body":"b"}`, nil, strPtr("b")},
		{"explicit null both", `{"title":null,"body":null}`, nil, nil},
		{"empty object", `{}`, nil, nil},
		{"empty strings distinct from null", `{"title":"","body":""}`, strPtr(""), strPtr("")},
		{"unknown ignored", `{"title":"t","extra":1}`, strPtr("t"), nil},
		{"duplicate last wins", `{"title":"a","title":"z"}`, strPtr("z"), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var in Input
			if err := json.Unmarshal([]byte(tc.json), &in); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !eqPtr(in.Title, tc.wantTitle) {
				t.Errorf("Title got %v want %v", in.Title, tc.wantTitle)
			}
			if !eqPtr(in.Body, tc.wantBody) {
				t.Errorf("Body got %v want %v", in.Body, tc.wantBody)
			}
		})
	}
}

func TestNewInputAndEquality(t *testing.T) {
	tests := []struct {
		name  string
		a, b  Input
		equal bool
	}{
		{"both nil", NewInput(nil, nil), NewInput(nil, nil), true},
		{"same values", NewInput(strPtr("t"), strPtr("b")), NewInput(strPtr("t"), strPtr("b")), true},
		{"title differs", NewInput(strPtr("t"), nil), NewInput(strPtr("x"), nil), false},
		{"nil vs empty", NewInput(nil, nil), NewInput(strPtr(""), nil), false},
		{"body nil vs set", NewInput(nil, strPtr("b")), NewInput(nil, nil), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := reflect.DeepEqual(tc.a, tc.b)
			if got != tc.equal {
				t.Fatalf("equal got %v want %v", got, tc.equal)
			}
		})
	}

	title, body := strPtr("t"), strPtr("b")
	in := NewInput(title, body)
	if in.Title != title || in.Body != body {
		t.Fatalf("NewInput did not assign pointers positionally")
	}
}