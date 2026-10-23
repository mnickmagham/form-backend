package store

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *StudentStore {
	t.Helper()
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?parseTime=true",
		env("DB_USER", "root"),
		env("DB_PASSWORD", "rootpassword"),
		env("DB_HOST", "localhost"),
		env("DB_PORT", "4656"),
		env("DB_NAME", "students_db"),
	)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &StudentStore{db: db}
}

func cleanupStudents(t *testing.T, db *sql.DB, ids []int) {
	t.Helper()
	for _, id := range ids {
		db.Exec("DELETE FROM students WHERE id = ?", id)
	}
}

func TestCreateAndGet(t *testing.T) {
	st := newTestStore(t)

	st.db.Exec("DELETE FROM students WHERE student_id = ?", "T001")

	s := &Student{
		StudentID:   "T001",
		Name:        "Test",
		LastName:    "User",
		Major:       "CS",
		Degree:      "BS",
		PhoneNumber: "555-0001",
	}
	if err := st.Create(s); err != nil {
		t.Fatalf("create: %v", err)
	}
	if s.ID == 0 {
		t.Fatal("expected ID to be set after create")
	}
	defer cleanupStudents(t, st.db, []int{s.ID})

	got, err := st.Get(s.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.StudentID != s.StudentID || got.Name != s.Name || got.LastName != s.LastName {
		t.Errorf("mismatch: got %+v", got)
	}
}

func TestGetNotFound(t *testing.T) {
	st := newTestStore(t)

	_, err := st.Get(999999)
	if err == nil {
		t.Fatal("expected error for missing student")
	}
	if !strings.Contains(err.Error(), "no rows") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpdate(t *testing.T) {
	st := newTestStore(t)

	st.db.Exec("DELETE FROM students WHERE student_id = ?", "T002")

	s := &Student{StudentID: "T002", Name: "Old", LastName: "Name", Major: "CS", Degree: "BS"}
	if err := st.Create(s); err != nil {
		t.Fatalf("create: %v", err)
	}

	s.Name = "New"
	s.Major = "Math"
	if err := st.Update(s.ID, s); err != nil {
		t.Fatalf("update: %v", err)
	}
	defer cleanupStudents(t, st.db, []int{s.ID})

	got, err := st.Get(s.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.Name != "New" || got.Major != "Math" {
		t.Errorf("update not persisted: %+v", got)
	}
}

func TestDelete(t *testing.T) {
	st := newTestStore(t)

	st.db.Exec("DELETE FROM students WHERE student_id = ?", "T003")

	s := &Student{StudentID: "T003", Name: "Delete", LastName: "Me", Major: "CS", Degree: "BS"}
	if err := st.Create(s); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := st.Delete(s.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := st.Get(s.ID)
	if err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestFilterExact(t *testing.T) {
	st := newTestStore(t)

	// Clean up any leftover test data from prior runs
	st.db.Exec("DELETE FROM students WHERE name = ? AND last_name IN (?, ?)", "Filter", "One", "Two")

	s1 := &Student{StudentID: "F1", Name: "Filter", LastName: "One", Major: "CS", Degree: "BS"}
	s2 := &Student{StudentID: "F2", Name: "Filter", LastName: "Two", Major: "Math", Degree: "MS"}
	if err := st.Create(s1); err != nil {
		t.Fatalf("create s1: %v", err)
	}
	if err := st.Create(s2); err != nil {
		t.Fatalf("create s2: %v", err)
	}
	defer cleanupStudents(t, st.db, []int{s1.ID, s2.ID})

	rows, err := st.FilterExact("major", "Math")
	if err != nil {
		t.Fatalf("filter exact: %v", err)
	}
	found := false
	for _, r := range rows {
		if r.StudentID == "F2" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected F2 in Math results, got %d rows", len(rows))
	}

	_, err = st.FilterExact("invalid_col", "x")
	if err == nil {
		t.Fatal("expected error for invalid column")
	}
}

func TestFilterLike(t *testing.T) {
	st := newTestStore(t)

	st.db.Exec("DELETE FROM students WHERE student_id = ?", "L1")

	s := &Student{StudentID: "L1", Name: "LikeTest", LastName: "User", Major: "CS", Degree: "BS"}
	if err := st.Create(s); err != nil {
		t.Fatalf("create: %v", err)
	}

	rows, err := st.FilterLike("name", "Like")
	if err != nil {
		t.Fatalf("filter like: %v", err)
	}
	if len(rows) == 0 || rows[0].StudentID != "L1" {
		t.Errorf("expected at least 1 like match, got %d", len(rows))
	}
}

func TestGlobalSearch(t *testing.T) {
	st := newTestStore(t)

	st.db.Exec("DELETE FROM students WHERE student_id = ?", "G1")

	s := &Student{StudentID: "G1", Name: "Global", LastName: "Search", Major: "Physics", Degree: "PhD"}
	if err := st.Create(s); err != nil {
		t.Fatalf("create: %v", err)
	}

	rows, err := st.GlobalSearch("Global")
	if err != nil {
		t.Fatalf("global search: %v", err)
	}
	if len(rows) == 0 || rows[0].StudentID != "G1" {
		t.Errorf("expected global search match, got %d", len(rows))
	}

	rows, err = st.GlobalSearch("Physics")
	if err != nil {
		t.Fatalf("global search major: %v", err)
	}
	if len(rows) == 0 || rows[0].Major != "Physics" {
		t.Errorf("expected global search to match major, got %d", len(rows))
	}
}

func TestPaginatedSearch(t *testing.T) {
	st := newTestStore(t)

	st.db.Exec("DELETE FROM students WHERE student_id LIKE 'P00%'")

	ids := make([]int, 0, 3)
	for i := 0; i < 3; i++ {
		s := &Student{StudentID: "P00" + string(rune('1'+i)), Name: "Page", LastName: "User", Major: "CS", Degree: "BS"}
		if err := st.Create(s); err != nil {
			t.Fatalf("create: %v", err)
		}
		ids = append(ids, s.ID)
	}
	defer cleanupStudents(t, st.db, ids)

	filter := map[string]string{"name": "Page"}
	students, total, err := st.PaginatedSearch(filter, "id", "ASC", 1, 1)
	if err != nil {
		t.Fatalf("paginated search: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(students) != 1 {
		t.Errorf("expected 1 student on page, got %d", len(students))
	}

	students, _, err = st.PaginatedSearch(filter, "id", "ASC", 2, 1)
	if err != nil {
		t.Fatalf("paginated search page 2: %v", err)
	}
	if len(students) != 1 {
		t.Errorf("expected 1 student on page 2, got %d", len(students))
	}

	students, total, err = st.PaginatedSearch(filter, "id", "ASC", 1, 10)
	if err != nil {
		t.Fatalf("paginated search with filter: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3 with Page filter, got %d", total)
	}

	students, _, err = st.PaginatedSearch(filter, "bad_col", "ASC", 1, 10)
	if err != nil {
		t.Fatalf("paginated search invalid sort: %v", err)
	}
	if len(students) != 3 {
		t.Errorf("expected 3 students, got %d", len(students))
	}
}

func TestOpenFailsOnBadDSN(t *testing.T) {
	origHost := os.Getenv("DB_HOST")
	os.Setenv("DB_HOST", "invalid.host.example")
	t.Cleanup(func() {
		if origHost == "" {
			os.Unsetenv("DB_HOST")
		} else {
			os.Setenv("DB_HOST", origHost)
		}
	})

	_, err := Open()
	if err == nil {
		t.Fatal("expected error connecting to invalid host")
	}
}
