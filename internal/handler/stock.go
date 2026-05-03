package handler

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/varcharC2k/vape-crm/internal/repository"
	"github.com/varcharC2k/vape-crm/internal/service"
)

// StockHandler — 재고 현황 조회 화면.
//
// 별도의 재고 테이블은 없고, products.stock_qty 를 그대로 보여준다.
// 매입/매출 트랜잭션이 stock_qty 를 갱신하므로 항상 실시간 값.
//
// ProductService.List 에 MinStockQty=1 필터를 걸어 stock_qty > 0 인 품목만 반환한다.
type StockHandler struct {
	svc  *service.ProductService
	tmpl *template.Template
}

func NewStockHandler(svc *service.ProductService, tmpl *template.Template) *StockHandler {
	return &StockHandler{svc: svc, tmpl: tmpl}
}

func (h *StockHandler) Register(r chi.Router) {
	r.Route("/stocks", func(r chi.Router) {
		r.Get("/", h.list)
	})
}

// list — GET /stocks/
//   - 일반 접근(HX-Request 없음): 풀 페이지
//   - HTMX 요청(필터 변경): tbody 파셜만
func (h *StockHandler) list(w http.ResponseWriter, r *http.Request) {
	filter := h.parseFilter(r)
	products, err := h.svc.List(r.Context(), filter)
	if err != nil {
		h.serverError(w, err)
		return
	}

	if isHTMX(r) {
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"stock-count-changed": %d}`, len(products)))
		h.render(w, "stocks_tbody", map[string]any{"Products": products})
		return
	}

	h.render(w, "layout", map[string]any{
		"Title":    "재고 현황",
		"Products": products,
	})
}

// parseFilter — 분류·이름 필터 + 항상 MinStockQty=1.
// 품목 화면의 parseFilter 를 재사용하고 MinStockQty 만 추가.
func (h *StockHandler) parseFilter(r *http.Request) repository.ProductFilter {
	filter := parseFilter(r) // product.go 의 패키지 함수 재사용 (q_category, q_name)
	min := 1
	filter.MinStockQty = &min
	return filter
}

func (h *StockHandler) render(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := h.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("템플릿 렌더 실패 (%s): %v", name, err)
		http.Error(w, "템플릿 렌더 실패", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("응답 쓰기 실패: %v", err)
	}
}

func (h *StockHandler) serverError(w http.ResponseWriter, err error) {
	log.Printf("서버 에러: %v", err)
	http.Error(w, "서버 오류", http.StatusInternalServerError)
}
