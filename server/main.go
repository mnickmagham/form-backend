package main

import (
	"log"
	"net/http"
	"os"

	"form-backend/server/internal/handler"
	"form-backend/server/internal/store"
	httpSwagger "github.com/swaggo/http-swagger"
)

func main() {
	db, err := store.Open()
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer db.Close()

	studentStore := store.NewStudentStore(db)
	h := handler.NewStudentHandler(studentStore)

	mux := http.NewServeMux()
	mux.Handle("GET /swagger/", httpSwagger.WrapHandler)
	mux.Handle("GET /", http.FileServer(http.Dir("ui")))
	mux.HandleFunc("POST /students", h.Register)
	mux.HandleFunc("GET /students", h.Search)
	mux.HandleFunc("GET /students/{id}", h.Get)
	mux.HandleFunc("PUT /students/{id}", h.Update)
	mux.HandleFunc("DELETE /students/{id}", h.Delete)

	addr := os.Getenv("SERVER_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}
