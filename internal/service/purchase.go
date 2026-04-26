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

// PurchaseService — 매입 거래 비즈니스 로직.
//
// 핵심 책임: 매입 등록·수정·삭제 시 products.stock_qty 를 트랜잭션으로 정정한다.
// repo 두 개(purchases, products) 를 같은 *sql.Tx 안에서 호출하기 위해 *sql.DB 도 보유한다.
type PurchaseService struct {
	db        *sql.DB
	purchases *repository.PurchaseRepository
	products  *repository.ProductRepository
}

func NewPurchaseService(
	db *sql.DB,
	purchases *repository.PurchaseRepository,
	products *repository.ProductRepository,
) *PurchaseService {
	return &PurchaseService{db: db, purchases: purchases, products: products}
}

// ErrPurchaseNotFound — 없는 ID 에 대한 조회/수정/삭제.
var ErrPurchaseNotFound = errors.New("매입을 찾을 수 없습니다")

// List/Get — 단순 위임 (트랜잭션 불필요).
func (s *PurchaseService) List(ctx context.Context, filter repository.PurchaseFilter) ([]*models.Purchase, error) {
	return s.purchases.List(ctx, filter)
}

func (s *PurchaseService) Get(ctx context.Context, id int64) (*models.Purchase, error) {
	p, err := s.purchases.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPurchaseNotFound
		}
		return nil, err
	}
	return p, nil
}

// Create — 트랜잭션:
//  1. INSERT purchase
//  2. UPDATE products.stock_qty += quantity
//
// 둘 중 하나라도 실패하면 둘 다 rollback. 매입은 등록됐는데 재고는 안 늘어나는 사고를 막는다.
func (s *PurchaseService) Create(ctx context.Context, p *models.Purchase) error {
	if errs := validatePurchase(p); len(errs) > 0 {
		return errs
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		if err := s.purchases.Create(ctx, tx, p); err != nil {
			return err
		}
		return s.products.AdjustStock(ctx, tx, p.ProductID, p.Quantity)
	})
}

// Update — 트랜잭션:
//  1. 이전 매입 조회 (트랜잭션 안)
//  2. UPDATE purchase
//  3. 재고 정정
//
// 재고 정정 규칙:
//   - 같은 품목: stock += (newQty - oldQty)
//   - 품목 변경: oldProduct.stock -= oldQty 그리고 newProduct.stock += newQty
func (s *PurchaseService) Update(ctx context.Context, p *models.Purchase) error {
	if errs := validatePurchase(p); len(errs) > 0 {
		return errs
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		old, err := s.purchases.GetByIDTx(ctx, tx, p.ID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrPurchaseNotFound
			}
			return err
		}
		if err := s.purchases.Update(ctx, tx, p); err != nil {
			return err
		}
		if old.ProductID == p.ProductID {
			delta := p.Quantity - old.Quantity
			if delta == 0 {
				return nil
			}
			return s.products.AdjustStock(ctx, tx, p.ProductID, delta)
		}
		// 품목 변경: 이전 품목 재고 차감 후 새 품목 재고 가산.
		if err := s.products.AdjustStock(ctx, tx, old.ProductID, -old.Quantity); err != nil {
			return err
		}
		return s.products.AdjustStock(ctx, tx, p.ProductID, p.Quantity)
	})
}

// Delete — 트랜잭션:
//  1. 매입 조회 (수량·품목 알아내기)
//  2. DELETE purchase
//  3. UPDATE products.stock_qty -= quantity
func (s *PurchaseService) Delete(ctx context.Context, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		old, err := s.purchases.GetByIDTx(ctx, tx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrPurchaseNotFound
			}
			return err
		}
		if err := s.purchases.Delete(ctx, tx, id); err != nil {
			return err
		}
		return s.products.AdjustStock(ctx, tx, old.ProductID, -old.Quantity)
	})
}

// tx — 트랜잭션 헬퍼.
// fn 이 nil 을 반환하면 commit, 아니면 rollback.
// defer Rollback 은 commit 후 호출되어도 안전 (이미 종료된 트랜잭션은 no-op).
func (s *PurchaseService) tx(ctx context.Context, fn func(*sql.Tx) error) error {
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

// validatePurchase — 필드별 검증. 트림된 결제수단을 p 에 반영.
func validatePurchase(p *models.Purchase) ValidationErrors {
	errs := ValidationErrors{}

	if p.TransactionDate.IsZero() {
		errs["transaction_date"] = "날짜는 필수입니다"
	}
	if p.ProductID <= 0 {
		errs["product_id"] = "품목을 선택하세요"
	}
	if p.Quantity <= 0 {
		errs["quantity"] = "수량은 1 이상이어야 합니다"
	}
	if p.Amount < 0 {
		errs["amount"] = "매입금액은 0 이상이어야 합니다"
	}

	pm := strings.TrimSpace(p.PaymentMethod)
	p.PaymentMethod = pm
	switch {
	case pm == "":
		errs["payment_method"] = "결제수단은 필수입니다"
	case utf8.RuneCountInString(pm) > 10:
		errs["payment_method"] = "결제수단은 10자를 넘을 수 없습니다"
	}

	if utf8.RuneCountInString(p.Memo) > 500 {
		errs["memo"] = "비고는 500자를 넘을 수 없습니다"
	}

	return errs
}
