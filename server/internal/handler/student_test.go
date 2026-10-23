package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
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

func (f *fakeStore) Get(id int) (*store.Student, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, s := range f.students {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (f *fakeStore) Update(id int, st *store.Student) error {
	if f.err != nil {
		return f.err
	}
	for i, s := range f.students {
		if s.ID == id {
			st.ID = id
			f.students[i] = st
			return nil
		}
	}
	return sql.ErrNoRows
}

func (f *fakeStore) Delete(id int) error {
	if f.err != nil {
		return f.err
	}
	for i, s := range f.students {
		if s.ID == id {
			f.students = append(f.students[:i], f.students[i+1:]...)
			return nil
		}
	}
	return sql.ErrNoRows
}

func (f *fakeStore) GlobalSearch(value string) ([]*store.Student, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []*store.Student
	for _, s := range f.students {
		if strings.Contains(s.Name, value) || strings.Contains(s.LastName, value) ||
			strings.Contains(s.StudentID, value) || strings.Contains(s.Major, value) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeStore) PaginatedSearch(filters map[string]string, sortBy, sortOrder string, page, limit int) ([]*store.Student, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	var matched []*store.Student
	for _, s := range f.students {
		match := true
		for col, val := range filters {
			if val == "" {
				continue
			}
			var field string
			switch col {
			case "student_id":
				field = s.StudentID
			case "name":
				field = s.Name
			case "last_name":
				field = s.LastName
			case "major":
				field = s.Major
			case "degree":
				field = s.Degree
			case "phone_number":
				field = s.PhoneNumber
			default:
				match = false
			}
			if !strings.Contains(field, val) {
				match = false
			}
		}
		if match {
			matched = append(matched, s)
		}
	}

	total := len(matched)
	start := (page - 1) * limit
	if start >= total {
		return []*store.Student{}, total, nil
	}
	end := start + limit
	if end > total {
		end = total
	}
	return matched[start:end], total, nil
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

func TestGetValid(t *testing.T) {
	f := &fakeStore{}
	h := NewStudentHandler(f)

	created := &store.Student{StudentID: "S20001", Name: "Jane", LastName: "Doe"}
	if err := f.Create(created); err != nil {
		t.Fatalf("create: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/students/1", nil)
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got store.Student
	decodeBody(t, w, &got)
	if got.ID != 1 || got.StudentID != "S20001" || got.Name != "Jane" {
		t.Errorf("unexpected student: %+v", got)
	}
}

func TestGetNotFound(t *testing.T) {
	f := &fakeStore{}
	h := NewStudentHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/students/999", nil)
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetInvalidID(t *testing.T) {
	f := &fakeStore{}
	h := NewStudentHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/students/abc", nil)
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetStoreError(t *testing.T) {
	f := &fakeStore{err: &boomError{}}
	h := NewStudentHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/students/1", nil)
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateValid(t *testing.T) {
	f := &fakeStore{}
	h := NewStudentHandler(f)

	created := &store.Student{StudentID: "S30001", Name: "Old", LastName: "Name"}
	if err := f.Create(created); err != nil {
		t.Fatalf("create: %v", err)
	}

	body := `{"student_id":"S30001","name":"New","last_name":"Name","major":"Math"}`
	req := httptest.NewRequest(http.MethodPut, "/students/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Update(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got store.Student
	decodeBody(t, w, &got)
	if got.ID != 1 || got.Name != "New" || got.Major != "Math" {
		t.Errorf("unexpected updated student: %+v", got)
	}
}

func TestUpdateNotFound(t *testing.T) {
	f := &fakeStore{}
	h := NewStudentHandler(f)

	body := `{"student_id":"S30002","name":"New","last_name":"Name"}`
	req := httptest.NewRequest(http.MethodPut, "/students/999", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Update(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateInvalidID(t *testing.T) {
	f := &fakeStore{}
	h := NewStudentHandler(f)

	body := `{"student_id":"S30003","name":"New","last_name":"Name"}`
	req := httptest.NewRequest(http.MethodPut, "/students/abc", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Update(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateStoreError(t *testing.T) {
	f := &fakeStore{err: &boomError{}}
	h := NewStudentHandler(f)

	body := `{"student_id":"S30004","name":"New","last_name":"Name"}`
	req := httptest.NewRequest(http.MethodPut, "/students/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Update(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteValid(t *testing.T) {
	f := &fakeStore{}
	h := NewStudentHandler(f)

	if err := f.Create(&store.Student{StudentID: "S40001", Name: "Delete", LastName: "Me"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/students/1", nil)
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(f.students) != 0 {
		t.Errorf("expected 0 students after delete, got %d", len(f.students))
	}
}

func TestDeleteNotFound(t *testing.T) {
	f := &fakeStore{}
	h := NewStudentHandler(f)

	req := httptest.NewRequest(http.MethodDelete, "/students/999", nil)
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteInvalidID(t *testing.T) {
	f := &fakeStore{}
	h := NewStudentHandler(f)

	req := httptest.NewRequest(http.MethodDelete, "/students/abc", nil)
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteStoreError(t *testing.T) {
	f := &fakeStore{err: &boomError{}}
	h := NewStudentHandler(f)

	req := httptest.NewRequest(http.MethodDelete, "/students/1", nil)
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSearchGlobal(t *testing.T) {
	f := &fakeStore{}
	h := NewStudentHandler(f)

	f.Create(&store.Student{StudentID: "S10001", Name: "Alice", LastName: "Smith", Major: "CS"})
	f.Create(&store.Student{StudentID: "S10002", Name: "Bob", LastName: "Jones", Major: "Math"})
	f.Create(&store.Student{StudentID: "S10003", Name: "Carol", LastName: "Smith", Major: "Physics"})

	req := httptest.NewRequest(http.MethodGet, "/students?q=Smith", nil)
	w := httptest.NewRecorder()
	h.Search(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	decodeBody(t, w, &resp)
	students := resp["students"].([]any)
	if len(students) != 2 {
		t.Errorf("expected 2 global search results, got %d", len(students))
	}
}

func TestSearchGlobalNoMatch(t *testing.T) {
	f := &fakeStore{}
	h := NewStudentHandler(f)

	f.Create(&store.Student{StudentID: "S10001", Name: "Alice", LastName: "Smith", Major: "CS"})

	req := httptest.NewRequest(http.MethodGet, "/students?q=ZZZ", nil)
	w := httptest.NewRecorder()
	h.Search(w, req)

	var resp map[string]any
	decodeBody(t, w, &resp)
	students := resp["students"].([]any)
	if len(students) != 0 {
		t.Errorf("expected 0 results, got %d", len(students))
	}
}

func TestSearchFieldFilter(t *testing.T) {
	f := &fakeStore{}
	h := NewStudentHandler(f)

	f.Create(&store.Student{StudentID: "S10001", Name: "Alice", LastName: "Smith", Major: "CS"})
	f.Create(&store.Student{StudentID: "S10002", Name: "Bob", LastName: "Jones", Major: "Math"})

	req := httptest.NewRequest(http.MethodGet, "/students?major=Math", nil)
	w := httptest.NewRecorder()
	h.Search(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	decodeBody(t, w, &resp)
	students := resp["students"].([]any)
	if len(students) != 1 {
		t.Errorf("expected 1 filtered result, got %d", len(students))
	}
}

func TestSearchPagination(t *testing.T) {
	f := &fakeStore{}
	h := NewStudentHandler(f)

	for i := 0; i < 5; i++ {
		f.Create(&store.Student{StudentID: fmt.Sprintf("S%d", i), Name: fmt.Sprintf("Student%d", i), Major: "CS"})
	}

	// page 1, limit 2
	req := httptest.NewRequest(http.MethodGet, "/students?page=1&limit=2", nil)
	w := httptest.NewRecorder()
	h.Search(w, req)

	var resp map[string]any
	decodeBody(t, w, &resp)
	students := resp["students"].([]any)
	if len(students) != 2 {
		t.Errorf("expected 2 on page 1, got %d", len(students))
	}
	if resp["total"] != float64(5) {
		t.Errorf("expected total 5, got %v", resp["total"])
	}
	if resp["page"] != float64(1) || resp["limit"] != float64(2) {
		t.Errorf("expected page 1 limit 2, got page=%v limit=%v", resp["page"], resp["limit"])
	}

	// page 3, limit 2 -> 1 item
	req = httptest.NewRequest(http.MethodGet, "/students?page=3&limit=2", nil)
	w = httptest.NewRecorder()
	h.Search(w, req)
	decodeBody(t, w, &resp)
	students = resp["students"].([]any)
	if len(students) != 1 {
		t.Errorf("expected 1 on page 3, got %d", len(students))
	}

	// page beyond range
	req = httptest.NewRequest(http.MethodGet, "/students?page=10&limit=2", nil)
	w = httptest.NewRecorder()
	h.Search(w, req)
	decodeBody(t, w, &resp)
	students = resp["students"].([]any)
	if len(students) != 0 {
		t.Errorf("expected 0 beyond range, got %d", len(students))
	}
}

func TestSearchInvalidPagination(t *testing.T) {
	f := &fakeStore{}
	h := NewStudentHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/students?page=abc", nil)
	w := httptest.NewRecorder()
	h.Search(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/students?limit=0", nil)
	w = httptest.NewRecorder()
	h.Search(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSearchStoreError(t *testing.T) {
	f := &fakeStore{err: &boomError{}}
	h := NewStudentHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/students?q=test", nil)
	w := httptest.NewRecorder()
	h.Search(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

type boomError struct{}

func (e *boomError) Error() string { return "boom" }
