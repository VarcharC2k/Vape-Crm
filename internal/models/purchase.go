package models

import "time"

// Purchase — 매입 거래 한 건.
//
// 금액은 int64 (원 단위 정수). 수량은 int.
//
// ProductName 은 DB 컬럼이 아니라 List/GetByID 에서 products 와 JOIN 해 함께
// 채워주는 표시용 필드. Create/Update 입력값으로는 무시되고 ProductID 만 의미가 있다.
type Purchase struct {
	ID              int64
	TransactionDate time.Time // 날짜만 의미 있음 (시간 부분은 0)
	ProductID       int64
	ProductName     string // JOIN 으로 채워지는 표시용 필드 (DB 컬럼 아님)
	Quantity        int
	Amount          int64  // 매입 총액
	PaymentMethod   string // 최대 10자
	Memo            string // 선택, 최대 500자
	CreatedAt       time.Time
}
