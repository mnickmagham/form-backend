package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"form-backend/server/internal/store"
)

// StudentStore is the subset of store.StudentStore the handler needs.
// The concrete *store.StudentStore satisfies this automatically,
// so the HTTP layer stays testable without a database.
type StudentStore interface {
	Create(*store.Student) error
	FilterExact(string, string) ([]*store.Student, error)
}

type StudentHandler struct {
	store StudentStore
}

func NewStudentHandler(s StudentStore) *StudentHandler {
	return &StudentHandler{store: s}
}

// @Summary Register a new student
// @Description Create a new student record. student_id, name and last_name are required.
// @Tags students
// @Accept json
// @Produce json
// @Param student body store.Student true "student data"
// @Success 201 {object} store.Student "created student"
// @Failure 400 {object} map[string]string "validation failed or invalid request body"
// @Failure 409 {object} map[string]string "student already registered"
// @Failure 500 {object} map[string]string "internal server error"
// @Router /students [post]
func (h *StudentHandler) Register(w http.ResponseWriter, r *http.Request) {
	var st store.Student
	if err := json.NewDecoder(r.Body).Decode(&st); err != nil {
		respond(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Validate required fields (major, degree, phone_number are optional per schema).
	var missing []string
	if st.StudentID == "" {
		missing = append(missing, "student_id")
	}
	if st.Name == "" {
		missing = append(missing, "name")
	}
	if st.LastName == "" {
		missing = append(missing, "last_name")
	}
	if len(missing) > 0 {
		respond(w, http.StatusBadRequest, map[string]any{
			"error":   "validation failed",
			"details": map[string]string{"missing": strings.Join(missing, ", ")},
		})
		return
	}

	// Duplicate check on student_id (DB UNIQUE constraint is the backstop).
	existing, err := h.store.FilterExact("student_id", st.StudentID)
	if err != nil {
		log.Printf("register duplicate check: %v", err)
		respond(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if len(existing) > 0 {
		respond(w, http.StatusConflict, map[string]string{"error": "student already registered"})
		return
	}

	if err := h.store.Create(&st); err != nil {
		log.Printf("register create: %v", err)
		respond(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	respond(w, http.StatusCreated, st)
}

// Search handles GET /students. Placeholder — to be implemented per the search plan.
func (h *StudentHandler) Search(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}

// Get handles GET /students/{id}. Placeholder — to be implemented per the get plan.
func (h *StudentHandler) Get(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}

// Update handles PUT /students/{id}. Placeholder — to be implemented per the update plan.
func (h *StudentHandler) Update(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}

// Delete handles DELETE /students/{id}. Placeholder — to be implemented per the delete plan.
func (h *StudentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("respond: %v", err)
	}
}
