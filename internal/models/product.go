package models

import (
	"fmt"
	"time"
)

// Category — 품목 분류.
// DB 에는 정수로 저장되고, 화면 표시는 String() 으로 변환한다.
//
// 주의: 새 분류는 반드시 맨 끝에 추가한다.
// 중간에 끼워 넣으면 기존 DB 레코드의 정수값과 의미가 어긋난다.
type Category int

const (
	CategoryLiquid Category = iota // 0. 액상
	CategoryDevice                 // 1. 기계
	CategoryCoil                   // 2. 코일
	CategoryPod                    // 3. 팟
)

// categoryNames — 정수 → 표시용 한글 이름.
var categoryNames = map[Category]string{
	CategoryLiquid: "액상",
	CategoryDevice: "기계",
	CategoryCoil:   "코일",
	CategoryPod:    "팟",
}

// categoryByName — 표시용 한글 이름 → Category.
// 폼 submit 으로 들어온 문자열을 역변환할 때 사용.
var categoryByName = map[string]Category{
	"액상": CategoryLiquid,
	"기계": CategoryDevice,
	"코일": CategoryCoil,
	"팟":  CategoryPod,
}

// String — fmt.Stringer 구현. 템플릿에서 {{.Category}} 로 한글이 바로 나오게 한다.
func (c Category) String() string {
	if name, ok := categoryNames[c]; ok {
		return name
	}
	return fmt.Sprintf("알수없음(%d)", int(c))
}

// IsValid — DB 에서 읽거나 외부에서 들어온 정수가 정의된 분류 범위 안인지 검사.
func (c Category) IsValid() bool {
	_, ok := categoryNames[c]
	return ok
}

// ParseCategory — 한글 이름을 Category 로 변환. 없으면 에러.
func ParseCategory(s string) (Category, error) {
	if c, ok := categoryByName[s]; ok {
		return c, nil
	}
	return 0, fmt.Errorf("유효하지 않은 분류: %q", s)
}

// AllCategories — 드롭다운/셀렉트 박스에서 항상 같은 순서로 나열하기 위한 목록.
// 맵 순회는 순서가 랜덤이라 화면마다 순서가 달라지는 버그를 막는다.
func AllCategories() []Category {
	return []Category{CategoryLiquid, CategoryDevice, CategoryCoil, CategoryPod}
}

// Product — 품목 마스터.
// 금액은 모두 int64 (원 단위 정수). float 사용 금지.
type Product struct {
	ID            int64
	Category      Category
	Name          string // 최대 50자, DB 에서 UNIQUE
	PurchasePrice int64  // 매입단가
	SalePrice     int64  // 매출단가
	StockQty      int    // 재고 수량 (수량은 음수 될 일 없지만 int 로 유지 — uint 은 Go 관행상 선호되지 않음)
	CreatedAt     time.Time
}
