package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/varcharC2k/vape-crm/internal/models"
)

// ProductRepository — products 테이블에 대한 CRUD 접근 계층.
// 비즈니스 로직(유효성 검증, 트랜잭션 묶음 등)은 service 계층이 담당한다.
// 이 파일은 DB 왕복만 책임진다.
type ProductRepository struct {
	db *sql.DB
}

// NewProductRepository — 핸들러/서비스에서 주입받아 쓸 수 있도록 생성자를 제공한다.
func NewProductRepository(db *sql.DB) *ProductRepository {
	return &ProductRepository{db: db}
}

// Create — 신규 품목을 등록하고 생성된 ID 를 p.ID 에 채워 넣는다.
// name UNIQUE 제약 위반 시 DB 드라이버가 에러를 반환하므로 중복 등록은 여기서 막힌다.
func (r *ProductRepository) Create(ctx context.Context, p *models.Product) error {
	const q = `
		INSERT INTO products (category, name, purchase_price, sale_price, stock_qty)
		VALUES (?, ?, ?, ?, ?)
	`
	res, err := r.db.ExecContext(ctx, q,
		int(p.Category), p.Name, p.PurchasePrice, p.SalePrice, p.StockQty,
	)
	if err != nil {
		return fmt.Errorf("품목 등록 실패: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("품목 ID 조회 실패: %w", err)
	}
	p.ID = id
	return nil
}

// GetByID — id 로 단일 품목 조회. 없으면 sql.ErrNoRows 를 그대로 반환한다.
// 호출 측에서 errors.Is(err, sql.ErrNoRows) 로 "없음"을 판별하는 게 관용구.
func (r *ProductRepository) GetByID(ctx context.Context, id int64) (*models.Product, error) {
	const q = `
		SELECT id, category, name, purchase_price, sale_price, stock_qty, created_at
		FROM products
		WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, q, id)
	return scanProduct(row)
}

// List — 전체 품목을 품목명 오름차순으로 반환한다.
// 분류 필터는 별도 메서드(또는 쿼리 파라미터 확장)로 추가 예정.
func (r *ProductRepository) List(ctx context.Context) ([]*models.Product, error) {
	const q = `
		SELECT id, category, name, purchase_price, sale_price, stock_qty, created_at
		FROM products
		ORDER BY name ASC
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("품목 목록 조회 실패: %w", err)
	}
	defer rows.Close()

	var products []*models.Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("품목 목록 스캔 실패: %w", err)
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("품목 목록 반복 실패: %w", err)
	}
	return products, nil
}

// Update — 모든 수정 가능한 필드(분류/이름/단가/재고)를 한 번에 갱신한다.
// 없는 ID 를 수정하려 하면 sql.ErrNoRows 를 반환한다.
func (r *ProductRepository) Update(ctx context.Context, p *models.Product) error {
	const q = `
		UPDATE products
		SET category = ?, name = ?, purchase_price = ?, sale_price = ?, stock_qty = ?
		WHERE id = ?
	`
	res, err := r.db.ExecContext(ctx, q,
		int(p.Category), p.Name, p.PurchasePrice, p.SalePrice, p.StockQty, p.ID,
	)
	if err != nil {
		return fmt.Errorf("품목 수정 실패: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("품목 수정 반영 행 수 조회 실패: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Delete — id 로 물리 삭제.
// 현재는 purchases/sales 테이블이 아직 없어 단순 삭제만 수행한다.
// 거래 기능(Phase 4) 도입 시 "과거 거래가 존재하면 삭제 거부" 가드를
// service 계층에서 추가할 예정 (A안).
func (r *ProductRepository) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM products WHERE id = ?`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("품목 삭제 실패: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("품목 삭제 반영 행 수 조회 실패: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// rowScanner — *sql.Row 와 *sql.Rows 가 공통으로 구현하는 Scan 메서드를 추상화한다.
// 덕분에 scanProduct 하나로 GetByID(단일)와 List(반복) 양쪽을 처리할 수 있다.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanProduct — 한 행을 Product 구조체로 채운다.
// DB 의 category 정수를 models.Category 로 캐스팅만 하고,
// 유효 범위 검사는 호출 측 책임(보통은 repository 신뢰 가능 — 저장 시점에 검증했으므로).
func scanProduct(s rowScanner) (*models.Product, error) {
	var p models.Product
	var categoryInt int
	if err := s.Scan(
		&p.ID,
		&categoryInt,
		&p.Name,
		&p.PurchasePrice,
		&p.SalePrice,
		&p.StockQty,
		&p.CreatedAt,
	); err != nil {
		return nil, err
	}
	p.Category = models.Category(categoryInt)
	return &p, nil
}
