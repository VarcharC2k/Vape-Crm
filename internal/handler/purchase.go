package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/varcharC2k/vape-crm/internal/models"
	"github.com/varcharC2k/vape-crm/internal/repository"
	"github.com/varcharC2k/vape-crm/internal/service"
)

// PurchaseHandler — /purchases 경로들의 HTTP 처리.
//
// productSvc 의존: newForm 시 기본 분류의 품목 목록을 채워야 하고,
// editForm 시 기존 매입의 품목 분류를 알아내려고 사용한다.
type PurchaseHandler struct {
	svc        *service.PurchaseService
	productSvc *service.ProductService
	tmpl       *template.Template
}

func NewPurchaseHandler(
	svc *service.PurchaseService,
	productSvc *service.ProductService,
	tmpl *template.Template,
) *PurchaseHandler {
	return &PurchaseHandler{svc: svc, productSvc: productSvc, tmpl: tmpl}
}

// Register — 라우트 등록.
func (h *PurchaseHandler) Register(r chi.Router) {
	r.Route("/purchases", func(r chi.Router) {
		r.Get("/", h.list)
		r.Get("/new", h.newForm)
		r.Post("/", h.create)
		r.Get("/{id}/edit", h.editForm)
		r.Put("/{id}", h.update)
		r.Delete("/{id}", h.delete)
	})
}

// list — GET /purchases/
//   - 일반 브라우저 접근(HX-Request 헤더 없음): layout + 풀 페이지 렌더 (필터 비어있는 상태)
//   - HTMX 요청(필터 변경 등): tbody 파셜만 반환 (q_month 파라미터의 월 필터 적용)
func (h *PurchaseHandler) list(w http.ResponseWriter, r *http.Request) {
	if isHTMX(r) {
		h.renderTbody(w, r)
		return
	}
	purchases, err := h.svc.List(r.Context(), repository.PurchaseFilter{})
	if err != nil {
		h.serverError(w, err)
		return
	}
	h.render(w, "layout", map[string]any{
		"Title":     "매입 관리",
		"Purchases": purchases,
	})
}

// renderTbody — 필터 적용한 tbody 파셜 + 카운트 갱신.
func (h *PurchaseHandler) renderTbody(w http.ResponseWriter, r *http.Request) {
	purchases, err := h.svc.List(r.Context(), parsePurchaseFilter(r))
	if err != nil {
		h.serverError(w, err)
		return
	}
	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"purchase-count-changed": %d}`, len(purchases)))
	h.render(w, "purchases_tbody", map[string]any{"Purchases": purchases})
}

// newForm — 빈 폼을 모달 body 에 주입.
//
// 기본값:
//   - 날짜: 오늘
//   - 분류: 액상 (CategoryLiquid)
//   - 결제수단: 현금
//
// 품목이 한 건도 없으면 안내 메시지를 표시하고 폼은 띄우지 않는다.
func (h *PurchaseHandler) newForm(w http.ResponseWriter, r *http.Request) {
	allProducts, err := h.productSvc.List(r.Context(), repository.ProductFilter{})
	if err != nil {
		h.serverError(w, err)
		return
	}
	if len(allProducts) == 0 {
		h.render(w, "purchases_no_products", nil)
		return
	}

	cat := models.CategoryLiquid
	products, err := h.productSvc.List(r.Context(), repository.ProductFilter{Category: &cat})
	if err != nil {
		h.serverError(w, err)
		return
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	h.render(w, "purchases_form", map[string]any{
		"Purchase": &models.Purchase{
			TransactionDate: today,
			PaymentMethod:   "현금",
		},
		"Errors":           nil,
		"SelectedCategory": cat,
		"Products":         products,
	})
}

// editForm — 값이 채워진 수정용 폼.
// 기존 매입의 품목 분류를 그대로 SelectedCategory 로 사용한다 (Purchase 모델에 JOIN 으로 들어있음).
func (h *PurchaseHandler) editForm(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	purchase, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrPurchaseNotFound) {
			http.NotFound(w, r)
			return
		}
		h.serverError(w, err)
		return
	}

	cat := purchase.ProductCategory
	products, err := h.productSvc.List(r.Context(), repository.ProductFilter{Category: &cat})
	if err != nil {
		h.serverError(w, err)
		return
	}

	h.render(w, "purchases_form", map[string]any{
		"Purchase":         purchase,
		"Errors":           nil,
		"SelectedCategory": cat,
		"Products":         products,
	})
}

func (h *PurchaseHandler) create(w http.ResponseWriter, r *http.Request) {
	p, err := parsePurchaseForm(r)
	if err != nil {
		http.Error(w, "폼 파싱 실패: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.resolveProductID(r.Context(), p); err != nil {
		if h.tryRenderFormError(w, r, err, p) {
			return
		}
		h.serverError(w, err)
		return
	}
	if err := h.svc.Create(r.Context(), p); err != nil {
		if h.tryRenderFormError(w, r, err, p) {
			return
		}
		h.serverError(w, err)
		return
	}
	h.renderTbodyAndTrigger(w, r)
}

func (h *PurchaseHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	p, err := parsePurchaseForm(r)
	if err != nil {
		http.Error(w, "폼 파싱 실패: "+err.Error(), http.StatusBadRequest)
		return
	}
	p.ID = id
	if err := h.resolveProductID(r.Context(), p); err != nil {
		if h.tryRenderFormError(w, r, err, p) {
			return
		}
		h.serverError(w, err)
		return
	}
	if err := h.svc.Update(r.Context(), p); err != nil {
		if errors.Is(err, service.ErrPurchaseNotFound) {
			http.NotFound(w, r)
			return
		}
		if h.tryRenderFormError(w, r, err, p) {
			return
		}
		h.serverError(w, err)
		return
	}
	h.renderTbodyAndTrigger(w, r)
}

func (h *PurchaseHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		if errors.Is(err, service.ErrPurchaseNotFound) {
			http.NotFound(w, r)
			return
		}
		h.serverError(w, err)
		return
	}
	h.renderTbodyAndTrigger(w, r)
}

// tryRenderFormError — 검증 실패 시 폼을 모달에 다시 그린다.
// 이 때 분류·품목 후보 목록도 함께 다시 만들어 폼이 정상 표시되도록 한다.
func (h *PurchaseHandler) tryRenderFormError(w http.ResponseWriter, r *http.Request, err error, p *models.Purchase) bool {
	var valErrs service.ValidationErrors
	if !errors.As(err, &valErrs) {
		return false
	}

	// 사용자가 고른 분류는 폼에 같이 왔지만 (name="category"),
	// product_id 가 유효하면 그 품목의 분류를 그대로 쓰는 게 안전.
	cat := models.CategoryLiquid
	if p.ProductID > 0 {
		if prod, perr := h.productSvc.Get(r.Context(), p.ProductID); perr == nil {
			cat = prod.Category
		}
	} else if catStr := r.FormValue("category"); catStr != "" {
		if catInt, perr := strconv.Atoi(catStr); perr == nil {
			c := models.Category(catInt)
			if c.IsValid() {
				cat = c
			}
		}
	}

	products, perr := h.productSvc.List(r.Context(), repository.ProductFilter{Category: &cat})
	if perr != nil {
		h.serverError(w, perr)
		return true
	}

	w.Header().Set("HX-Retarget", "#modal-body")
	w.Header().Set("HX-Reswap", "innerHTML")
	w.WriteHeader(http.StatusUnprocessableEntity)
	h.render(w, "purchases_form", map[string]any{
		"Purchase":         p,
		"Errors":           valErrs,
		"SelectedCategory": cat,
		"Products":         products,
	})
	return true
}

func (h *PurchaseHandler) renderTbodyAndTrigger(w http.ResponseWriter, r *http.Request) {
	// CRUD 응답에도 현재 필터 유지 — 폼/삭제 버튼이 hx-include 로 q_month 를 함께 보냄.
	purchases, err := h.svc.List(r.Context(), parsePurchaseFilter(r))
	if err != nil {
		h.serverError(w, err)
		return
	}
	trigger := fmt.Sprintf(`{"purchase-saved": null, "purchase-count-changed": %d}`, len(purchases))
	w.Header().Set("HX-Trigger", trigger)
	h.render(w, "purchases_tbody", map[string]any{"Purchases": purchases})
}

func (h *PurchaseHandler) render(w http.ResponseWriter, name string, data any) {
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

func (h *PurchaseHandler) serverError(w http.ResponseWriter, err error) {
	log.Printf("서버 에러: %v", err)
	http.Error(w, "서버 오류", http.StatusInternalServerError)
}

// parsePurchaseFilter — 요청에서 매입 검색 필터를 추출.
//
// 현재는 q_month 만 지원 — '<input type="month">' 가 보내는 'YYYY-MM' 문자열.
// 빈 값이면 무필터 (전체 조회).
//
// 'YYYY-MM' 을 그 달의 첫째 날 ~ 마지막 날 범위로 변환해 PurchaseFilter 의
// DateFrom/DateTo 에 채운다. (repository.List 가 이미 날짜 범위 필터를 지원함)
//
// r.FormValue 는 GET 의 query string 과 POST/DELETE 의 form/query 양쪽을 모두 읽으므로,
// CRUD 요청에서 hx-include 로 함께 들어와도 동일하게 동작한다.
func parsePurchaseFilter(r *http.Request) repository.PurchaseFilter {
	var filter repository.PurchaseFilter

	monthStr := strings.TrimSpace(r.FormValue("q_month"))
	if monthStr == "" {
		return filter
	}

	t, err := time.Parse("2006-01", monthStr)
	if err != nil {
		return filter // 형식 오류면 무시 (조용히 전체 조회)
	}

	year, month := t.Year(), t.Month()
	from := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	// 다음달 1일에서 하루 빼면 이번달 마지막 날
	to := time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	filter.DateFrom = &from
	filter.DateTo = &to
	return filter
}

// parsePurchaseForm — 폼 필드를 파싱해 Purchase 구조체로.
//
// "category" 필드는 UI 헬퍼라 무시한다.
// "product_name" 은 사용자가 datalist 자동완성으로 입력한 품목명.
// 호출 측에서 이름 -> ID 로 변환해야 한다 (resolveProductID).
// ProductName 을 채워서 폼 재렌더 시에도 입력값이 보존되도록 한다.
func parsePurchaseForm(r *http.Request) (*models.Purchase, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}

	dateStr := r.FormValue("transaction_date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return nil, fmt.Errorf("날짜 파싱 실패: %w", err)
	}

	quantity, err := strconv.Atoi(r.FormValue("quantity"))
	if err != nil {
		return nil, fmt.Errorf("수량 파싱 실패: %w", err)
	}

	return &models.Purchase{
		TransactionDate: date,
		ProductName:     strings.TrimSpace(r.FormValue("product_name")),
		ProductID:       0, // 핸들러가 ProductName -> ID 로 변환 후 채움
		Quantity:        quantity,
		PaymentMethod:   r.FormValue("payment_method"),
		Memo:            r.FormValue("memo"),
	}, nil
}

// resolveProductID — 폼의 product_name 을 실제 ID 로 변환.
// 빈 문자열은 0 반환 (svc 검증이 빈 ProductID 를 잡아준다).
// 존재하지 않는 이름이면 ValidationErrors 로 폼에 에러 표시.
func (h *PurchaseHandler) resolveProductID(ctx context.Context, p *models.Purchase) error {
	if p.ProductName == "" {
		return nil // svc.Create/Update 의 ProductID <= 0 검증에 위임
	}
	prod, err := h.productSvc.GetByName(ctx, p.ProductName)
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			return service.ValidationErrors{
				"product_name": fmt.Sprintf("'%s' 은(는) 등록된 품목이 아닙니다", p.ProductName),
			}
		}
		return err
	}
	p.ProductID = prod.ID
	return nil
}
