package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"form-backend/server/internal/store"
)

// StudentStore is the subset of store.StudentStore the handler needs.
// The concrete *store.StudentStore satisfies this automatically,
// so the HTTP layer stays testable without a database.
type StudentStore interface {
	Create(*store.Student) error
	Get(int) (*store.Student, error)
	Update(int, *store.Student) error
	Delete(int) error
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

// @Summary Get a student by ID
// @Description Retrieve a single student record by its ID.
// @Tags students
// @Produce json
// @Param id path int true "student ID"
// @Success 200 {object} store.Student "student record"
// @Failure 400 {object} map[string]string "invalid student id"
// @Failure 404 {object} map[string]string "student not found"
// @Failure 500 {object} map[string]string "internal server error"
// @Router /students/{id} [get]
func (h *StudentHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		respond(w, http.StatusBadRequest, map[string]string{"error": "invalid student id"})
		return
	}

	st, err := h.store.Get(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respond(w, http.StatusNotFound, map[string]string{"error": "student not found"})
			return
		}
		log.Printf("get student: %v", err)
		respond(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	respond(w, http.StatusOK, st)
}

// @Summary Update a student
// @Description Update an existing student record by ID.
// @Tags students
// @Accept json
// @Produce json
// @Param id path int true "student ID"
// @Param student body store.Student true "updated student data"
// @Success 200 {object} store.Student "updated student"
// @Failure 400 {object} map[string]string "validation failed or invalid request body"
// @Failure 404 {object} map[string]string "student not found"
// @Failure 500 {object} map[string]string "internal server error"
// @Router /students/{id} [put]
func (h *StudentHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		respond(w, http.StatusBadRequest, map[string]string{"error": "invalid student id"})
		return
	}

	var st store.Student
	if err := json.NewDecoder(r.Body).Decode(&st); err != nil {
		respond(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

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

	if _, err := h.store.Get(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respond(w, http.StatusNotFound, map[string]string{"error": "student not found"})
			return
		}
		log.Printf("update get student: %v", err)
		respond(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	if err := h.store.Update(id, &st); err != nil {
		log.Printf("update student: %v", err)
		respond(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	st.ID = id
	respond(w, http.StatusOK, st)
}

// @Summary Delete a student
// @Description Delete an existing student record by ID.
// @Tags students
// @Produce json
// @Param id path int true "student ID"
// @Success 200 {object} map[string]string "student deleted"
// @Failure 400 {object} map[string]string "invalid student id"
// @Failure 404 {object} map[string]string "student not found"
// @Failure 500 {object} map[string]string "internal server error"
// @Router /students/{id} [delete]
func (h *StudentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		respond(w, http.StatusBadRequest, map[string]string{"error": "invalid student id"})
		return
	}

	if _, err := h.store.Get(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respond(w, http.StatusNotFound, map[string]string{"error": "student not found"})
			return
		}
		log.Printf("delete get student: %v", err)
		respond(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	if err := h.store.Delete(id); err != nil {
		log.Printf("delete student: %v", err)
		respond(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	respond(w, http.StatusOK, map[string]string{"message": "student deleted"})
}

// Search handles GET /students. Placeholder — to be implemented per the search plan.
func (h *StudentHandler) Search(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}

func parseID(r *http.Request) (int, error) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return 0, errors.New("no id in path")
	}
	return strconv.Atoi(parts[len(parts)-1])
}

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("respond: %v", err)
	}
}
