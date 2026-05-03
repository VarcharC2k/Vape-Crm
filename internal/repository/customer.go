package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/varcharC2k/vape-crm/internal/models"
)

// CustomerRepository — customers 테이블 CRUD.
//
// 매출 트랜잭션과 묶이는 일은 없으므로 (고객 등록은 단독 INSERT) 모든 메서드가 r.db 직접 사용.
type CustomerRepository struct {
	db *sql.DB
}

func NewCustomerRepository(db *sql.DB) *CustomerRepository {
	return &CustomerRepository{db: db}
}

// CustomerFilter — List 필터.
// 부분일치 (LIKE %x%) — 이름과 휴대폰 둘 다 동일 패턴.
type CustomerFilter struct {
	Name   string // 빈 문자열 = 전체
	Mobile string // 빈 문자열 = 전체
}

// Create — 신규 고객 등록.
// 휴대폰 UNIQUE 제약 위반 시 DB 가 에러 반환 → service 가 한글 메시지로 변환.
func (r *CustomerRepository) Create(ctx context.Context, c *models.Customer) error {
	const q = `
		INSERT INTO customers (name, mobile, birth_date, email, address, memo, registration_date)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	res, err := r.db.ExecContext(ctx, q,
		c.Name,
		nullStr(c.Mobile),
		nullDateStr(c.BirthDate),
		nullStr(c.Email),
		nullStr(c.Address),
		nullStr(c.Memo),
		c.RegistrationDate.Format("2006-01-02"),
	)
	if err != nil {
		return fmt.Errorf("고객 등록 실패: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("고객 ID 조회 실패: %w", err)
	}
	c.ID = id
	return nil
}

// GetByID — id 로 단일 조회.
func (r *CustomerRepository) GetByID(ctx context.Context, id int64) (*models.Customer, error) {
	const q = `
		SELECT id, name, mobile, birth_date, email, address, memo, registration_date
		FROM customers
		WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, q, id)
	return scanCustomer(row)
}

// List — 필터 조건에 맞는 고객 목록.
//
// 정렬:
//   - 이름 검색이 있으면: 앞쪽 일치 우선 (CLAUDE.md 부분일치 표준)
//   - 그 외: 이름 ASC
//
// 휴대폰 검색은 단순 부분일치만 (앞쪽 일치 우선 정렬은 안 함 — 보통 뒤 4자리로 검색하므로).
func (r *CustomerRepository) List(ctx context.Context, filter CustomerFilter) ([]*models.Customer, error) {
	var (
		wheres []string
		args   []any
	)

	name := strings.TrimSpace(filter.Name)
	if name != "" {
		wheres = append(wheres, "name LIKE '%' || ? || '%'")
		args = append(args, name)
	}

	mobile := strings.TrimSpace(filter.Mobile)
	if mobile != "" {
		wheres = append(wheres, "mobile LIKE '%' || ? || '%'")
		args = append(args, mobile)
	}

	q := `SELECT id, name, mobile, birth_date, email, address, memo, registration_date FROM customers`
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
		return nil, fmt.Errorf("고객 목록 조회 실패: %w", err)
	}
	defer rows.Close()

	var customers []*models.Customer
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, fmt.Errorf("고객 목록 스캔 실패: %w", err)
		}
		customers = append(customers, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("고객 목록 반복 실패: %w", err)
	}
	return customers, nil
}

// Update — 모든 수정 가능 필드를 한 번에 갱신.
func (r *CustomerRepository) Update(ctx context.Context, c *models.Customer) error {
	const q = `
		UPDATE customers
		SET name = ?, mobile = ?, birth_date = ?, email = ?, address = ?, memo = ?, registration_date = ?
		WHERE id = ?
	`
	res, err := r.db.ExecContext(ctx, q,
		c.Name,
		nullStr(c.Mobile),
		nullDateStr(c.BirthDate),
		nullStr(c.Email),
		nullStr(c.Address),
		nullStr(c.Memo),
		c.RegistrationDate.Format("2006-01-02"),
		c.ID,
	)
	if err != nil {
		return fmt.Errorf("고객 수정 실패: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("고객 수정 반영 행 수 조회 실패: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Delete — 물리 삭제.
// 매출 거래가 있어도 삭제 허용 (사용자 정책). 매출 테이블 추가 시 customer_id 처리는 거기서 결정.
func (r *CustomerRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM customers WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("고객 삭제 실패: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("고객 삭제 반영 행 수 조회 실패: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// scanCustomer — row/rows 한 행을 Customer 로 채운다.
//
// NULL 허용 컬럼은 sql.NullString / sql.NullTime 으로 받아 모델 필드(빈 문자열/zero time)로 변환.
// registration_date 와 birth_date 는 time.Time 으로 직접 스캔
// (modernc.org/sqlite 가 DATE 자동 정규화 — 작업 로그 2026-04-26 참조).
func scanCustomer(s rowScanner) (*models.Customer, error) {
	var c models.Customer
	var mobile, email, address, memo sql.NullString
	var birthDate sql.NullTime
	if err := s.Scan(
		&c.ID,
		&c.Name,
		&mobile,
		&birthDate,
		&email,
		&address,
		&memo,
		&c.RegistrationDate,
	); err != nil {
		return nil, err
	}
	if mobile.Valid {
		c.Mobile = mobile.String
	}
	if email.Valid {
		c.Email = email.String
	}
	if address.Valid {
		c.Address = address.String
	}
	if memo.Valid {
		c.Memo = memo.String
	}
	if birthDate.Valid {
		c.BirthDate = birthDate.Time
	}
	return &c, nil
}

// nullStr — 빈 문자열을 NULL 로 변환.
// database/sql 은 nil 인자를 NULL 로 INSERT 한다.
func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nullDateStr — zero time 을 NULL 로 변환, 그 외엔 'YYYY-MM-DD' 문자열로.
func nullDateStr(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Format("2006-01-02")
}
