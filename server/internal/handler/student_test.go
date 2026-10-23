package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"form-backend/server/internal/store"
)

// fakeStore implements StudentStore in memory.
type fakeStore struct {
	students []*store.Student
	err      error
}

func (f *fakeStore) Create(s *store.Student) error {
	if f.err != nil {
		return f.err
	}
	s.ID = len(f.students) + 1
	f.students = append(f.students, s)
	return nil
}

func (f *fakeStore) FilterExact(column, value string) ([]*store.Student, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []*store.Student
	for _, s := range f.students {
		match := false
		switch column {
		case "student_id":
			match = s.StudentID == value
		case "name":
			match = s.Name == value
		case "last_name":
			match = s.LastName == value
		}
		if match {
			out = append(out, s)
		}
	}
	return out, nil
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func TestRegisterValid(t *testing.T) {
	f := &fakeStore{}
	h := NewStudentHandler(f)

	body := `{"student_id":"S10001","name":"Jane","last_name":"Doe","major":"CS","degree":"BS"}`
	req := httptest.NewRequest(http.MethodPost, "/students", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var got store.Student
	decodeBody(t, w, &got)
	if got.StudentID != "S10001" || got.Name != "Jane" || got.ID == 0 {
		t.Errorf("unexpected created student: %+v", got)
	}
	if len(f.students) != 1 {
		t.Errorf("expected 1 student stored, got %d", len(f.students))
	}
}

func TestRegisterDuplicate(t *testing.T) {
	f := &fakeStore{students: []*store.Student{{StudentID: "S00001", Name: "Existing"}}}
	h := NewStudentHandler(f)

	body := `{"student_id":"S00001","name":"Jane","last_name":"Doe"}`
	req := httptest.NewRequest(http.MethodPost, "/students", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	decodeBody(t, w, &resp)
	if resp["error"] != "student already registered" {
		t.Errorf("unexpected error: %v", resp)
	}
}

func TestRegisterMissingFields(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		missing string
	}{
		{"no student_id", `{"name":"Jane","last_name":"Doe"}`, "student_id"},
		{"no name", `{"student_id":"S10002","last_name":"Doe"}`, "name"},
		{"no last_name", `{"student_id":"S10002","name":"Jane"}`, "last_name"},
		{"empty body", `{}`, "student_id, name, last_name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeStore{}
			h := NewStudentHandler(f)
			req := httptest.NewRequest(http.MethodPost, "/students", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.Register(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			var resp map[string]any
			decodeBody(t, w, &resp)
			if resp["error"] != "validation failed" {
				t.Errorf("expected validation failed error, got %v", resp)
			}
			details, ok := resp["details"].(map[string]any)
			if !ok {
				t.Fatalf("expected details map, got %T", resp["details"])
			}
			if details["missing"] != tt.missing {
				t.Errorf("expected missing '%s', got %v", tt.missing, details["missing"])
			}
		})
	}
}

func TestRegisterInvalidJSON(t *testing.T) {
	f := &fakeStore{}
	h := NewStudentHandler(f)

	req := httptest.NewRequest(http.MethodPost, "/students", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRegisterStoreError(t *testing.T) {
	f := &fakeStore{err: &boomError{}}
	h := NewStudentHandler(f)

	body := `{"student_id":"S10003","name":"Jane","last_name":"Doe"}`
	req := httptest.NewRequest(http.MethodPost, "/students", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

type boomError struct{}

func (e *boomError) Error() string { return "boom" }
