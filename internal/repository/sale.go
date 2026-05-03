package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/varcharC2k/vape-crm/internal/models"
)

// SaleRepository — sales 테이블 CRUD.
//
// 쓰기 메서드(Create/Update/Delete) 는 *sql.Tx 를 받는다 — 매출은 항상 재고 차감과 함께
// 트랜잭션에 묶이기 때문 (purchase 와 동일 패턴).
//
// 읽기 메서드(GetByID/List) 는 r.db 직접 사용.
// 트랜잭션 안에서 이전 상태를 읽어야 할 때(Update/Delete 직전) 는 GetByIDTx 사용.
type SaleRepository struct {
	db *sql.DB
}

func NewSaleRepository(db *sql.DB) *SaleRepository {
	return &SaleRepository{db: db}
}

// SaleFilter — 검색 필터.
//   - 날짜 범위, 품목 ID 는 정확 매칭
//   - 고객명/휴대폰은 부분일치 (스냅샷 컬럼 기준)
type SaleFilter struct {
	DateFrom       *time.Time // 포함, nil = 무제한 과거
	DateTo         *time.Time // 포함, nil = 무제한 미래
	ProductID      *int64     // nil = 전체 품목
	CustomerName   string     // 빈 문자열 = 전체 (스냅샷 LIKE)
	CustomerMobile string     // 빈 문자열 = 전체 (스냅샷 LIKE)
}

// Create — *sql.Tx 안에서 INSERT.
// 호출 측 service 가 같은 트랜잭션에서 products.stock_qty -= quantity 를 수행한다.
func (r *SaleRepository) Create(ctx context.Context, tx *sql.Tx, s *models.Sale) error {
	const q = `
		INSERT INTO sales (transaction_date, product_id, customer_name, customer_mobile,
		                   quantity, payment_method, memo)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	res, err := tx.ExecContext(ctx, q,
		s.TransactionDate.Format("2006-01-02"),
		s.ProductID,
		nullStr(s.CustomerName),
		nullStr(s.CustomerMobile),
		s.Quantity,
		s.PaymentMethod,
		nullStr(s.Memo),
	)
	if err != nil {
		return fmt.Errorf("매출 등록 실패: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("매출 ID 조회 실패: %w", err)
	}
	s.ID = id
	return nil
}

// GetByID — 단일 매출 조회 (품목명·분류 JOIN 포함).
func (r *SaleRepository) GetByID(ctx context.Context, id int64) (*models.Sale, error) {
	return r.getByID(ctx, r.db, id)
}

// GetByIDTx — 트랜잭션 안에서 단일 매출 조회.
// Update/Delete 시 이전 품목·수량을 같은 트랜잭션에서 읽어 재고 정정 계산에 사용.
func (r *SaleRepository) GetByIDTx(ctx context.Context, tx *sql.Tx, id int64) (*models.Sale, error) {
	return r.getByID(ctx, tx, id)
}

func (r *SaleRepository) getByID(ctx context.Context, q rowQuerier, id int64) (*models.Sale, error) {
	const sqlStr = `
		SELECT s.id, s.transaction_date, s.product_id, prod.name, prod.category,
		       s.customer_name, s.customer_mobile,
		       s.quantity, s.payment_method, s.memo, s.created_at
		FROM sales s
		JOIN products prod ON prod.id = s.product_id
		WHERE s.id = ?
	`
	row := q.QueryRowContext(ctx, sqlStr, id)
	return scanSale(row)
}

// List — 필터 조건에 맞는 매출 목록을 반환.
// 정렬: 거래일 DESC, 같은 날짜는 id DESC (최근 등록이 위로) — 매입과 동일.
func (r *SaleRepository) List(ctx context.Context, filter SaleFilter) ([]*models.Sale, error) {
	var (
		wheres []string
		args   []any
	)

	if filter.DateFrom != nil {
		wheres = append(wheres, "s.transaction_date >= ?")
		args = append(args, filter.DateFrom.Format("2006-01-02"))
	}
	if filter.DateTo != nil {
		wheres = append(wheres, "s.transaction_date <= ?")
		args = append(args, filter.DateTo.Format("2006-01-02"))
	}
	if filter.ProductID != nil {
		wheres = append(wheres, "s.product_id = ?")
		args = append(args, *filter.ProductID)
	}

	name := strings.TrimSpace(filter.CustomerName)
	if name != "" {
		wheres = append(wheres, "s.customer_name LIKE '%' || ? || '%'")
		args = append(args, name)
	}
	mobile := strings.TrimSpace(filter.CustomerMobile)
	if mobile != "" {
		wheres = append(wheres, "s.customer_mobile LIKE '%' || ? || '%'")
		args = append(args, mobile)
	}

	sqlStr := `
		SELECT s.id, s.transaction_date, s.product_id, prod.name, prod.category,
		       s.customer_name, s.customer_mobile,
		       s.quantity, s.payment_method, s.memo, s.created_at
		FROM sales s
		JOIN products prod ON prod.id = s.product_id
	`
	if len(wheres) > 0 {
		sqlStr += " WHERE " + strings.Join(wheres, " AND ")
	}
	sqlStr += " ORDER BY s.transaction_date DESC, s.id DESC"

	rows, err := r.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("매출 목록 조회 실패: %w", err)
	}
	defer rows.Close()

	var sales []*models.Sale
	for rows.Next() {
		s, err := scanSale(rows)
		if err != nil {
			return nil, fmt.Errorf("매출 목록 스캔 실패: %w", err)
		}
		sales = append(sales, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("매출 목록 반복 실패: %w", err)
	}
	return sales, nil
}

// Update — *sql.Tx 안에서 UPDATE. 재고 정정은 service 가 같은 트랜잭션에서 수행.
func (r *SaleRepository) Update(ctx context.Context, tx *sql.Tx, s *models.Sale) error {
	const q = `
		UPDATE sales
		SET transaction_date = ?, product_id = ?,
		    customer_name = ?, customer_mobile = ?,
		    quantity = ?, payment_method = ?, memo = ?
		WHERE id = ?
	`
	res, err := tx.ExecContext(ctx, q,
		s.TransactionDate.Format("2006-01-02"),
		s.ProductID,
		nullStr(s.CustomerName),
		nullStr(s.CustomerMobile),
		s.Quantity,
		s.PaymentMethod,
		nullStr(s.Memo),
		s.ID,
	)
	if err != nil {
		return fmt.Errorf("매출 수정 실패: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("매출 수정 반영 행 수 조회 실패: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Delete — *sql.Tx 안에서 DELETE. 재고 복구는 service 가 같은 트랜잭션에서 수행.
func (r *SaleRepository) Delete(ctx context.Context, tx *sql.Tx, id int64) error {
	res, err := tx.ExecContext(ctx, "DELETE FROM sales WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("매출 삭제 실패: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("매출 삭제 반영 행 수 조회 실패: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// scanSale — row/rows 한 행을 Sale 로 채운다.
//
// 날짜는 time.Time 직접 스캔 (modernc.org/sqlite 자동 정규화).
// 고객 스냅샷 / 메모는 NULL 허용이라 sql.NullString 으로 받는다.
// category 는 정수로 받아 Category 로 캐스팅.
func scanSale(s rowScanner) (*models.Sale, error) {
	var sale models.Sale
	var categoryInt int
	var customerName, customerMobile, memo sql.NullString
	if err := s.Scan(
		&sale.ID,
		&sale.TransactionDate,
		&sale.ProductID,
		&sale.ProductName,
		&categoryInt,
		&customerName,
		&customerMobile,
		&sale.Quantity,
		&sale.PaymentMethod,
		&memo,
		&sale.CreatedAt,
	); err != nil {
		return nil, err
	}
	sale.ProductCategory = models.Category(categoryInt)
	if customerName.Valid {
		sale.CustomerName = customerName.String
	}
	if customerMobile.Valid {
		sale.CustomerMobile = customerMobile.String
	}
	if memo.Valid {
		sale.Memo = memo.String
	}
	return &sale, nil
}
