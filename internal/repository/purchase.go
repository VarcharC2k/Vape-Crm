package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/varcharC2k/vape-crm/internal/models"
)

// PurchaseRepository — purchases 테이블 CRUD.
//
// 쓰기 메서드(Create/Update/Delete) 는 *sql.Tx 를 받는다.
// 매입은 항상 재고 조정과 함께 트랜잭션에 묶이기 때문.
//
// 읽기 메서드(GetByID/List) 는 r.db 를 직접 사용한다.
// 트랜잭션 안에서 이전 상태를 읽어야 할 때(Update/Delete 직전) 는 GetByIDTx 사용.
type PurchaseRepository struct {
	db *sql.DB
}

func NewPurchaseRepository(db *sql.DB) *PurchaseRepository {
	return &PurchaseRepository{db: db}
}

// PurchaseFilter — 검색 필터.
// 빈 값(nil) 인 항목은 필터링하지 않는다.
type PurchaseFilter struct {
	DateFrom  *time.Time // 포함, nil 이면 무제한 과거
	DateTo    *time.Time // 포함, nil 이면 무제한 미래
	ProductID *int64     // nil 이면 전체 품목
}

// Create — *sql.Tx 안에서 INSERT.
// 호출 측 service 가 같은 트랜잭션에서 products.stock_qty 도 가산한다.
func (r *PurchaseRepository) Create(ctx context.Context, tx *sql.Tx, p *models.Purchase) error {
	const q = `
		INSERT INTO purchases (transaction_date, product_id, quantity, amount, payment_method, memo)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	res, err := tx.ExecContext(ctx, q,
		p.TransactionDate.Format("2006-01-02"),
		p.ProductID,
		p.Quantity,
		p.Amount,
		p.PaymentMethod,
		p.Memo,
	)
	if err != nil {
		return fmt.Errorf("매입 등록 실패: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("매입 ID 조회 실패: %w", err)
	}
	p.ID = id
	return nil
}

// GetByID — 단일 매입 조회 (품목명 JOIN 포함).
// 없으면 sql.ErrNoRows.
func (r *PurchaseRepository) GetByID(ctx context.Context, id int64) (*models.Purchase, error) {
	return r.getByID(ctx, r.db, id)
}

// GetByIDTx — 트랜잭션 안에서 단일 매입 조회.
// Update/Delete 시 이전 품목·수량을 같은 트랜잭션에서 읽어 재고 정정 계산에 사용.
func (r *PurchaseRepository) GetByIDTx(ctx context.Context, tx *sql.Tx, id int64) (*models.Purchase, error) {
	return r.getByID(ctx, tx, id)
}

// rowQuerier — *sql.DB 와 *sql.Tx 가 공통으로 만족하는 메서드 집합.
// getByID 의 공통 구현을 두 컨텍스트에서 재사용하기 위함.
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (r *PurchaseRepository) getByID(ctx context.Context, q rowQuerier, id int64) (*models.Purchase, error) {
	const sqlStr = `
		SELECT p.id, p.transaction_date, p.product_id, prod.name,
		       p.quantity, p.amount, p.payment_method, p.memo, p.created_at
		FROM purchases p
		JOIN products prod ON prod.id = p.product_id
		WHERE p.id = ?
	`
	row := q.QueryRowContext(ctx, sqlStr, id)
	return scanPurchase(row)
}

// List — 필터 조건에 맞는 매입 목록을 반환.
// 정렬: 거래일 DESC, 같은 날짜는 id DESC (최근 등록이 위로).
func (r *PurchaseRepository) List(ctx context.Context, filter PurchaseFilter) ([]*models.Purchase, error) {
	var (
		wheres []string
		args   []any
	)

	if filter.DateFrom != nil {
		wheres = append(wheres, "p.transaction_date >= ?")
		args = append(args, filter.DateFrom.Format("2006-01-02"))
	}
	if filter.DateTo != nil {
		wheres = append(wheres, "p.transaction_date <= ?")
		args = append(args, filter.DateTo.Format("2006-01-02"))
	}
	if filter.ProductID != nil {
		wheres = append(wheres, "p.product_id = ?")
		args = append(args, *filter.ProductID)
	}

	sqlStr := `
		SELECT p.id, p.transaction_date, p.product_id, prod.name,
		       p.quantity, p.amount, p.payment_method, p.memo, p.created_at
		FROM purchases p
		JOIN products prod ON prod.id = p.product_id
	`
	if len(wheres) > 0 {
		sqlStr += " WHERE " + strings.Join(wheres, " AND ")
	}
	sqlStr += " ORDER BY p.transaction_date DESC, p.id DESC"

	rows, err := r.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("매입 목록 조회 실패: %w", err)
	}
	defer rows.Close()

	var purchases []*models.Purchase
	for rows.Next() {
		p, err := scanPurchase(rows)
		if err != nil {
			return nil, fmt.Errorf("매입 목록 스캔 실패: %w", err)
		}
		purchases = append(purchases, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("매입 목록 반복 실패: %w", err)
	}
	return purchases, nil
}

// Update — *sql.Tx 안에서 UPDATE.
// 재고 정정은 service 가 같은 트랜잭션에서 수행한다.
func (r *PurchaseRepository) Update(ctx context.Context, tx *sql.Tx, p *models.Purchase) error {
	const q = `
		UPDATE purchases
		SET transaction_date = ?, product_id = ?, quantity = ?, amount = ?, payment_method = ?, memo = ?
		WHERE id = ?
	`
	res, err := tx.ExecContext(ctx, q,
		p.TransactionDate.Format("2006-01-02"),
		p.ProductID,
		p.Quantity,
		p.Amount,
		p.PaymentMethod,
		p.Memo,
		p.ID,
	)
	if err != nil {
		return fmt.Errorf("매입 수정 실패: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("매입 수정 반영 행 수 조회 실패: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Delete — *sql.Tx 안에서 DELETE.
// 재고 차감은 service 가 같은 트랜잭션에서 수행한다.
func (r *PurchaseRepository) Delete(ctx context.Context, tx *sql.Tx, id int64) error {
	res, err := tx.ExecContext(ctx, "DELETE FROM purchases WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("매입 삭제 실패: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("매입 삭제 반영 행 수 조회 실패: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// scanPurchase — row/rows 한 행을 Purchase 구조체로 채운다.
//
// 날짜 컬럼은 time.Time 으로 직접 스캔한다.
// modernc.org/sqlite 가 DATE/DATETIME 컬럼을 RFC3339 로 정규화해서 돌려주기 때문에
// time.Time 스캔이 가장 안전하다. (string 으로 받으면 'YYYY-MM-DDT00:00:00Z' 같은
// 추가된 시간 부분이 따라와서 직접 파싱할 때 layout 이 맞지 않는다.)
//
// memo 는 NULL 허용 컬럼이라 sql.NullString 으로 받는다.
func scanPurchase(s rowScanner) (*models.Purchase, error) {
	var p models.Purchase
	var memo sql.NullString
	if err := s.Scan(
		&p.ID,
		&p.TransactionDate,
		&p.ProductID,
		&p.ProductName,
		&p.Quantity,
		&p.Amount,
		&p.PaymentMethod,
		&memo,
		&p.CreatedAt,
	); err != nil {
		return nil, err
	}
	if memo.Valid {
		p.Memo = memo.String
	}
	return &p, nil
}
