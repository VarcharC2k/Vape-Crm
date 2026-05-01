package models

import "time"

// Purchase — 매입 거래 한 건.
//
// 수량은 int. 매입금액 필드는 사용하지 않기로 결정 (2026-04-26).
//
// ProductName, ProductCategory 는 DB 컬럼이 아니라 List/GetByID 에서 products 와
// JOIN 해 함께 채워주는 표시용 필드. Create/Update 입력값으로는 무시되고
// ProductID 만 의미가 있다.
type Purchase struct {
	ID              int64
	TransactionDate time.Time // 날짜만 의미 있음 (시간 부분은 0)
	ProductID       int64
	ProductName     string   // JOIN: products.name
	ProductCategory Category // JOIN: products.category
	Quantity        int
	PaymentMethod   string // 최대 10자
	Memo            string // 선택, 최대 500자
	CreatedAt       time.Time
}
