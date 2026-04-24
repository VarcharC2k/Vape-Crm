package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/varcharC2k/vape-crm/internal/models"
	"github.com/varcharC2k/vape-crm/internal/repository"
)

// ProductView — 화면 표시용으로 재고금액(재고수량 × 매입단가)을 미리 계산해 붙인 구조체.
// 템플릿에서 {{.StockAmount}} 로 바로 쓸 수 있다.
// *models.Product 를 임베딩하므로 기존 필드(.Name, .Category 등)는 그대로 접근 가능.
type ProductView struct {
	*models.Product
	StockAmount int64
}

// ProductService — repository 위에 유효성 검증과 비즈니스 규칙을 얹는다.
// 핸들러는 이 계층까지만 알고, repository 나 models 의 내부를 직접 건드리지 않는다.
type ProductService struct {
	repo *repository.ProductRepository
}

func NewProductService(repo *repository.ProductRepository) *ProductService {
	return &ProductService{repo: repo}
}

// ErrProductNotFound — 없는 ID 에 대한 조회/수정/삭제 시도.
// 핸들러에서 errors.Is 로 판별해 404 응답으로 바꾼다.
var ErrProductNotFound = errors.New("품목을 찾을 수 없습니다")

// ValidationErrors — 필드별 에러 메시지 모음.
// 폼 재렌더 시 {{.Errors.name}} 처럼 필드에 직접 꽂아 쓸 수 있다.
type ValidationErrors map[string]string

// Error — error 인터페이스 구현. 로그·디버깅용 합친 문자열.
func (v ValidationErrors) Error() string {
	parts := make([]string, 0, len(v))
	for k, msg := range v {
		parts = append(parts, fmt.Sprintf("%s: %s", k, msg))
	}
	return strings.Join(parts, ", ")
}

// List — 전체 품목을 화면용 뷰로 반환.
func (s *ProductService) List(ctx context.Context) ([]ProductView, error) {
	products, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]ProductView, 0, len(products))
	for _, p := range products {
		views = append(views, toView(p))
	}
	return views, nil
}

// Get — 단일 품목 조회. 없으면 ErrProductNotFound.
func (s *ProductService) Get(ctx context.Context, id int64) (*ProductView, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrProductNotFound
		}
		return nil, err
	}
	v := toView(p)
	return &v, nil
}

// Create — 검증 통과 시 등록. 이름 중복은 DB 단 UNIQUE 제약에서 걸리고,
// 여기서 한글 메시지로 감싸서 폼에 표시 가능하게 만든다.
func (s *ProductService) Create(ctx context.Context, p *models.Product) error {
	if errs := validate(p); len(errs) > 0 {
		return errs
	}
	if err := s.repo.Create(ctx, p); err != nil {
		if isUniqueViolation(err) {
			return ValidationErrors{"name": "이미 등록된 품목명입니다"}
		}
		return err
	}
	return nil
}

// Update — 검증 + 수정. 없는 ID 는 ErrProductNotFound.
func (s *ProductService) Update(ctx context.Context, p *models.Product) error {
	if errs := validate(p); len(errs) > 0 {
		return errs
	}
	err := s.repo.Update(ctx, p)
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrProductNotFound
	}
	if isUniqueViolation(err) {
		return ValidationErrors{"name": "이미 등록된 품목명입니다"}
	}
	return err
}

// Delete — 단순 삭제.
// 거래 기능 추가 시 "과거 거래 존재하면 거부" A안 가드를 여기에 추가한다.
func (s *ProductService) Delete(ctx context.Context, id int64) error {
	err := s.repo.Delete(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrProductNotFound
	}
	return err
}

// validate — 필드별 검증. 트림된 이름을 p 에 반영한다.
// 반환값 길이가 0 이면 에러 없음.
func validate(p *models.Product) ValidationErrors {
	errs := ValidationErrors{}

	name := strings.TrimSpace(p.Name)
	p.Name = name // 트리밍 결과를 호출측에 반영

	switch {
	case name == "":
		errs["name"] = "품목명은 필수입니다"
	case utf8.RuneCountInString(name) > 50:
		errs["name"] = "품목명은 50자를 넘을 수 없습니다"
	}

	if !p.Category.IsValid() {
		errs["category"] = "유효하지 않은 분류입니다"
	}
	if p.PurchasePrice < 0 {
		errs["purchase_price"] = "매입단가는 0 이상이어야 합니다"
	}
	if p.SalePrice < 0 {
		errs["sale_price"] = "매출단가는 0 이상이어야 합니다"
	}
	if p.StockQty < 0 {
		errs["stock_qty"] = "재고 수량은 0 이상이어야 합니다"
	}
	return errs
}

// isUniqueViolation — modernc.org/sqlite 의 UNIQUE 제약 위반을 메시지로 판별.
// 드라이버 고유 에러 코드를 타입 단언으로 뽑는 방법도 있지만,
// 드라이버 의존을 service 로 끌어오는 게 싫어서 단순 substring 매칭으로 처리.
// 메시지가 바뀌는 드문 경우엔 여기만 손보면 된다.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed: UNIQUE")
}

func toView(p *models.Product) ProductView {
	return ProductView{
		Product:     p,
		StockAmount: int64(p.StockQty) * p.PurchasePrice,
	}
}
