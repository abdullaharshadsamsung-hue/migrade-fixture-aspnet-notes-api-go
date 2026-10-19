// Package httpapi exposes the Notes REST API over HTTP.
//
// It is the Go port of the ASP.NET Core minimal-API endpoints declared in the
// source Program.cs. Status codes, error bodies and request-binding failures
// mirror the ASP.NET behaviour (Production environment) as closely as the
// standard library allows.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"migrated-app/internal/notes"
)

// maxBodyBytes mirrors Kestrel's default MaxRequestBodySize (30,000,000 bytes).
const maxBodyBytes = 30_000_000

// NoteStore is the storage behaviour the handlers depend on. *notes.Store
// satisfies it; tests may substitute their own implementation.
type NoteStore interface {
	All(ctx context.Context) []notes.Note
	Get(ctx context.Context, id int32) (notes.Note, bool)
	Create(ctx context.Context, title, body string) notes.Note
	Update(ctx context.Context, id int32, title, body *string) (notes.Note, bool)
	Delete(ctx context.Context, id int32) bool
}

// Handler holds the dependencies shared by all Notes API endpoints.
type Handler struct {
	store NoteStore
}

// NewHandler returns a Handler backed by the given store.
func NewHandler(store NoteStore) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes registers every Notes API route on r.
//
// A single in-memory store is created here and shared by all handlers,
// mirroring the source's builder.Services.AddSingleton<NoteStore>().
//
// MIGRATION_NOTE: ASP.NET routing matches literal segments case-insensitively
// and tolerates a trailing slash (e.g. "/NOTES/"). chi is case-sensitive and
// strict about trailing slashes; replicating that requires a path-normalising
// middleware installed on the root router (r.Use before any route is added),
// which is owned by cmd/server/router.go. Review if clients rely on it.
func RegisterRoutes(r chi.Router) {
	NewHandler(notes.NewStore()).Routes(r)
}

// Routes registers this handler's endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/health", h.health)
	r.Get("/notes", h.list)
	r.Get("/notes/{id}", h.get)
	r.Post("/notes", h.create)
	r.Put("/notes/{id}", h.update)
	r.Delete("/notes/{id}", h.delete)
}

type errorBody struct {
	Error string `json:"error"`
}

var errNotFound = errorBody{Error: "not found"}

// health is the liveness check: GET /health -> 200 {"status":"ok"}.
func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// list handles GET /notes.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.All(r.Context()))
}

// get handles GET /notes/{id:int}.
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(r)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	note, found := h.store.Get(r.Context(), id)
	if !found {
		writeJSON(w, http.StatusNotFound, errNotFound)
		return
	}
	writeJSON(w, http.StatusOK, note)
}

// create handles POST /notes.
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	input, status := bindInput(w, r)
	if status != 0 {
		w.WriteHeader(status)
		return
	}
	if isNullOrWhiteSpace(input.Title) {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "title is required"})
		return
	}
	body := ""
	if input.Body != nil {
		body = *input.Body
	}
	note := h.store.Create(r.Context(), *input.Title, body)
	w.Header().Set("Location", "/notes/"+strconv.FormatInt(int64(note.ID), 10))
	writeJSON(w, http.StatusCreated, note)
}

// update handles PUT /notes/{id:int}. It behaves like PATCH: nil fields are
// left unchanged. The title is validated before existence is checked.
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(r)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	input, status := bindInput(w, r)
	if status != 0 {
		w.WriteHeader(status)
		return
	}
	if input.Title != nil && isNullOrWhiteSpace(input.Title) {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "title must not be empty"})
		return
	}
	note, found := h.store.Update(r.Context(), id, input.Title, input.Body)
	if !found {
		writeJSON(w, http.StatusNotFound, errNotFound)
		return
	}
	writeJSON(w, http.StatusOK, note)
}

// delete handles DELETE /notes/{id:int}.
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(r)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if !h.store.Delete(r.Context(), id) {
		writeJSON(w, http.StatusNotFound, errNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// routeID emulates ASP.NET's {id:int} route constraint (int.TryParse with
// invariant culture): surrounding whitespace and a leading sign are allowed,
// and the value must fit in 32 bits. A false result means "route did not
// match", which the caller turns into an empty-body 404.
//
// MIGRATION_NOTE: in ASP.NET a failed constraint means no endpoint matches,
// so even a wrong HTTP method yields 404. With chi, e.g. POST /notes/abc
// returns 405 because method routing happens before the handler parses id.
func routeID(r *http.Request) (int32, bool) {
	raw := strings.TrimSpace(chi.URLParam(r, "id"))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, false
	}
	return int32(v), true
}

// bindInput emulates ASP.NET minimal-API implicit JSON body binding for a
// required NoteInput parameter. It returns a non-zero HTTP status (with no
// body to be written) when binding fails:
//   - 415 when the Content-Type is missing or not JSON
//   - 413 when the body exceeds Kestrel's default limit
//   - 400 when the body is empty, the literal null, not a JSON object,
//     malformed, has wrong field types, or has trailing data
func bindInput(w http.ResponseWriter, r *http.Request) (notes.Input, int) {
	var in notes.Input
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		return in, http.StatusUnsupportedMediaType
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return in, http.StatusRequestEntityTooLarge
		}
		return in, http.StatusBadRequest
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		// Covers empty bodies, the literal null and non-object roots.
		return in, http.StatusBadRequest
	}
	// json.Unmarshal rejects malformed JSON, type mismatches and trailing
	// data, matching System.Text.Json's behaviour.
	if err := json.Unmarshal(trimmed, &in); err != nil {
		return in, http.StatusBadRequest
	}
	return in, 0
}

// hasJSONContentType reports whether ct is application/json or a +json type,
// matching ASP.NET's HttpRequest.HasJsonContentType.
func hasJSONContentType(ct string) bool {
	if ct == "" {
		return false
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	return mt == "application/json" || strings.HasSuffix(mt, "+json")
}

// isNullOrWhiteSpace mirrors C#'s string.IsNullOrWhiteSpace.
func isNullOrWhiteSpace(s *string) bool {
	return s == nil || strings.TrimSpace(*s) == ""
}

// writeJSON serialises v as the response body with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
