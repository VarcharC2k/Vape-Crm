package handler

import (
	"bytes"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/varcharC2k/vape-crm/internal/models"
	"github.com/varcharC2k/vape-crm/internal/service"
)

// ProductHandler — /products 경로들의 HTTP 처리.
// 서비스와 템플릿 세트를 주입받아 사용한다.
type ProductHandler struct {
	svc  *service.ProductService
	tmpl *template.Template
}

func NewProductHandler(svc *service.ProductService, tmpl *template.Template) *ProductHandler {
	return &ProductHandler{svc: svc, tmpl: tmpl}
}

// Register — 라우트 등록. main 에서 r.Mount 대신 이 메서드를 호출한다.
func (h *ProductHandler) Register(r chi.Router) {
	r.Route("/products", func(r chi.Router) {
		r.Get("/", h.list)                 // 목록 전체 페이지
		r.Get("/new", h.newForm)           // 등록 모달 body
		r.Post("/", h.create)              // 등록 실행 -> 갱신된 tbody
		r.Get("/{id}/edit", h.editForm)    // 수정 모달 body (값 채워짐)
		r.Put("/{id}", h.update)           // 수정 실행 -> 갱신된 tbody
		r.Delete("/{id}", h.delete)        // 삭제 -> 갱신된 tbody
	})
}

// list — 목록 페이지(풀 HTML). layout + products/list 조합을 실행.
func (h *ProductHandler) list(w http.ResponseWriter, r *http.Request) {
	products, err := h.svc.List(r.Context())
	if err != nil {
		h.serverError(w, err)
		return
	}
	h.render(w, "layout", map[string]any{
		"Title":    "품목 관리",
		"Products": products,
	})
}

// newForm — 빈 폼을 모달 body 에 주입.
// 응답 HTML 이 #modal-body 에 swap 되면 클라 JS 가 dialog.showModal() 호출.
func (h *ProductHandler) newForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, "products_form", formData(&models.Product{}, nil))
}

// editForm — 값이 채워진 수정용 폼.
func (h *ProductHandler) editForm(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	view, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			http.NotFound(w, r)
			return
		}
		h.serverError(w, err)
		return
	}
	h.render(w, "products_form", formData(view.Product, nil))
}

// create — POST /products. 성공 시 갱신된 tbody + product-saved 트리거.
// 검증 실패 시 422 + 모달로 폼 재주입.
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

// update — PUT /products/{id}. 동일 패턴.
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

// delete — DELETE /products/{id}. 성공 시 갱신된 tbody 만 반환(모달 없음).
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
	// 폼이 원래 hx-target="#products-tbody" 로 쐈기 때문에
	// 에러 응답은 모달 쪽으로 재지정한다.
	w.Header().Set("HX-Retarget", "#modal-body")
	w.Header().Set("HX-Reswap", "innerHTML")
	w.WriteHeader(http.StatusUnprocessableEntity)
	h.render(w, "products_form", formData(p, valErrs))
	return true
}

// renderTbodyAndTrigger — 갱신된 tbody 반환 + HX-Trigger 로 모달 닫기 신호.
// 클라의 이벤트 리스너("product-saved")가 dialog.close() 호출.
func (h *ProductHandler) renderTbodyAndTrigger(w http.ResponseWriter, r *http.Request) {
	products, err := h.svc.List(r.Context())
	if err != nil {
		h.serverError(w, err)
		return
	}
	w.Header().Set("HX-Trigger", "product-saved")
	h.render(w, "products_tbody", map[string]any{"Products": products})
}

// render — 템플릿을 버퍼에 먼저 쓴 뒤 일괄 flush.
// 중간에 에러가 나면 반쪽짜리 응답이 나가지 않도록 방어.
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

// formData — 폼 템플릿용 공통 데이터 구조.
// Product 는 값이 채워진 상태(수정) 또는 제로값(등록).
// Errors 는 검증 실패 시에만 채워져 필드별 메시지 노출에 쓰인다.
func formData(p *models.Product, errs service.ValidationErrors) map[string]any {
	return map[string]any{
		"Product": p,
		"Errors":  errs,
	}
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

// parseProductForm — 폼 필드를 파싱해 Product 구조체로.
// type="number" required 덕분에 파싱 실패는 드물다.
func parseProductForm(r *http.Request) (*models.Product, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	categoryInt, err := strconv.Atoi(r.FormValue("category"))
	if err != nil {
		return nil, err
	}
	purchase, err := strconv.ParseInt(r.FormValue("purchase_price"), 10, 64)
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
		Category:      models.Category(categoryInt),
		Name:          r.FormValue("name"),
		PurchasePrice: purchase,
		SalePrice:     sale,
		StockQty:      stock,
	}, nil
}
