package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"migrated-app/internal/notes"
)

type noteJSON struct {
	ID    int32  `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// spyStore wraps a real store and counts mutating calls.
type spyStore struct {
	inner   *notes.Store
	creates int
	updates int
	deletes int
}

func newSpy() *spyStore { return &spyStore{inner: notes.NewStore()} }

func (s *spyStore) All(ctx context.Context) []notes.Note { return s.inner.All(ctx) }
func (s *spyStore) Get(ctx context.Context, id int32) (notes.Note, bool) {
	return s.inner.Get(ctx, id)
}
func (s *spyStore) Create(ctx context.Context, title, body string) notes.Note {
	s.creates++
	return s.inner.Create(ctx, title, body)
}
func (s *spyStore) Update(ctx context.Context, id int32, title, body *string) (notes.Note, bool) {
	s.updates++
	return s.inner.Update(ctx, id, title, body)
}
func (s *spyStore) Delete(ctx context.Context, id int32) bool {
	s.deletes++
	return s.inner.Delete(ctx, id)
}

func newRouter(store NoteStore) http.Handler {
	r := chi.NewRouter()
	NewHandler(store).Routes(r)
	return r
}

const jsonCT = "application/json"

func do(t *testing.T, h http.Handler, method, path, body, ct string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeNote(t *testing.T, rec *httptest.ResponseRecorder) noteJSON {
	t.Helper()
	var n noteJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &n); err != nil {
		t.Fatalf("decode note: %v (body %q)", err, rec.Body.String())
	}
	return n
}

func decodeList(t *testing.T, rec *httptest.ResponseRecorder) []noteJSON {
	t.Helper()
	var l []noteJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &l); err != nil {
		t.Fatalf("decode list: %v (body %q)", err, rec.Body.String())
	}
	return l
}

func assertError(t *testing.T, rec *httptest.ResponseRecorder, status int, msg string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (body %q)", rec.Code, status, rec.Body.String())
	}
	var e map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode error body: %v (%q)", err, rec.Body.String())
	}
	if len(e) != 1 || e["error"] != msg {
		t.Fatalf("error body = %v, want {error:%q}", e, msg)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type = %q", ct)
	}
}

func seed(t *testing.T, h http.Handler, title, body string) noteJSON {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"title": title, "body": body})
	rec := do(t, h, http.MethodPost, "/notes", string(b), jsonCT)
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed: status %d", rec.Code)
	}
	return decodeNote(t, rec)
}

func TestHealth(t *testing.T) {
	spy := newSpy()
	h := newRouter(spy)
	seed(t, h, "x", "y")
	before := spy.creates
	for i := 0; i < 2; i++ {
		rec := do(t, h, http.MethodGet, "/health", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var m map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		if len(m) != 1 || m["status"] != "ok" {
			t.Fatalf("body = %v", m)
		}
	}
	if spy.creates != before || spy.updates != 0 || spy.deletes != 0 {
		t.Fatal("health modified store")
	}
	if len(spy.All(context.Background())) != 1 {
		t.Fatal("store changed")
	}
}

func TestListNotes(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		h := newRouter(newSpy())
		rec := do(t, h, http.MethodGet, "/notes", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
			t.Fatalf("body = %q, want []", got)
		}
	})

	t.Run("after create, update, delete", func(t *testing.T) {
		spy := newSpy()
		h := newRouter(spy)
		a := seed(t, h, "a", "A")
		b := seed(t, h, "b", "B")
		c := seed(t, h, "c", "C")
		if rec := do(t, h, http.MethodDelete, "/notes/"+strconv.Itoa(int(b.ID)), "", ""); rec.Code != http.StatusNoContent {
			t.Fatalf("delete status %d", rec.Code)
		}
		if rec := do(t, h, http.MethodPut, "/notes/"+strconv.Itoa(int(c.ID)), `{"title":"c2"}`, jsonCT); rec.Code != http.StatusOK {
			t.Fatalf("update status %d", rec.Code)
		}
		mutations := spy.creates + spy.updates + spy.deletes

		rec := do(t, h, http.MethodGet, "/notes", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		list := decodeList(t, rec)
		want := map[int32]noteJSON{
			a.ID: {a.ID, "a", "A"},
			c.ID: {c.ID, "c2", "C"},
		}
		if len(list) != len(want) {
			t.Fatalf("list = %+v", list)
		}
		for _, n := range list {
			if want[n.ID] != n {
				t.Fatalf("note %+v, want %+v", n, want[n.ID])
			}
			g := do(t, h, http.MethodGet, "/notes/"+strconv.Itoa(int(n.ID)), "", "")
			if g.Code != http.StatusOK || decodeNote(t, g) != n {
				t.Fatalf("GET of listed note %d failed", n.ID)
			}
		}
		if spy.creates+spy.updates+spy.deletes != mutations {
			t.Fatal("list mutated store")
		}
	})
}

func TestGetNote(t *testing.T) {
	h := newRouter(newSpy())
	n := seed(t, h, "t", "b")
	gone := seed(t, h, "g", "")
	do(t, h, http.MethodDelete, "/notes/"+strconv.Itoa(int(gone.ID)), "", "")

	tests := []struct {
		name      string
		path      string
		status    int
		wantNote  *noteJSON
		wantError bool
		emptyBody bool
	}{
		{"existing", "/notes/" + strconv.Itoa(int(n.ID)), 200, &n, false, false},
		{"leading plus sign", "/notes/+" + strconv.Itoa(int(n.ID)), 200, &n, false, false},
		{"unknown", "/notes/999", 404, nil, true, false},
		{"deleted", "/notes/" + strconv.Itoa(int(gone.ID)), 404, nil, true, false},
		{"negative unknown", "/notes/-5", 404, nil, true, false},
		{"non-integer", "/notes/abc", 404, nil, false, true},
		{"decimal", "/notes/1.5", 404, nil, false, true},
		{"overflow int32", "/notes/99999999999", 404, nil, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, tc.path, "", "")
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
			if tc.wantNote != nil && decodeNote(t, rec) != *tc.wantNote {
				t.Fatalf("note mismatch: %s", rec.Body.String())
			}
			if tc.wantError {
				assertError(t, rec, 404, "not found")
			}
			if tc.emptyBody && rec.Body.Len() != 0 {
				t.Fatalf("expected empty body, got %q", rec.Body.String())
			}
		})
	}
}

func TestCreateNote(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		ct        string
		status    int
		errMsg    string
		wantTitle string
		wantBody  string
	}{
		{"valid", `{"title":"Hello","body":"World"}`, jsonCT, 201, "", "Hello", "World"},
		{"body omitted", `{"title":"Hello"}`, jsonCT, 201, "", "Hello", ""},
		{"body null", `{"title":"Hello","body":null}`, jsonCT, 201, "", "Hello", ""},
		{"charset param", `{"title":"T"}`, "application/json; charset=utf-8", 201, "", "T", ""},
		{"+json type", `{"title":"T"}`, "application/merge-patch+json", 201, "", "T", ""},
		{"surrounding whitespace", "  \n{\"title\":\"T\"}\n ", jsonCT, 201, "", "T", ""},
		{"title missing", `{"body":"x"}`, jsonCT, 400, "title is required", "", ""},
		{"title null", `{"title":null}`, jsonCT, 400, "title is required", "", ""},
		{"title empty", `{"title":""}`, jsonCT, 400, "title is required", "", ""},
		{"title whitespace", `{"title":" \t\n"}`, jsonCT, 400, "title is required", "", ""},
		{"empty object", `{}`, jsonCT, 400, "title is required", "", ""},
		{"empty body", ``, jsonCT, 400, "", "", ""},
		{"whitespace body", `   `, jsonCT, 400, "", "", ""},
		{"literal null", `null`, jsonCT, 400, "", "", ""},
		{"array root", `[]`, jsonCT, 400, "", "", ""},
		{"malformed", `{"title":`, jsonCT, 400, "", "", ""},
		{"wrong type", `{"title":5}`, jsonCT, 400, "", "", ""},
		{"trailing data", `{"title":"a"} x`, jsonCT, 400, "", "", ""},
		{"missing content type", `{"title":"a"}`, "", 415, "", "", ""},
		{"text/plain", `{"title":"a"}`, "text/plain", 415, "", "", ""},
		{"bad content type", `{"title":"a"}`, ";;;", 415, "", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spy := newSpy()
			h := newRouter(spy)
			rec := do(t, h, http.MethodPost, "/notes", tc.body, tc.ct)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d (%q)", rec.Code, tc.status, rec.Body.String())
			}
			if tc.status == 201 {
				n := decodeNote(t, rec)
				if n.Title != tc.wantTitle || n.Body != tc.wantBody || n.ID == 0 {
					t.Fatalf("note = %+v", n)
				}
				if loc := rec.Header().Get("Location"); loc != "/notes/"+strconv.Itoa(int(n.ID)) {
					t.Fatalf("Location = %q", loc)
				}
				g := do(t, h, http.MethodGet, "/notes/"+strconv.Itoa(int(n.ID)), "", "")
				if g.Code != 200 || decodeNote(t, g) != n {
					t.Fatal("created note not retrievable")
				}
				if l := decodeList(t, do(t, h, http.MethodGet, "/notes", "", "")); len(l) != 1 || l[0] != n {
					t.Fatalf("list = %+v", l)
				}
				return
			}
			if tc.errMsg != "" {
				assertError(t, rec, tc.status, tc.errMsg)
			} else if rec.Body.Len() != 0 {
				t.Fatalf("expected empty body, got %q", rec.Body.String())
			}
			if spy.creates != 0 || len(spy.All(context.Background())) != 0 {
				t.Fatal("failed create modified store")
			}
		})
	}
}

func TestCreateNoteDistinctIDs(t *testing.T) {
	h := newRouter(newSpy())
	a := seed(t, h, "one", "")
	b := seed(t, h, "two", "")
	if a.ID == b.ID {
		t.Fatalf("ids not distinct: %d", a.ID)
	}
	if l := decodeList(t, do(t, h, http.MethodGet, "/notes", "", "")); len(l) != 2 {
		t.Fatalf("list = %+v", l)
	}
}

func TestCreateNoteTooLarge(t *testing.T) {
	if testing.Short() {
		t.Skip("large body")
	}
	spy := newSpy()
	h := newRouter(spy)
	big := `{"title":"` + strings.Repeat("a", maxBodyBytes) + `"}`
	rec := do(t, h, http.MethodPost, "/notes", big, jsonCT)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d", rec.Code)
	}
	if spy.creates != 0 {
		t.Fatal("store modified")
	}
}

func TestUpdateNote(t *testing.T) {
	tests := []struct {
		name      string
		id        string // "" => existing note
		body      string
		ct        string
		status    int
		errMsg    string
		wantTitle string
		wantBody  string
		emptyBody bool
	}{
		{"title and body", "", `{"title":"T2","body":"B2"}`, jsonCT, 200, "", "T2", "B2", false},
		{"only body", "", `{"body":"B2"}`, jsonCT, 200, "", "T", "B2", false},
		{"only title", "", `{"title":"T2"}`, jsonCT, 200, "", "T2", "B", false},
		{"body empty string", "", `{"body":""}`, jsonCT, 200, "", "T", "", false},
		{"nulls", "", `{"title":null,"body":null}`, jsonCT, 200, "", "T", "B", false},
		{"empty object", "", `{}`, jsonCT, 200, "", "T", "B", false},
		{"empty title", "", `{"title":""}`, jsonCT, 400, "title must not be empty", "", "", false},
		{"whitespace title", "", `{"title":"   "}`, jsonCT, 400, "title must not be empty", "", "", false},
		{"empty title nonexistent id", "999", `{"title":""}`, jsonCT, 400, "title must not be empty", "", "", false},
		{"nonexistent id", "999", `{"title":"x"}`, jsonCT, 404, "not found", "", "", false},
		{"nonexistent id empty object", "999", `{}`, jsonCT, 404, "not found", "", "", false},
		{"non-integer id", "abc", `{"title":"x"}`, jsonCT, 404, "", "", "", true},
		{"malformed json", "", `{"title"`, jsonCT, 400, "", "", "", true},
		{"null body", "", `null`, jsonCT, 400, "", "", "", true},
		{"empty request body", "", ``, jsonCT, 400, "", "", "", true},
		{"wrong type", "", `{"body":1}`, jsonCT, 400, "", "", "", true},
		{"no content type", "", `{"title":"x"}`, "", 415, "", "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spy := newSpy()
			h := newRouter(spy)
			n := seed(t, h, "T", "B")
			id := tc.id
			if id == "" {
				id = strconv.Itoa(int(n.ID))
			}
			rec := do(t, h, http.MethodPut, "/notes/"+id, tc.body, tc.ct)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d (%q)", rec.Code, tc.status, rec.Body.String())
			}
			stored := decodeNote(t, do(t, h, http.MethodGet, "/notes/"+strconv.Itoa(int(n.ID)), "", ""))
			if tc.status == 200 {
				got := decodeNote(t, rec)
				want := noteJSON{n.ID, tc.wantTitle, tc.wantBody}
				if got != want || stored != want {
					t.Fatalf("got %+v stored %+v want %+v", got, stored, want)
				}
				return
			}
			if tc.errMsg != "" {
				assertError(t, rec, tc.status, tc.errMsg)
			}
			if tc.emptyBody && rec.Body.Len() != 0 {
				t.Fatalf("expected empty body, got %q", rec.Body.String())
			}
			if stored != n {
				t.Fatalf("failed update modified note: %+v", stored)
			}
			if tc.status == 400 && tc.errMsg != "" && spy.updates != 0 {
				t.Fatal("store Update called despite validation failure")
			}
		})
	}
}

func TestDeleteNote(t *testing.T) {
	h := newRouter(newSpy())
	a := seed(t, h, "a", "")
	b := seed(t, h, "b", "")
	aPath := "/notes/" + strconv.Itoa(int(a.ID))

	steps := []struct {
		name      string
		path      string
		status    int
		errMsg    string
		emptyBody bool
	}{
		{"existing", aPath, 204, "", true},
		{"already deleted", aPath, 404, "not found", false},
		{"nonexistent", "/notes/999", 404, "not found", false},
		{"non-integer", "/notes/abc", 404, "", true},
	}
	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			rec := do(t, h, http.MethodDelete, s.path, "", "")
			if rec.Code != s.status {
				t.Fatalf("status = %d, want %d", rec.Code, s.status)
			}
			if s.errMsg != "" {
				assertError(t, rec, s.status, s.errMsg)
			}
			if s.emptyBody && rec.Body.Len() != 0 {
				t.Fatalf("body = %q", rec.Body.String())
			}
		})
	}

	if rec := do(t, h, http.MethodGet, aPath, "", ""); rec.Code != 404 {
		t.Fatalf("deleted note GET status %d", rec.Code)
	}
	l := decodeList(t, do(t, h, http.MethodGet, "/notes", "", ""))
	if len(l) != 1 || l[0] != b {
		t.Fatalf("list after delete = %+v", l)
	}
}

func TestRegisterRoutes(t *testing.T) {
	r := chi.NewRouter()
	RegisterRoutes(r)
	if rec := do(t, r, http.MethodGet, "/health", "", ""); rec.Code != 200 {
		t.Fatalf("health status %d", rec.Code)
	}
	if rec := do(t, r, http.MethodPost, "/notes", `{"title":"x"}`, jsonCT); rec.Code != 201 {
		t.Fatalf("create status %d", rec.Code)
	}
	if l := decodeList(t, do(t, r, http.MethodGet, "/notes", "", "")); len(l) != 1 {
		t.Fatalf("shared store not used: %+v", l)
	}
}

func TestHasJSONContentType(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"", false},
		{"application/json", true},
		{"Application/JSON", true},
		{"application/json; charset=utf-8", true},
		{"application/problem+json", true},
		{"text/json", false},
		{"text/plain", false},
		{"invalid;;", false},
	}
	for _, tc := range tests {
		if got := hasJSONContentType(tc.ct); got != tc.want {
			t.Errorf("hasJSONContentType(%q) = %v, want %v", tc.ct, got, tc.want)
		}
	}
}

func TestIsNullOrWhiteSpace(t *testing.T) {
	s := func(v string) *string { return &v }
	tests := []struct {
		name string
		in   *string
		want bool
	}{
		{"nil", nil, true},
		{"empty", s(""), true},
		{"spaces", s(" \t\r\n"), true},
		{"text", s("a"), false},
		{"padded", s(" a "), false},
	}
	for _, tc := range tests {
		if got := isNullOrWhiteSpace(tc.in); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}