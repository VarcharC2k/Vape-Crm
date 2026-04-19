package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/varcharC2k/vape-crm/internal/db"
)

func main() {
	conn, err := db.Open("data/vape-crm.db")
	if err != nil {
		log.Fatalf("DB 연결 실패: %v", err)
	}
	defer conn.Close()
	log.Println("DB 연결 성공")

	r := chi.NewRouter()
	r.Use(middleware.Logger)

	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		if _, err := w.Write([]byte("Hello, vape-crm")); err != nil {
			log.Printf("응답 쓰기 실패: %v", err)
		}
	})

	addr := ":8080"
	log.Printf("서버 시작: %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
