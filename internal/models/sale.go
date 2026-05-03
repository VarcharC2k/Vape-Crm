package models

import "time"

// Sale — 매출 거래 한 건.
//
// Product 는 FK + JOIN 표시 필드(`ProductName`, `ProductCategory`).
// Customer 는 FK 없이 등록 시점의 이름·휴대폰 스냅샷만 보관. 비회원이면 둘 다 빈 문자열.
//
// 고객을 삭제해도 매출 이력은 스냅샷 덕분에 그대로 보존된다 (사용자 결정).
type Sale struct {
	ID              int64
	TransactionDate time.Time

	// 품목 (FK + JOIN 표시 필드)
	ProductID       int64
	ProductName     string   // JOIN: products.name
	ProductCategory Category // JOIN: products.category

	// 고객 (스냅샷, FK 없음). 비회원이면 둘 다 ""
	CustomerName   string
	CustomerMobile string

	Quantity      int
	PaymentMethod string // 최대 10자
	Memo          string // 선택, 최대 500자
	CreatedAt     time.Time
}
