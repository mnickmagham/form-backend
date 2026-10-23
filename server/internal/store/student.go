package store

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/go-sql-driver/mysql"
)

type Student struct {
	ID          int    `json:"id"`
	StudentID   string `json:"student_id"`
	Name        string `json:"name"`
	LastName    string `json:"last_name"`
	Major       string `json:"major"`
	Degree      string `json:"degree"`
	PhoneNumber string `json:"phone_number"`
}

type StudentStore struct {
	db *sql.DB
}

func Open() (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?parseTime=true",
		env("DB_USER", "root"),
		env("DB_PASSWORD", "rootpassword"),
		env("DB_HOST", "localhost"),
		env("DB_PORT", "3306"),
		env("DB_NAME", "students_db"),
	)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func NewStudentStore(db *sql.DB) *StudentStore {
	return &StudentStore{db: db}
}

var allowedColumns = map[string]bool{
	"student_id":  true,
	"name":        true,
	"last_name":   true,
	"major":       true,
	"degree":      true,
	"phone_number": true,
}

func (s *StudentStore) Create(st *Student) error {
	res, err := s.db.Exec(
		`INSERT INTO students (student_id, name, last_name, major, degree, phone_number)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		st.StudentID, st.Name, st.LastName, st.Major, st.Degree, st.PhoneNumber,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	st.ID = int(id)
	return nil
}

func (s *StudentStore) Get(id int) (*Student, error) {
	st := &Student{}
	err := s.db.QueryRow(
		`SELECT id, student_id, name, last_name, major, degree, phone_number
		 FROM students WHERE id = ?`,
		id,
	).Scan(&st.ID, &st.StudentID, &st.Name, &st.LastName, &st.Major, &st.Degree, &st.PhoneNumber)
	if err != nil {
		return nil, err
	}
	return st, nil
}

func (s *StudentStore) Update(id int, st *Student) error {
	_, err := s.db.Exec(
		`UPDATE students
		 SET student_id = ?, name = ?, last_name = ?, major = ?, degree = ?, phone_number = ?
		 WHERE id = ?`,
		st.StudentID, st.Name, st.LastName, st.Major, st.Degree, st.PhoneNumber, id,
	)
	return err
}

func (s *StudentStore) Delete(id int) error {
	_, err := s.db.Exec(`DELETE FROM students WHERE id = ?`, id)
	return err
}

func (s *StudentStore) FilterExact(column, value string) ([]*Student, error) {
	if !allowedColumns[column] {
		return nil, fmt.Errorf("invalid column: %s", column)
	}
	query := fmt.Sprintf(
		`SELECT id, student_id, name, last_name, major, degree, phone_number
		 FROM students WHERE %s = ? ORDER BY id`,
		column,
	)
	rows, err := s.db.Query(query, value)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStudents(rows)
}

func (s *StudentStore) FilterLike(column, value string) ([]*Student, error) {
	if !allowedColumns[column] {
		return nil, fmt.Errorf("invalid column: %s", column)
	}
	query := fmt.Sprintf(
		`SELECT id, student_id, name, last_name, major, degree, phone_number
		 FROM students WHERE %s LIKE ? ORDER BY id`,
		column,
	)
	rows, err := s.db.Query(query, "%"+value+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStudents(rows)
}

func (s *StudentStore) GlobalSearch(value string) ([]*Student, error) {
	like := "%" + value + "%"
	query := `SELECT id, student_id, name, last_name, major, degree, phone_number
	          FROM students
	          WHERE name LIKE ? OR last_name LIKE ? OR student_id LIKE ? OR major LIKE ?
	          ORDER BY id`
	rows, err := s.db.Query(query, like, like, like, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStudents(rows)
}

func (s *StudentStore) PaginatedSearch(filters map[string]string, sortBy, sortOrder string, page, limit int) ([]*Student, int, error) {
	if !allowedColumns[sortBy] {
		sortBy = "id"
	}
	if sortOrder != "ASC" && sortOrder != "DESC" {
		sortOrder = "ASC"
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	args := []any{}
	clauses := []string{}
	for col, val := range filters {
		if val == "" || !allowedColumns[col] {
			continue
		}
		clauses = append(clauses, fmt.Sprintf("%s LIKE ?", col))
		args = append(args, "%"+val+"%")
	}

	baseQuery := `FROM students`
	if len(clauses) > 0 {
		baseQuery += " WHERE " + joinClauses(clauses)
	}

	// Count total matching rows
	countQuery := "SELECT COUNT(*) " + baseQuery
	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Fetch page
	offset := (page - 1) * limit
	query := `SELECT id, student_id, name, last_name, major, degree, phone_number ` +
		baseQuery +
		` ORDER BY ` + sortBy + ` ` + sortOrder +
		` LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	students, err := scanStudents(rows)
	if err != nil {
		return nil, 0, err
	}
	return students, total, nil
}

func joinClauses(clauses []string) string {
	out := clauses[0]
	for i := 1; i < len(clauses); i++ {
		out += " AND " + clauses[i]
	}
	return out
}

func scanStudents(rows *sql.Rows) ([]*Student, error) {
	var out []*Student
	for rows.Next() {
		st := &Student{}
		if err := rows.Scan(&st.ID, &st.StudentID, &st.Name, &st.LastName, &st.Major, &st.Degree, &st.PhoneNumber); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
