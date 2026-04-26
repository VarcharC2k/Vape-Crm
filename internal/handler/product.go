package handler

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/varcharC2k/vape-crm/internal/models"
	"github.com/varcharC2k/vape-crm/internal/repository"
	"github.com/varcharC2k/vape-crm/internal/service"
)

// ProductHandler — /products 경로들의 HTTP 처리.
type ProductHandler struct {
	svc  *service.ProductService
	tmpl *template.Template
}

func NewProductHandler(svc *service.ProductService, tmpl *template.Template) *ProductHandler {
	return &ProductHandler{svc: svc, tmpl: tmpl}
}

// Register — 라우트 등록.
func (h *ProductHandler) Register(r chi.Router) {
	r.Route("/products", func(r chi.Router) {
		r.Get("/", h.list)              // 목록 (HX-Request 헤더 유무로 풀페이지/tbody 분기)
		r.Get("/new", h.newForm)        // 등록 모달 body
		r.Post("/", h.create)           // 등록 실행 -> 갱신된 tbody (필터 반영)
		r.Get("/{id}/edit", h.editForm) // 수정 모달 body
		r.Put("/{id}", h.update)        // 수정 실행 -> 갱신된 tbody
		r.Delete("/{id}", h.delete)     // 삭제 -> 갱신된 tbody
	})
}

// list — GET /products/
//   - 일반 브라우저 접근(HX-Request 헤더 없음): layout + 풀 페이지 렌더, 필터 인풋 비어있는 상태.
//   - HTMX 요청(필터 인풋 변경 등): tbody 파셜만 반환, 쿼리 파라미터의 필터 적용.
func (h *ProductHandler) list(w http.ResponseWriter, r *http.Request) {
	if isHTMX(r) {
		h.renderTbody(w, r)
		return
	}

	// 풀 페이지는 필터 없이 전체 목록.
	products, err := h.svc.List(r.Context(), repository.ProductFilter{})
	if err != nil {
		h.serverError(w, err)
		return
	}
	h.render(w, "layout", map[string]any{
		"Title":    "품목 관리",
		"Products": products,
	})
}

// renderTbody — 필터를 적용해 tbody 파셜만 반환.
// 카운트 갱신 트리거도 함께 보낸다(모달 닫기 신호는 빠짐).
func (h *ProductHandler) renderTbody(w http.ResponseWriter, r *http.Request) {
	products, err := h.svc.List(r.Context(), parseFilter(r))
	if err != nil {
		h.serverError(w, err)
		return
	}
	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"product-count-changed": %d}`, len(products)))
	h.render(w, "products_tbody", map[string]any{"Products": products})
}

// newForm — 빈 폼을 모달 body 에 주입.
//
// 매출단가는 기본값 25000 으로 초기화한다.
// 폼 검증 실패 후 재렌더 시에는 사용자가 입력한 값이 보존되도록
// 이 기본값은 newForm 에서만 적용하고 create 핸들러에선 손대지 않는다.
func (h *ProductHandler) newForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, "products_form", formData(&models.Product{
		SalePrice: 25000,
	}, nil))
}

// editForm — 값이 채워진 수정용 폼.
func (h *ProductHandler) editForm(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			http.NotFound(w, r)
			return
		}
		h.serverError(w, err)
		return
	}
	h.render(w, "products_form", formData(p, nil))
}

func (h *ProductHandler) create(w http.ResponseWriter, r *http.Request) {
	p, err := parseProductForm(r)
	if err != nil {
		http.Error(w, "폼 파싱 실패: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.svc.Create(r.Context(), p); err != nil {
		if h.tryRenderFormError(w, err, p) {
			return
		}
		h.serverError(w, err)
		return
	}
	h.renderTbodyAndTrigger(w, r)
}

func (h *ProductHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	p, err := parseProductForm(r)
	if err != nil {
		http.Error(w, "폼 파싱 실패: "+err.Error(), http.StatusBadRequest)
		return
	}
	p.ID = id

	if err := h.svc.Update(r.Context(), p); err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			http.NotFound(w, r)
			return
		}
		if h.tryRenderFormError(w, err, p) {
			return
		}
		h.serverError(w, err)
		return
	}
	h.renderTbodyAndTrigger(w, r)
}

func (h *ProductHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			http.NotFound(w, r)
			return
		}
		h.serverError(w, err)
		return
	}
	h.renderTbodyAndTrigger(w, r)
}

// tryRenderFormError — err 가 ValidationErrors 면 모달 폼을 재렌더.
// HTMX 응답 헤더로 swap 대상을 tbody 가 아닌 모달 body 로 전환한다.
// 처리했으면 true, 아니면 false.
func (h *ProductHandler) tryRenderFormError(w http.ResponseWriter, err error, p *models.Product) bool {
	var valErrs service.ValidationErrors
	if !errors.As(err, &valErrs) {
		return false
	}
	w.Header().Set("HX-Retarget", "#modal-body")
	w.Header().Set("HX-Reswap", "innerHTML")
	w.WriteHeader(http.StatusUnprocessableEntity)
	h.render(w, "products_form", formData(p, valErrs))
	return true
}

// renderTbodyAndTrigger — CRUD 성공 후 호출.
// 필터(폼/쿼리에 동봉돼 들어옴) 적용해 tbody 파셜 반환 + 모달 닫기 + 카운트 갱신.
func (h *ProductHandler) renderTbodyAndTrigger(w http.ResponseWriter, r *http.Request) {
	products, err := h.svc.List(r.Context(), parseFilter(r))
	if err != nil {
		h.serverError(w, err)
		return
	}
	trigger := fmt.Sprintf(`{"product-saved": null, "product-count-changed": %d}`, len(products))
	w.Header().Set("HX-Trigger", trigger)
	h.render(w, "products_tbody", map[string]any{"Products": products})
}

// render — 템플릿을 버퍼에 먼저 쓴 뒤 일괄 flush.
func (h *ProductHandler) render(w http.ResponseWriter, name string, data any) {
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

func (h *ProductHandler) serverError(w http.ResponseWriter, err error) {
	log.Printf("서버 에러: %v", err)
	http.Error(w, "서버 오류", http.StatusInternalServerError)
}

func formData(p *models.Product, errs service.ValidationErrors) map[string]any {
	return map[string]any{
		"Product": p,
		"Errors":  errs,
	}
}

// isHTMX — HTMX 발 요청인지(HX-Request 헤더 유무).
// 풀 페이지 vs 파셜 응답 분기 기준.
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

// parseFilter — 요청에서 필터 인풋 값을 추출.
//
// 필드명 규칙:
//   - q_category: "" 또는 "-1" 이면 전체, 그 외엔 정수로 파싱.
//   - q_name:     공백만이면 전체.
//
// q_ 접두사를 쓰는 이유: 폼의 "name", "category" (품목 자체 필드) 와 충돌 방지.
//
// r.FormValue 는 GET 의 query string 과 POST 의 form body 양쪽을 모두 읽으므로
// 필터 인풋이 hx-include 로 함께 보내질 때(POST/PUT/DELETE) 도 동일하게 동작한다.
func parseFilter(r *http.Request) repository.ProductFilter {
	var filter repository.ProductFilter

	if catStr := r.FormValue("q_category"); catStr != "" && catStr != "-1" {
		if catInt, err := strconv.Atoi(catStr); err == nil {
			cat := models.Category(catInt)
			if cat.IsValid() {
				filter.Category = &cat
			}
		}
	}

	filter.Name = r.FormValue("q_name")
	return filter
}

// parseProductForm — 폼 필드를 파싱해 Product 구조체로.
func parseProductForm(r *http.Request) (*models.Product, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	categoryInt, err := strconv.Atoi(r.FormValue("category"))
	if err != nil {
		return nil, err
	}
	sale, err := strconv.ParseInt(r.FormValue("sale_price"), 10, 64)
	if err != nil {
		return nil, err
	}
	stock, err := strconv.Atoi(r.FormValue("stock_qty"))
	if err != nil {
		return nil, err
	}
	return &models.Product{
		Category:  models.Category(categoryInt),
		Name:      r.FormValue("name"),
		SalePrice: sale,
		StockQty:  stock,
	}, nil
}
