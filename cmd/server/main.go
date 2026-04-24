package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/varcharC2k/vape-crm/internal/db"
	"github.com/varcharC2k/vape-crm/internal/handler"
	"github.com/varcharC2k/vape-crm/internal/repository"
	"github.com/varcharC2k/vape-crm/internal/service"
)

func main() {
	// 1. DB 연결 + 마이그레이션
	conn, err := db.Open("data/vape-crm.db")
	if err != nil {
		log.Fatalf("DB 연결 실패: %v", err)
	}
	defer conn.Close()
	log.Println("DB 연결 성공")

	if err := db.Migrate(conn); err != nil {
		log.Fatalf("DB 마이그레이션 실패: %v", err)
	}
	log.Println("DB 마이그레이션 완료")

	// 2. 템플릿 로드 (embed 된 web/templates 에서)
	tmpl, err := handler.LoadTemplates()
	if err != nil {
		log.Fatalf("템플릿 로드 실패: %v", err)
	}

	// 3. 의존성 주입 (repo -> service -> handler)
	productRepo := repository.NewProductRepository(conn)
	productSvc := service.NewProductService(productRepo)
	productHandler := handler.NewProductHandler(productSvc, tmpl)

	// 4. 라우터
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// 루트 접속 시 품목 관리로
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/products/", http.StatusSeeOther)
	})

	productHandler.Register(r)

	// 5. 기동
	addr := ":8080"
	log.Printf("서버 시작: http://localhost%s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
