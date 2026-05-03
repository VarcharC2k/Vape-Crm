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

// SaleService — 매출 거래 비즈니스 로직.
//
// 핵심 책임: 매출 등록·수정·삭제 시 products.stock_qty 를 트랜잭션으로 정정한다.
// 음수 재고는 사용자 결정으로 허용 (재고 부족 검증 안 함).
//
// 매입 패턴(PurchaseService) 과 거의 동일하지만 재고 부호가 반대.
type SaleService struct {
	db       *sql.DB
	sales    *repository.SaleRepository
	products *repository.ProductRepository
}

func NewSaleService(
	db *sql.DB,
	sales *repository.SaleRepository,
	products *repository.ProductRepository,
) *SaleService {
	return &SaleService{db: db, sales: sales, products: products}
}

// ErrSaleNotFound — 없는 ID 에 대한 조회/수정/삭제.
var ErrSaleNotFound = errors.New("매출을 찾을 수 없습니다")

// List/Get — 단순 위임.
func (s *SaleService) List(ctx context.Context, filter repository.SaleFilter) ([]*models.Sale, error) {
	return s.sales.List(ctx, filter)
}

func (s *SaleService) Get(ctx context.Context, id int64) (*models.Sale, error) {
	sale, err := s.sales.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSaleNotFound
		}
		return nil, err
	}
	return sale, nil
}

// Create — 트랜잭션:
//  1. INSERT sale
//  2. UPDATE products.stock_qty -= quantity
//
// 음수 재고는 허용 (사용자 결정). 재고 검증 안 함.
func (s *SaleService) Create(ctx context.Context, sale *models.Sale) error {
	if errs := validateSale(sale); len(errs) > 0 {
		return errs
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		if err := s.sales.Create(ctx, tx, sale); err != nil {
			return err
		}
		// 매출은 재고 차감 (negative delta)
		return s.products.AdjustStock(ctx, tx, sale.ProductID, -sale.Quantity)
	})
}

// Update — 트랜잭션:
//  1. 이전 매출 조회 (트랜잭션 안)
//  2. UPDATE sale
//  3. 재고 정정 (매입과 부호 반대)
//
// 재고 정정 규칙 (매출 기준):
//   - 같은 품목: stock += (oldQty - newQty)
//     · oldQty > newQty (예: 5→3): 더 적게 팔았으니 stock += 2 (복구)
//     · oldQty < newQty (예: 5→7): 더 많이 팔았으니 stock -= 2 (추가 차감)
//   - 품목 변경: oldProduct.stock += oldQty (복구), newProduct.stock -= newQty (차감)
func (s *SaleService) Update(ctx context.Context, sale *models.Sale) error {
	if errs := validateSale(sale); len(errs) > 0 {
		return errs
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		old, err := s.sales.GetByIDTx(ctx, tx, sale.ID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrSaleNotFound
			}
			return err
		}
		if err := s.sales.Update(ctx, tx, sale); err != nil {
			return err
		}
		if old.ProductID == sale.ProductID {
			delta := old.Quantity - sale.Quantity // 매출 부호: 매입과 반대
			if delta == 0 {
				return nil
			}
			return s.products.AdjustStock(ctx, tx, sale.ProductID, delta)
		}
		// 품목 변경: 이전 품목 복구 + 새 품목 차감
		if err := s.products.AdjustStock(ctx, tx, old.ProductID, old.Quantity); err != nil {
			return err
		}
		return s.products.AdjustStock(ctx, tx, sale.ProductID, -sale.Quantity)
	})
}

// Delete — 트랜잭션:
//  1. 매출 조회 (수량·품목 알아내기)
//  2. DELETE sale
//  3. UPDATE products.stock_qty += quantity (복구)
func (s *SaleService) Delete(ctx context.Context, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		old, err := s.sales.GetByIDTx(ctx, tx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrSaleNotFound
			}
			return err
		}
		if err := s.sales.Delete(ctx, tx, id); err != nil {
			return err
		}
		// 매출 삭제 → 재고 복구
		return s.products.AdjustStock(ctx, tx, old.ProductID, old.Quantity)
	})
}

// tx — 트랜잭션 헬퍼 (PurchaseService 와 동일).
func (s *SaleService) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("트랜잭션 시작 실패: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// validateSale — 필드별 검증. 트림된 값을 반영.
//
// 고객 정보는 선택 (비회원 거래 OK). 형식 검증은 안 함 (매장 자율 + 클라단 입력).
func validateSale(sale *models.Sale) ValidationErrors {
	errs := ValidationErrors{}

	if sale.TransactionDate.IsZero() {
		errs["transaction_date"] = "날짜는 필수입니다"
	}
	if sale.ProductID <= 0 {
		errs["product_id"] = "품목을 입력하세요"
	}
	if sale.Quantity <= 0 {
		errs["quantity"] = "수량은 1 이상이어야 합니다"
	}

	pm := strings.TrimSpace(sale.PaymentMethod)
	sale.PaymentMethod = pm
	switch {
	case pm == "":
		errs["payment_method"] = "결제수단은 필수입니다"
	case utf8.RuneCountInString(pm) > 10:
		errs["payment_method"] = "결제수단은 10자를 넘을 수 없습니다"
	}

	customerName := strings.TrimSpace(sale.CustomerName)
	sale.CustomerName = customerName
	if utf8.RuneCountInString(customerName) > 50 {
		errs["customer_name"] = "고객명은 50자를 넘을 수 없습니다"
	}

	customerMobile := strings.TrimSpace(sale.CustomerMobile)
	sale.CustomerMobile = customerMobile
	if utf8.RuneCountInString(customerMobile) > 20 {
		errs["customer_mobile"] = "휴대폰 번호는 20자를 넘을 수 없습니다"
	}

	if utf8.RuneCountInString(sale.Memo) > 500 {
		errs["memo"] = "비고는 500자를 넘을 수 없습니다"
	}

	return errs
}
