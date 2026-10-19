// Package notes contains the data models and in-memory storage for the
// Notes API.
package notes

// Note is a persisted note as stored and returned by the API.
//
// It mirrors the C# positional record `Note(int Id, string Title, string Body)`.
// Values are passed by value, so callers receive copies and cannot mutate a
// stored note; this preserves the record's immutability semantics. Go struct
// equality (==) gives the same value-based equality the record provided.
type Note struct {
	// ID is the note's identifier. int32 keeps the 32-bit range of C#'s int.
	ID int32 `json:"id"`
	// Title is the note's title. Always present (non-nullable).
	Title string `json:"title"`
	// Body is the note's content. Always present (non-nullable).
	Body string `json:"body"`
}

// NewNote constructs a Note positionally, mirroring `new Note(Id, Title, Body)`.
func NewNote(id int32, title, body string) Note {
	return Note{ID: id, Title: title, Body: body}
}

// Input is the request body for create and update operations.
//
// It mirrors the C# record `NoteInput(string? Title, string? Body)`. Both
// fields are pointers so an absent or JSON-null field (nil) stays distinct
// from an empty string.
//
// encoding/json decoding matches System.Text.Json's web defaults used by
// ASP.NET minimal APIs: property names match case-insensitively, unknown
// properties are ignored, and duplicate keys are last-wins.
type Input struct {
	// Title is the requested title, or nil when absent/null.
	Title *string `json:"title"`
	// Body is the requested body, or nil when absent/null.
	Body *string `json:"body"`
}

// NewInput constructs an Input positionally, mirroring `new NoteInput(Title, Body)`.
func NewInput(title, body *string) Input {
	return Input{Title: title, Body: body}
}
