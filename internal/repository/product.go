package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

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

// ProductFilter — List 조회 시 적용할 필터.
// 빈 값(nil 또는 "") 인 항목은 필터링하지 않는다.
type ProductFilter struct {
	Category *models.Category // nil 이면 전체 분류
	Name     string           // 빈 문자열이면 전체 이름 (앞뒤 공백은 트림됨)
}

// IsEmpty — 어떤 필터도 걸려있지 않은지.
func (f ProductFilter) IsEmpty() bool {
	return f.Category == nil && strings.TrimSpace(f.Name) == ""
}

// Create — 신규 품목을 등록하고 생성된 ID 를 p.ID 에 채워 넣는다.
// name UNIQUE 제약 위반 시 DB 드라이버가 에러를 반환하므로 중복 등록은 여기서 막힌다.
func (r *ProductRepository) Create(ctx context.Context, p *models.Product) error {
	const q = `
		INSERT INTO products (category, name, sale_price, stock_qty)
		VALUES (?, ?, ?, ?)
	`
	res, err := r.db.ExecContext(ctx, q,
		int(p.Category), p.Name, p.SalePrice, p.StockQty,
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
func (r *ProductRepository) GetByID(ctx context.Context, id int64) (*models.Product, error) {
	const q = `
		SELECT id, category, name, sale_price, stock_qty, created_at
		FROM products
		WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, q, id)
	return scanProduct(row)
}

// List — 필터 조건에 맞는 품목 목록을 반환한다.
//
// 정렬 규칙(CLAUDE.md 의 부분일치 검색 표준):
//   - 이름 검색이 있으면: '검색어%' 매칭(앞쪽 일치)을 1순위, 그 외 부분 일치를 2순위, 각 그룹 내에서 이름 ASC.
//   - 이름 검색이 없으면: 단순히 이름 ASC.
//
// 필터가 비어있으면 전체 목록을 이름 오름차순으로 반환.
func (r *ProductRepository) List(ctx context.Context, filter ProductFilter) ([]*models.Product, error) {
	var (
		wheres []string
		args   []any
	)

	if filter.Category != nil {
		wheres = append(wheres, "category = ?")
		args = append(args, int(*filter.Category))
	}

	name := strings.TrimSpace(filter.Name)
	if name != "" {
		wheres = append(wheres, "name LIKE '%' || ? || '%'")
		args = append(args, name)
	}

	q := `SELECT id, category, name, sale_price, stock_qty, created_at FROM products`
	if len(wheres) > 0 {
		q += " WHERE " + strings.Join(wheres, " AND ")
	}

	if name != "" {
		q += " ORDER BY CASE WHEN name LIKE ? || '%' THEN 1 ELSE 2 END, name ASC"
		args = append(args, name)
	} else {
		q += " ORDER BY name ASC"
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
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

// Update — 모든 수정 가능한 필드(분류/이름/매출단가/재고)를 한 번에 갱신한다.
// 없는 ID 를 수정하려 하면 sql.ErrNoRows 를 반환한다.
func (r *ProductRepository) Update(ctx context.Context, p *models.Product) error {
	const q = `
		UPDATE products
		SET category = ?, name = ?, sale_price = ?, stock_qty = ?
		WHERE id = ?
	`
	res, err := r.db.ExecContext(ctx, q,
		int(p.Category), p.Name, p.SalePrice, p.StockQty, p.ID,
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

// AdjustStock — products.stock_qty 를 delta 만큼 가감한다.
// delta 는 양수(매입·매출 취소) 또는 음수(매출·매입 취소) 모두 가능.
//
// 항상 트랜잭션 안에서 호출되도록 *sql.Tx 를 받는다 — 매입 등록·매출 등록 등에서
// 거래 INSERT 와 재고 UPDATE 가 원자적으로 묶여야 하기 때문.
//
// 음수 재고는 SQLite 가 막아주지 않는다. 비즈니스적으로 음수가 안 되도록
// service 단에서 사전에 계산·검증해야 한다.
func (r *ProductRepository) AdjustStock(ctx context.Context, tx *sql.Tx, productID int64, delta int) error {
	res, err := tx.ExecContext(ctx,
		"UPDATE products SET stock_qty = stock_qty + ? WHERE id = ?",
		delta, productID,
	)
	if err != nil {
		return fmt.Errorf("재고 조정 실패: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("재고 조정 반영 행 수 조회 실패: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// rowScanner — *sql.Row 와 *sql.Rows 가 공통으로 구현하는 Scan 메서드를 추상화한다.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanProduct — 한 행을 Product 구조체로 채운다.
func scanProduct(s rowScanner) (*models.Product, error) {
	var p models.Product
	var categoryInt int
	if err := s.Scan(
		&p.ID,
		&categoryInt,
		&p.Name,
		&p.SalePrice,
		&p.StockQty,
		&p.CreatedAt,
	); err != nil {
		return nil, err
	}
	p.Category = models.Category(categoryInt)
	return &p, nil
}
